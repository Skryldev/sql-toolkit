package db

import (
	"context"
	"log/slog"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Hook interface
// ─────────────────────────────────────────────────────────────────────────────

// Hook is called before and after every statement execution.
// Both methods receive the same context, query, and args so tracing spans
// can be started in BeforeQuery and ended in AfterQuery.
//
// Implementations MUST be goroutine-safe and SHOULD be non-blocking.
// Panics inside a hook are recovered by the hook chain and logged.
type Hook interface {
	// BeforeQuery is invoked immediately before the statement is sent to the
	// database driver. Returning an error cancels execution.
	BeforeQuery(ctx context.Context, query string, args []any)

	// AfterQuery is invoked after the driver returns. duration is the
	// wall-clock time spent in the driver call. err is the (already mapped)
	// error returned to the caller — nil on success.
	AfterQuery(ctx context.Context, query string, args []any, duration time.Duration, err error)
}

// ─────────────────────────────────────────────────────────────────────────────
// hookChain — internal dispatcher
// ─────────────────────────────────────────────────────────────────────────────

type hookChain struct {
	hooks []Hook
}

func newHookChain(hooks []Hook) hookChain {
	filtered := make([]Hook, 0, len(hooks))
	for _, h := range hooks {
		if h != nil {
			filtered = append(filtered, h)
		}
	}
	return hookChain{hooks: filtered}
}

func (c hookChain) Before(ctx context.Context, query string, args []any) {
	for _, h := range c.hooks {
		safeBeforeQuery(h, ctx, query, args)
	}
}

func (c hookChain) After(ctx context.Context, query string, args []any, d time.Duration, err error) {
	for _, h := range c.hooks {
		safeAfterQuery(h, ctx, query, args, d, err)
	}
}

func safeBeforeQuery(h Hook, ctx context.Context, query string, args []any) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("sqltoolkit/db: hook panic in BeforeQuery", "panic", r)
		}
	}()
	h.BeforeQuery(ctx, query, args)
}

func safeAfterQuery(h Hook, ctx context.Context, query string, args []any, d time.Duration, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("sqltoolkit/db: hook panic in AfterQuery", "panic", r)
		}
	}()
	h.AfterQuery(ctx, query, args, d, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in hooks — ready to use, zero external dependencies
// ─────────────────────────────────────────────────────────────────────────────

// ── Logging hook ─────────────────────────────────────────────────────────────

// LogHookConfig configures the structured logging hook.
type LogHookConfig struct {
	// Logger defaults to slog.Default() if nil.
	Logger *slog.Logger
	// SlowQueryThreshold logs a warning when duration exceeds this value.
	// Zero disables slow-query logging.
	SlowQueryThreshold time.Duration
	// LogArgs includes bound parameters in log entries (disable in prod if
	// args may contain PII).
	LogArgs bool
}

// NewLogHook returns a Hook that emits structured log entries via slog.
func NewLogHook(cfg LogHookConfig) Hook {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &logHook{cfg: cfg, logger: logger}
}

type logHook struct {
	cfg    LogHookConfig
	logger *slog.Logger
}

func (h *logHook) BeforeQuery(_ context.Context, _ string, _ []any) {}

func (h *logHook) AfterQuery(ctx context.Context, query string, args []any, d time.Duration, err error) {
	attrs := []any{
		slog.String("query", trimQuery(query)),
		slog.Duration("duration", d),
	}
	if h.cfg.LogArgs && len(args) > 0 {
		attrs = append(attrs, slog.Any("args", args))
	}

	if err != nil {
		h.logger.ErrorContext(ctx, "sqltoolkit/db: query error", append(attrs, slog.Any("error", err))...)
		return
	}

	if h.cfg.SlowQueryThreshold > 0 && d > h.cfg.SlowQueryThreshold {
		h.logger.WarnContext(ctx, "sqltoolkit/db: slow query", attrs...)
		return
	}

	h.logger.DebugContext(ctx, "sqltoolkit/db: query", attrs...)
}

func trimQuery(q string) string {
	if len(q) > 500 {
		return q[:500] + "…"
	}
	return q
}

// ── Metrics hook ─────────────────────────────────────────────────────────────

// MetricsCollector is the interface your metrics backend must implement.
// Compatible with Prometheus, StatsD, DataDog, etc.
type MetricsCollector interface {
	// RecordQuery is called after every statement.
	// success is false if err != nil.
	RecordQuery(query string, duration time.Duration, success bool)
}

// NewMetricsHook returns a Hook that delegates to a MetricsCollector.
func NewMetricsHook(collector MetricsCollector) Hook {
	return &metricsHook{c: collector}
}

type metricsHook struct{ c MetricsCollector }

func (h *metricsHook) BeforeQuery(_ context.Context, _ string, _ []any) {}
func (h *metricsHook) AfterQuery(_ context.Context, query string, _ []any, d time.Duration, err error) {
	h.c.RecordQuery(query, d, err == nil)
}

// ── Tracing hook ─────────────────────────────────────────────────────────────

// Tracer is the interface your tracing backend must implement.
// Compatible with OpenTelemetry, Jaeger, DataDog APM, etc.
type Tracer interface {
	// StartSpan is called before the query. The returned context must carry
	// the span so that EndSpan can finish it.
	StartSpan(ctx context.Context, query string) context.Context
	// EndSpan is called after the query completes.
	EndSpan(ctx context.Context, err error)
}

// NewTracingHook returns a Hook wrapping a Tracer.
func NewTracingHook(t Tracer) Hook { return &tracingHook{t: t} }

type tracingHook struct{ t Tracer }

func (h *tracingHook) BeforeQuery(_ context.Context, _ string, _ []any) {}
func (h *tracingHook) AfterQuery(ctx context.Context, query string, _ []any, _ time.Duration, err error) {
	spanCtx := h.t.StartSpan(ctx, query)
	h.t.EndSpan(spanCtx, err)
}

// ── Composite hook helper ─────────────────────────────────────────────────────

// CompositeHook combines multiple hooks into one. Useful when you need to pass
// a single Hook value but want multiple behaviours.
func CompositeHook(hooks ...Hook) Hook { return &compositeHook{hooks: hooks} }

type compositeHook struct{ hooks []Hook }

func (c *compositeHook) BeforeQuery(ctx context.Context, q string, args []any) {
	for _, h := range c.hooks {
		h.BeforeQuery(ctx, q, args)
	}
}
func (c *compositeHook) AfterQuery(ctx context.Context, q string, args []any, d time.Duration, err error) {
	for _, h := range c.hooks {
		h.AfterQuery(ctx, q, args, d, err)
	}
}
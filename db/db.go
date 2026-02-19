// Package db provides a production-grade, SQL-first, ultra-performant
// database toolkit for Go. It is NOT an ORM — all SQL is explicit,
// transparent, and developer-controlled.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Config
// ─────────────────────────────────────────────────────────────────────────────

// Config holds all options for opening and managing the connection pool.
type Config struct {
	// DSN is the driver-specific data-source name.
	DSN string

	// DriverName is "pgx", "postgres", "mysql", or "sqlite3".
	DriverName string

	// Pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	// Default query timeout applied when no deadline is set on the context.
	// Zero means no default timeout.
	DefaultTimeout time.Duration

	// Hooks executed around every statement (logging, metrics, tracing).
	// All hooks are optional; nil entries are silently skipped.
	Hooks []Hook
}

// ─────────────────────────────────────────────────────────────────────────────
// DB — the central type
// ─────────────────────────────────────────────────────────────────────────────

// DB is a thin, concurrency-safe wrapper around *sql.DB.
// It adds context-aware helpers, hook dispatch, unified error mapping,
// and transaction management — nothing more.
//
// All methods accept a context.Context so callers always control timeouts
// and cancellation. The underlying *sql.DB is always accessible via Raw().
type DB struct {
	sqldb   *sql.DB
	cfg     Config
	hooks   hookChain
	errMap  ErrorMapper
}

// Open opens the database described by cfg and verifies connectivity with Ping.
// Callers are responsible for calling Close() when the application shuts down.
func Open(cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("sqltoolkit/db: DSN must not be empty")
	}
	if cfg.DriverName == "" {
		return nil, fmt.Errorf("sqltoolkit/db: DriverName must not be empty")
	}

	sqldb, err := sql.Open(cfg.DriverName, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sqltoolkit/db: open: %w", err)
	}

	// Pool tuning
	if cfg.MaxOpenConns > 0 {
		sqldb.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqldb.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqldb.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		sqldb.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	d := &DB{
		sqldb:  sqldb,
		cfg:    cfg,
		hooks:  newHookChain(cfg.Hooks),
		errMap: DefaultErrorMapper(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqltoolkit/db: ping: %w", err)
	}

	return d, nil
}

// MustOpen is like Open but panics on error. Useful in main() initialisation.
func MustOpen(cfg Config) *DB {
	d, err := Open(cfg)
	if err != nil {
		panic(err)
	}
	return d
}

// Raw returns the underlying *sql.DB for advanced use cases.
// Prefer the wrapper methods where possible.
func (d *DB) Raw() *sql.DB { return d.sqldb }

// SetErrorMapper replaces the default error mapper with a custom one.
// Use this to add driver-specific error code translations.
func (d *DB) SetErrorMapper(m ErrorMapper) { d.errMap = m }

// Close closes all pooled connections and frees resources.
// Safe to call multiple times.
func (d *DB) Close() error { return d.sqldb.Close() }

// Ping verifies that the database is reachable.
func (d *DB) Ping(ctx context.Context) error {
	ctx = d.applyDefaultTimeout(ctx)
	return d.sqldb.PingContext(ctx)
}

// Stats returns pool statistics for monitoring.
func (d *DB) Stats() sql.DBStats { return d.sqldb.Stats() }

// ─────────────────────────────────────────────────────────────────────────────
// Query execution helpers
// ─────────────────────────────────────────────────────────────────────────────

// Exec executes a statement that returns no rows (INSERT, UPDATE, DELETE, DDL).
// It returns the number of rows affected and any error translated through the
// unified error mapper.
func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx = d.applyDefaultTimeout(ctx)
	start := time.Now()
	d.hooks.Before(ctx, query, args)
	res, err := d.sqldb.ExecContext(ctx, query, args...)
	err = d.mapErr(err)
	d.hooks.After(ctx, query, args, time.Since(start), err)
	return res, err
}

// Query executes a query that returns rows.
// The caller MUST close the returned *sql.Rows.
func (d *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx = d.applyDefaultTimeout(ctx)
	start := time.Now()
	d.hooks.Before(ctx, query, args)
	rows, err := d.sqldb.QueryContext(ctx, query, args...)
	err = d.mapErr(err)
	d.hooks.After(ctx, query, args, time.Since(start), err)
	return rows, err
}

// QueryRow executes a query expected to return at most one row.
// Use Scan() on the returned *sql.Row; ErrNotFound is returned when no row
// matches.
func (d *DB) QueryRow(ctx context.Context, query string, args ...any) *Row {
	ctx = d.applyDefaultTimeout(ctx)
	start := time.Now()
	d.hooks.Before(ctx, query, args)
	raw := d.sqldb.QueryRowContext(ctx, query, args...)
	d.hooks.After(ctx, query, args, time.Since(start), nil) // err unknown until Scan
	return &Row{raw: raw, errMap: d.errMap}
}

// ─────────────────────────────────────────────────────────────────────────────
// Prepared statements (optional caching layer)
// ─────────────────────────────────────────────────────────────────────────────

// Prepare creates a prepared statement for repeated use.
// The caller is responsible for calling stmt.Close().
func (d *DB) Prepare(ctx context.Context, query string) (*Stmt, error) {
	ctx = d.applyDefaultTimeout(ctx)
	s, err := d.sqldb.PrepareContext(ctx, query)
	if err != nil {
		return nil, d.mapErr(err)
	}
	return &Stmt{stmt: s, query: query, hooks: d.hooks, errMap: d.errMap}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Batch helpers
// ─────────────────────────────────────────────────────────────────────────────

// BatchExec runs multiple (query, args) pairs inside a single transaction.
// All statements succeed or none do. Use this for bulk inserts / updates
// where each row needs independent parameter binding.
//
// Example (bulk insert):
//
//	err := db.BatchExec(ctx, "INSERT INTO users(name,email) VALUES($1,$2)", rows,
//	    func(row UserRow) []any { return []any{row.Name, row.Email} })
func BatchExec[T any](
	d *DB,
	ctx context.Context,
	query string,
	items []T,
	argsFn func(T) []any,
) error {
	return d.ExecTx(ctx, func(tx *Tx) error {
		stmt, err := tx.Prepare(ctx, query)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, item := range items {
			if _, err := stmt.Exec(ctx, argsFn(item)...); err != nil {
				return err
			}
		}
		return nil
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func (d *DB) applyDefaultTimeout(ctx context.Context) context.Context {
	if d.cfg.DefaultTimeout == 0 {
		return ctx
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx // caller already set a deadline
	}
	ctx, _ = context.WithTimeout(ctx, d.cfg.DefaultTimeout) //nolint:govet
	return ctx
}

func (d *DB) mapErr(err error) error {
	if err == nil {
		return nil
	}
	return d.errMap.Map(err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Row — wraps *sql.Row to translate errors uniformly
// ─────────────────────────────────────────────────────────────────────────────

// Row wraps *sql.Row and maps errors through the unified error mapper.
type Row struct {
	raw    *sql.Row
	errMap ErrorMapper
}

// Scan copies columns from the matched row into dest values.
// ErrNotFound is returned when no row was found.
func (r *Row) Scan(dest ...any) error {
	err := r.raw.Scan(dest...)
	return r.errMap.Map(err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Stmt — wraps *sql.Stmt
// ─────────────────────────────────────────────────────────────────────────────

// Stmt wraps a prepared *sql.Stmt with hook dispatch and error mapping.
type Stmt struct {
	stmt   *sql.Stmt
	query  string
	hooks  hookChain
	errMap ErrorMapper
}

// Exec executes the prepared statement.
func (s *Stmt) Exec(ctx context.Context, args ...any) (sql.Result, error) {
	start := time.Now()
	s.hooks.Before(ctx, s.query, args)
	res, err := s.stmt.ExecContext(ctx, args...)
	err = s.errMap.Map(err)
	s.hooks.After(ctx, s.query, args, time.Since(start), err)
	return res, err
}

// QueryRow executes the prepared statement expecting one row.
func (s *Stmt) QueryRow(ctx context.Context, args ...any) *Row {
	start := time.Now()
	s.hooks.Before(ctx, s.query, args)
	raw := s.stmt.QueryRowContext(ctx, args...)
	s.hooks.After(ctx, s.query, args, time.Since(start), nil)
	return &Row{raw: raw, errMap: s.errMap}
}

// Close releases the prepared statement resources.
func (s *Stmt) Close() error { return s.stmt.Close() }

// ─────────────────────────────────────────────────────────────────────────────
// WithRetry — resilience helper
// ─────────────────────────────────────────────────────────────────────────────

// RetryConfig controls retry behaviour for transient errors.
type RetryConfig struct {
	MaxAttempts int
	Delay       time.Duration
	// RetryOn decides whether a given error should trigger a retry.
	// Defaults to retrying on ErrDeadlock and ErrTimeout if nil.
	RetryOn func(error) bool
}

// WithRetry executes fn, retrying on transient errors per cfg.
// It is safe to pass a transaction operation inside fn; just make sure fn
// is idempotent or handles partial state correctly.
func WithRetry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	retryOn := cfg.RetryOn
	if retryOn == nil {
		retryOn = func(err error) bool {
			return IsDeadlock(err) || IsTimeout(err)
		}
	}
	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.Delay):
			}
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !retryOn(lastErr) {
			return lastErr
		}
	}
	return fmt.Errorf("sqltoolkit/db: all %d attempts failed, last error: %w", cfg.MaxAttempts, lastErr)
}
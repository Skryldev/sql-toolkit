// main.go — Production-Grade SQL Toolkit for Go
// ============================================================
// This file demonstrates every capability of the toolkit:
//
//  1. DB initialisation with connection pool tuning
//  2. Hook setup (logging, metrics, tracing)
//  3. InsertUser
//  4. GetUserByID
//  5. GetByEmail
//  6. UpdateUser (partial)
//  7. DeleteUser
//  8. Transaction usage
//  9. Type-safe error handling
// 10. Batch insert
// 11. Retry / timeout
// 12. Migration execution (programmatic)
// ============================================================
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Skryldev/sql-toolkit/db"
	"github.com/Skryldev/sql-toolkit/models"
	"github.com/Skryldev/sql-toolkit/repo"

	// Blank-import the postgres driver so it self-registers with database/sql.
	_ "github.com/lib/pq"
)

func main() {
	// ── 0. Structured logger ──────────────────────────────────────────────
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// ── 1. DB initialisation ─────────────────────────────────────────────
	//
	// All configuration is explicit. No environment magic inside the toolkit.
	// Use DSNFromEnv() or build the DSN string yourself.

	dsn := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/devdb?sslmode=disable")

	database := db.MustOpen(db.Config{
		DSN:             dsn,
		DriverName:      "postgres",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
		DefaultTimeout:  10 * time.Second,

		// ── Hooks ─────────────────────────────────────────────────────────
		Hooks: []db.Hook{
			// Logging hook: debug-level for all queries, warn for slow ones
			db.NewLogHook(db.LogHookConfig{
				Logger:             logger,
				SlowQueryThreshold: 200 * time.Millisecond,
				LogArgs:            false, // set true in development only
			}),

			// Metrics hook: plug in your Prometheus / StatsD collector
			db.NewMetricsHook(&prometheusCollector{}),

			// Tracing hook: plug in your OpenTelemetry tracer
			db.NewTracingHook(&noopTracer{}),
		},
	})
	defer database.Close()

	slog.Info("database connected", "stats", database.Stats())

	ctx := context.Background()

	// ── 2. InsertUser ─────────────────────────────────────────────────────
	userRepo := repo.NewUserRepo(database)

	alice, err := userRepo.Insert(ctx, models.CreateUserParams{
		Name:  "Alice Smith",
		Email: "alice@example.com",
	})
	if err != nil {
		if db.IsDuplicateKey(err) {
			slog.Warn("insert skipped — email already exists")
		} else {
			fatalf("insert user: %v", err)
		}
	} else {
		slog.Info("inserted user", "id", alice.ID, "email", alice.Email)
	}

	// ── 3. GetUserByID ────────────────────────────────────────────────────
	if alice != nil {
		fetched, err := userRepo.GetByID(ctx, alice.ID)
		if err != nil {
			if db.IsNotFound(err) {
				slog.Warn("user not found", "id", alice.ID)
			} else {
				fatalf("get user: %v", err)
			}
		} else {
			slog.Info("fetched user", "user", fetched)
		}
	}

	// ── 4. GetByEmail ─────────────────────────────────────────────────────
	byEmail, err := userRepo.GetByEmail(ctx, "alice@example.com")
	if err != nil {
		if !db.IsNotFound(err) {
			fatalf("get by email: %v", err)
		}
	} else {
		slog.Info("found by email", "id", byEmail.ID)
	}

	// ── 5. UpdateUser (partial) ───────────────────────────────────────────
	if alice != nil {
		newName := "Alice Johnson"
		updated, err := userRepo.Update(ctx, models.UpdateUserParams{
			ID:   alice.ID,
			Name: &newName,
			// Email is nil → not touched
		})
		if err != nil {
			fatalf("update user: %v", err)
		}
		slog.Info("updated user", "name", updated.Name)
	}

	// ── 6. Transaction usage ──────────────────────────────────────────────
	//
	// ExecTx automatically commits on success and rolls back on any error
	// or panic. The *Tx satisfies db.Querier so repositories work unchanged.

	err = database.ExecTx(ctx, func(tx *db.Tx) error {
		txRepo := repo.NewUserRepo(tx)

		bob, err := txRepo.Insert(ctx, models.CreateUserParams{
			Name:  "Bob Builder",
			Email: "bob@example.com",
		})
		if err != nil {
			return fmt.Errorf("insert bob: %w", err)
		}

		carol, err := txRepo.Insert(ctx, models.CreateUserParams{
			Name:  "Carol White",
			Email: "carol@example.com",
		})
		if err != nil {
			return fmt.Errorf("insert carol: %w", err)
		}

		slog.Info("tx: inserted users", "bob", bob.ID, "carol", carol.ID)
		return nil // ← triggers COMMIT
	})
	if err != nil {
		fatalf("transaction failed: %v", err)
	}

	// ── 7. Type-safe error handling ───────────────────────────────────────
	//
	// All errors are mapped to sentinel types. Use errors.Is() or the
	// convenience helpers (db.IsNotFound, db.IsDuplicateKey, etc.).

	_, err = userRepo.GetByID(ctx, 999_999)
	switch {
	case db.IsNotFound(err):
		slog.Info("correctly handled not-found")
	case db.IsTimeout(err):
		slog.Error("query timed out")
	case err != nil:
		slog.Error("unexpected error", "err", err)
	}

	// Attempting a duplicate insert to demonstrate ErrDuplicateKey:
	_, err = userRepo.Insert(ctx, models.CreateUserParams{
		Name:  "Alice Again",
		Email: "alice@example.com", // already exists
	})
	if db.IsDuplicateKey(err) {
		slog.Info("correctly caught duplicate key error")
	}

	// Inspect the underlying driver error when needed:
	var dbErr *db.DBError
	if errors.As(err, &dbErr) {
		slog.Debug("raw driver error", "cause", dbErr.Cause)
	}

	// ── 8. Batch insert ───────────────────────────────────────────────────
	batchParams := []models.CreateUserParams{
		{Name: "Dave", Email: "dave@example.com"},
		{Name: "Eve", Email: "eve@example.com"},
		{Name: "Frank", Email: "frank@example.com"},
	}

	// Option A: use the generic BatchExec helper (wraps in a tx automatically)
	err = db.BatchExec(
		database, ctx,
		`INSERT INTO users (name, email, created_at, updated_at)
		 VALUES ($1, $2, NOW(), NOW())`,
		batchParams,
		func(p models.CreateUserParams) []any {
			return []any{p.Name, p.Email}
		},
	)
	if err != nil {
		slog.Error("batch insert failed", "err", err)
	} else {
		slog.Info("batch insert: done", "count", len(batchParams))
	}

	// Option B: use the repository's BatchInsert (returns full user records)
	inserted, err := userRepo.BatchInsert(ctx, []models.CreateUserParams{
		{Name: "Grace", Email: "grace@example.com"},
		{Name: "Hank", Email: "hank@example.com"},
	})
	if err != nil {
		slog.Error("repo batch insert failed", "err", err)
	} else {
		slog.Info("repo batch insert", "inserted", len(inserted))
	}

	// ── 9. Retry / timeout ────────────────────────────────────────────────
	//
	// WithRetry wraps any operation with configurable retry logic.
	// By default it retries on ErrDeadlock and ErrTimeout.

	retryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err = db.WithRetry(retryCtx, db.RetryConfig{
		MaxAttempts: 3,
		Delay:       100 * time.Millisecond,
		RetryOn: func(err error) bool {
			return db.IsDeadlock(err) || db.IsTimeout(err)
		},
	}, func() error {
		_, err := userRepo.Insert(ctx, models.CreateUserParams{
			Name:  "Retry User",
			Email: fmt.Sprintf("retry-%d@example.com", time.Now().UnixNano()),
		})
		return err
	})
	if err != nil {
		slog.Error("retry operation failed", "err", err)
	} else {
		slog.Info("retry operation succeeded")
	}

	// ── 10. DeleteUser ────────────────────────────────────────────────────
	if alice != nil {
		if err := userRepo.Delete(ctx, alice.ID); err != nil {
			if !db.IsNotFound(err) {
				fatalf("delete user: %v", err)
			}
		} else {
			slog.Info("deleted user", "id", alice.ID)
		}
	}

	// ── 11. Health check / pool stats ─────────────────────────────────────
	if err := database.Ping(ctx); err != nil {
		slog.Error("health check failed", "err", err)
	} else {
		stats := database.Stats()
		slog.Info("pool stats",
			"open", stats.OpenConnections,
			"idle", stats.Idle,
			"in_use", stats.InUse,
			"wait_count", stats.WaitCount,
		)
	}

	slog.Info("all examples completed")
}

// ─────────────────────────────────────────────────────────────────────────────
// Stub implementations for hooks (replace with real ones in your project)
// ─────────────────────────────────────────────────────────────────────────────

// prometheusCollector is a stub MetricsCollector.
// Replace with your Prometheus/StatsD/DataDog implementation.
type prometheusCollector struct{}

func (p *prometheusCollector) RecordQuery(query string, d time.Duration, ok bool) {
	// e.g. histogram.Observe(d.Seconds())
}

// noopTracer is a stub Tracer. Replace with your OpenTelemetry tracer.
type noopTracer struct{}

func (t *noopTracer) StartSpan(ctx context.Context, _ string) context.Context { return ctx }
func (t *noopTracer) EndSpan(_ context.Context, _ error)                      {}

// ─────────────────────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatalf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
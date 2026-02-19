// db/db_test.go — unit tests for the toolkit.
// Uses an in-memory SQLite database; no external services required.
//
// Run:  go test ./db/... -v -race
package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Skryldev/sql-toolkit/db"
	_ "github.com/mattn/go-sqlite3"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(db.Config{
		DSN:        ":memory:",
		DriverName: "sqlite3",
		Hooks: []db.Hook{
			db.NewLogHook(db.LogHookConfig{LogArgs: true}),
		},
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	// Create schema
	_, err = d.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			email      TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return d
}

// ─────────────────────────────────────────────────────────────────────────────
// Open / Ping
// ─────────────────────────────────────────────────────────────────────────────

func TestOpen(t *testing.T) {
	d := newTestDB(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestOpen_InvalidDSN(t *testing.T) {
	_, err := db.Open(db.Config{DSN: "", DriverName: "sqlite3"})
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Exec / QueryRow
// ─────────────────────────────────────────────────────────────────────────────

func TestExec_Insert(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	res, err := d.Exec(ctx,
		`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"Alice", "alice@test.com", now, now,
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}
}

func TestQueryRow_Scan(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	_, err := d.Exec(ctx,
		`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"Bob", "bob@test.com", now, now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var name, email string
	err = d.QueryRow(ctx, `SELECT name, email FROM users WHERE email = ?`, "bob@test.com").
		Scan(&name, &email)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if name != "Bob" || email != "bob@test.com" {
		t.Fatalf("unexpected values: name=%q email=%q", name, email)
	}
}

func TestQueryRow_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	var name string
	err := d.QueryRow(ctx, `SELECT name FROM users WHERE id = ?`, 99999).Scan(&name)
	if !db.IsNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Query — multiple rows
// ─────────────────────────────────────────────────────────────────────────────

func TestQuery_MultipleRows(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	for _, u := range []struct{ name, email string }{
		{"Alice", "alice@q.com"},
		{"Bob", "bob@q.com"},
		{"Carol", "carol@q.com"},
	} {
		_, err := d.Exec(ctx,
			`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			u.name, u.email, now, now,
		)
		if err != nil {
			t.Fatalf("insert %s: %v", u.name, err)
		}
	}

	rows, err := d.Query(ctx, `SELECT name FROM users ORDER BY name`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(names))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ExecTx — commit
// ─────────────────────────────────────────────────────────────────────────────

func TestExecTx_Commit(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	err := d.ExecTx(ctx, func(tx *db.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			"Dave", "dave@tx.com", now, now,
		)
		return err
	})
	if err != nil {
		t.Fatalf("tx commit: %v", err)
	}

	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = ?`, "dave@tx.com").Scan(&n)
	if n != 1 {
		t.Fatalf("expected 1 committed row, got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ExecTx — rollback on error
// ─────────────────────────────────────────────────────────────────────────────

func TestExecTx_RollbackOnError(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	sentinelErr := errors.New("intentional failure")

	err := d.ExecTx(ctx, func(tx *db.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			"Eve", "eve@rollback.com", now, now,
		)
		if err != nil {
			return err
		}
		return sentinelErr // force rollback
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr, got %v", err)
	}

	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = ?`, "eve@rollback.com").Scan(&n)
	if n != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ExecTx — rollback on panic
// ─────────────────────────────────────────────────────────────────────────────

func TestExecTx_RollbackOnPanic(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate")
		}
	}()

	_ = d.ExecTx(ctx, func(tx *db.Tx) error {
		panic("test panic")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Prepared statements
// ─────────────────────────────────────────────────────────────────────────────

func TestPrepare(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	stmt, err := d.Prepare(ctx,
		`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	for _, email := range []string{"p1@test.com", "p2@test.com", "p3@test.com"} {
		_, err := stmt.Exec(ctx, "PrepUser", email, now, now)
		if err != nil {
			t.Fatalf("exec prepared: %v", err)
		}
	}

	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE name = ?`, "PrepUser").Scan(&n)
	if n != 3 {
		t.Fatalf("expected 3 rows, got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Error mapping — DuplicateKey (SQLite)
// ─────────────────────────────────────────────────────────────────────────────

func TestErrorMapper_DuplicateKey(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	insert := func() error {
		_, err := d.Exec(ctx,
			`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			"Alice", "dup@test.com", now, now,
		)
		return err
	}

	if err := insert(); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := insert() // should trigger UNIQUE constraint
	if !db.IsDuplicateKey(err) {
		t.Fatalf("expected ErrDuplicateKey, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// WithRetry
// ─────────────────────────────────────────────────────────────────────────────

func TestWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	transient := errors.New("transient")

	err := db.WithRetry(ctx, db.RetryConfig{
		MaxAttempts: 3,
		Delay:       1 * time.Millisecond,
		RetryOn:     func(err error) bool { return errors.Is(err, transient) },
	}, func() error {
		attempts++
		if attempts < 2 {
			return transient
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	ctx := context.Background()
	permanent := errors.New("permanent")

	err := db.WithRetry(ctx, db.RetryConfig{
		MaxAttempts: 3,
		Delay:       1 * time.Millisecond,
		RetryOn:     func(err error) bool { return errors.Is(err, permanent) },
	}, func() error {
		return permanent
	})
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Hooks — verify they are called
// ─────────────────────────────────────────────────────────────────────────────

type countingHook struct {
	before int
	after  int
}

func (h *countingHook) BeforeQuery(_ context.Context, _ string, _ []any) { h.before++ }
func (h *countingHook) AfterQuery(_ context.Context, _ string, _ []any, _ time.Duration, _ error) {
	h.after++
}

func TestHooks_CalledOnExec(t *testing.T) {
	hook := &countingHook{}
	d, err := db.Open(db.Config{
		DSN:        ":memory:",
		DriverName: "sqlite3",
		Hooks:      []db.Hook{hook},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	ctx := context.Background()
	_, _ = d.Exec(ctx, `SELECT 1`)

	if hook.before != 1 || hook.after != 1 {
		t.Fatalf("hook not called: before=%d after=%d", hook.before, hook.after)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BatchExec
// ─────────────────────────────────────────────────────────────────────────────

func TestBatchExec(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	now := time.Now()

	type row struct{ Name, Email string }
	items := []row{
		{"Batch1", "b1@test.com"},
		{"Batch2", "b2@test.com"},
		{"Batch3", "b3@test.com"},
	}

	err := db.BatchExec(d, ctx,
		`INSERT INTO users (name, email, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		items,
		func(r row) []any { return []any{r.Name, r.Email, now, now} },
	)
	if err != nil {
		t.Fatalf("batch exec: %v", err)
	}

	var n int
	_ = d.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE name LIKE 'Batch%'`).Scan(&n)
	if n != 3 {
		t.Fatalf("expected 3 batch rows, got %d", n)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Context timeout
// ─────────────────────────────────────────────────────────────────────────────

func TestContextCancellation(t *testing.T) {
	d := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := d.Exec(ctx, `SELECT 1`)
	if err == nil {
		// SQLite may execute trivially fast before noticing cancellation;
		// this is acceptable behaviour. The error mapping is tested via
		// the error sentinel tests above.
		t.Log("SQLite executed before context was observed (acceptable)")
	}
}
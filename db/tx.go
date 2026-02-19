package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Tx — transaction wrapper
// ─────────────────────────────────────────────────────────────────────────────

// Tx is a thin wrapper around *sql.Tx that mirrors the DB API surface so that
// repository code can accept either *DB or *Tx via the Querier interface.
type Tx struct {
	sqltx  *sql.Tx
	hooks  hookChain
	errMap ErrorMapper
	cfg    Config
}

// Raw returns the underlying *sql.Tx for advanced use.
func (t *Tx) Raw() *sql.Tx { return t.sqltx }

// Exec executes a statement that does not return rows.
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	t.hooks.Before(ctx, query, args)
	res, err := t.sqltx.ExecContext(ctx, query, args...)
	err = t.mapErr(err)
	t.hooks.After(ctx, query, args, time.Since(start), err)
	return res, err
}

// Query executes a query returning rows. The caller MUST close *sql.Rows.
func (t *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	t.hooks.Before(ctx, query, args)
	rows, err := t.sqltx.QueryContext(ctx, query, args...)
	err = t.mapErr(err)
	t.hooks.After(ctx, query, args, time.Since(start), err)
	return rows, err
}

// QueryRow executes a query expected to return at most one row.
func (t *Tx) QueryRow(ctx context.Context, query string, args ...any) *Row {
	start := time.Now()
	t.hooks.Before(ctx, query, args)
	raw := t.sqltx.QueryRowContext(ctx, query, args...)
	t.hooks.After(ctx, query, args, time.Since(start), nil)
	return &Row{raw: raw, errMap: t.errMap}
}

// Prepare creates a prepared statement within the transaction.
func (t *Tx) Prepare(ctx context.Context, query string) (*Stmt, error) {
	s, err := t.sqltx.PrepareContext(ctx, query)
	if err != nil {
		return nil, t.mapErr(err)
	}
	return &Stmt{stmt: s, query: query, hooks: t.hooks, errMap: t.errMap}, nil
}

func (t *Tx) mapErr(err error) error {
	if err == nil {
		return nil
	}
	return t.errMap.Map(err)
}

// ─────────────────────────────────────────────────────────────────────────────
// ExecTx — the primary transaction helper on *DB
// ─────────────────────────────────────────────────────────────────────────────

// TxOptions allows callers to configure isolation level and read-only flag.
type TxOptions struct {
	Isolation sql.IsolationLevel
	ReadOnly  bool
}

// ExecTx starts a transaction, executes fn, and automatically commits on
// success or rolls back on error or panic. Nested calls (within an already
// active transaction) are NOT supported by the standard driver — use
// savepoints if you need that level of control.
//
//	err := db.ExecTx(ctx, func(tx *Tx) error {
//	    if _, err := tx.Exec(ctx, "UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromID); err != nil {
//	        return err
//	    }
//	    _, err := tx.Exec(ctx, "UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toID)
//	    return err
//	})
func (d *DB) ExecTx(ctx context.Context, fn func(*Tx) error, opts ...TxOptions) (err error) {
	return d.ExecTxOpts(ctx, fn, opts...)
}

// ExecTxOpts is ExecTx with explicit options forwarding.
func (d *DB) ExecTxOpts(ctx context.Context, fn func(*Tx) error, opts ...TxOptions) (err error) {
	ctx = d.applyDefaultTimeout(ctx)

	var sqlOpts *sql.TxOptions
	if len(opts) > 0 {
		sqlOpts = &sql.TxOptions{
			Isolation: opts[0].Isolation,
			ReadOnly:  opts[0].ReadOnly,
		}
	}

	sqltx, err := d.sqldb.BeginTx(ctx, sqlOpts)
	if err != nil {
		return d.mapErr(err)
	}

	tx := &Tx{
		sqltx:  sqltx,
		hooks:  d.hooks,
		errMap: d.errMap,
		cfg:    d.cfg,
	}

	// Ensure rollback on panic or error.
	defer func() {
		if p := recover(); p != nil {
			_ = sqltx.Rollback()
			panic(p) // re-panic after rollback
		}
		if err != nil {
			if rbErr := sqltx.Rollback(); rbErr != nil {
				// Wrap both errors so callers see the full picture.
				err = fmt.Errorf("sqltoolkit/db: rollback failed (%v) after original error: %w", rbErr, err)
			}
		}
	}()

	err = fn(tx)
	if err != nil {
		return d.mapErr(err) // rollback handled by defer
	}

	if err = sqltx.Commit(); err != nil {
		return d.mapErr(err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Querier — the shared interface accepted by repositories
// ─────────────────────────────────────────────────────────────────────────────

// Querier is the minimal interface shared by both *DB and *Tx.
// Repository constructors should accept Querier instead of *DB so they work
// seamlessly inside transactions.
//
//	type UserRepo struct{ q db.Querier }
//	func NewUserRepo(q db.Querier) *UserRepo { return &UserRepo{q: q} }
type Querier interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *Row
	Prepare(ctx context.Context, query string) (*Stmt, error)
}

// Verify at compile-time that both *DB and *Tx satisfy Querier.
var (
	_ Querier = (*DB)(nil)
	_ Querier = (*Tx)(nil)
)
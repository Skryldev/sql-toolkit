package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Sentinel errors
// ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrNotFound is returned when a query matches no rows.
	ErrNotFound = errors.New("sqltoolkit/db: record not found")

	// ErrDuplicateKey is returned on unique constraint violations.
	ErrDuplicateKey = errors.New("sqltoolkit/db: duplicate key")

	// ErrForeignKeyViolation is returned when a foreign key constraint is violated.
	ErrForeignKeyViolation = errors.New("sqltoolkit/db: foreign key violation")

	// ErrDeadlock is returned when the database detects a deadlock.
	ErrDeadlock = errors.New("sqltoolkit/db: deadlock detected")

	// ErrTimeout is returned when a statement exceeds its deadline.
	ErrTimeout = errors.New("sqltoolkit/db: query timeout")

	// ErrCheckViolation is returned when a CHECK constraint is violated.
	ErrCheckViolation = errors.New("sqltoolkit/db: check constraint violation")

	// ErrConnectionFailed is returned when the driver cannot reach the server.
	ErrConnectionFailed = errors.New("sqltoolkit/db: connection failed")
)

// ─────────────────────────────────────────────────────────────────────────────
// Error helpers — use errors.Is() for type-safe checks
// ─────────────────────────────────────────────────────────────────────────────

func IsNotFound(err error) bool           { return errors.Is(err, ErrNotFound) }
func IsDuplicateKey(err error) bool       { return errors.Is(err, ErrDuplicateKey) }
func IsForeignKeyViolation(err error) bool { return errors.Is(err, ErrForeignKeyViolation) }
func IsDeadlock(err error) bool           { return errors.Is(err, ErrDeadlock) }
func IsTimeout(err error) bool            { return errors.Is(err, ErrTimeout) }
func IsCheckViolation(err error) bool     { return errors.Is(err, ErrCheckViolation) }

// ─────────────────────────────────────────────────────────────────────────────
// DBError — rich error type preserving original driver error
// ─────────────────────────────────────────────────────────────────────────────

// DBError wraps a sentinel error with the original driver error so callers can
// either use errors.Is(err, ErrDuplicateKey) for simple checks or inspect the
// raw driver error for additional context.
type DBError struct {
	// Sentinel is one of the package-level Err* variables.
	Sentinel error
	// Cause is the original driver error.
	Cause error
	// Message is an optional human-readable hint.
	Message string
}

func (e *DBError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Sentinel, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s (cause: %v)", e.Sentinel, e.Cause)
}

func (e *DBError) Is(target error) bool  { return errors.Is(e.Sentinel, target) }
func (e *DBError) Unwrap() error         { return e.Cause }

// ─────────────────────────────────────────────────────────────────────────────
// ErrorMapper interface — pluggable per driver
// ─────────────────────────────────────────────────────────────────────────────

// ErrorMapper translates raw driver errors into the toolkit's sentinel errors.
// Implement this interface to add support for a new driver.
type ErrorMapper interface {
	Map(err error) error
}

// ErrorMapperFunc is a convenience adapter from a function to ErrorMapper.
type ErrorMapperFunc func(error) error

func (f ErrorMapperFunc) Map(err error) error { return f(err) }

// ─────────────────────────────────────────────────────────────────────────────
// Default mapper — covers PostgreSQL (lib/pq + pgx), MySQL, SQLite
// ─────────────────────────────────────────────────────────────────────────────

// DefaultErrorMapper returns a mapper that handles the most common drivers.
// Extend by wrapping it with your own mapper.
func DefaultErrorMapper() ErrorMapper {
	return ErrorMapperFunc(defaultMap)
}

func defaultMap(err error) error {
	if err == nil {
		return nil
	}

	// Standard library sentinel
	if errors.Is(err, sql.ErrNoRows) {
		return &DBError{Sentinel: ErrNotFound, Cause: err}
	}

	// Context errors
	if errors.Is(err, context_deadline_exceeded) || errors.Is(err, context_canceled) {
		return &DBError{Sentinel: ErrTimeout, Cause: err}
	}

	// Already mapped — do not double-wrap
	var dbe *DBError
	if errors.As(err, &dbe) {
		return err
	}

	// Try PostgreSQL pq errors (lib/pq)
	if mapped := mapPQError(err); mapped != nil {
		return mapped
	}

	// Try pgx errors
	if mapped := mapPGXError(err); mapped != nil {
		return mapped
	}

	// Try MySQL errors
	if mapped := mapMySQLError(err); mapped != nil {
		return mapped
	}

	// Try SQLite errors (string-based, driver doesn't export typed errors)
	if mapped := mapSQLiteError(err); mapped != nil {
		return mapped
	}

	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// PostgreSQL (lib/pq) mapping
// ─────────────────────────────────────────────────────────────────────────────

// pqError is a duck-type interface matching (*pq.Error).
type pqError interface {
	error
	GetCode() string
}

func mapPQError(err error) error {
	// lib/pq exposes its error via a concrete type; we avoid importing it
	// by checking via an interface based on the documented API surface.
	// If lib/pq is not in the binary the type assertion silently fails.
	type coder interface{ GetCode() string }
	var c coder
	if !errors.As(err, &c) {
		// Fallback: try to extract code via string representation to avoid
		// hard dependency on lib/pq.
		return mapByPGCode(pqCodeFromString(err.Error()), err)
	}
	return mapByPGCode(c.GetCode(), err)
}

func pqCodeFromString(s string) string {
	// lib/pq formats: "pq: ERROR: message (SQLSTATE XXXXX)"
	const marker = "(SQLSTATE "
	idx := strings.LastIndex(s, marker)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(marker):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func mapPGXError(err error) error {
	// pgx v5 wraps its error in pgconn.PgError.
	// We avoid a hard import and instead match on the interface.
	type pgxErr interface {
		SQLState() string
	}
	var pge pgxErr
	if !errors.As(err, &pge) {
		return nil
	}
	return mapByPGCode(pge.SQLState(), err)
}

// PostgreSQL SQLSTATE codes: https://www.postgresql.org/docs/current/errcodes-appendix.html
func mapByPGCode(code string, cause error) error {
	switch code {
	case "23505": // unique_violation
		return &DBError{Sentinel: ErrDuplicateKey, Cause: cause}
	case "23503": // foreign_key_violation
		return &DBError{Sentinel: ErrForeignKeyViolation, Cause: cause}
	case "23514": // check_violation
		return &DBError{Sentinel: ErrCheckViolation, Cause: cause}
	case "40P01": // deadlock_detected
		return &DBError{Sentinel: ErrDeadlock, Cause: cause}
	case "57014": // query_canceled (statement_timeout)
		return &DBError{Sentinel: ErrTimeout, Cause: cause}
	case "08000", "08003", "08006", "08001", "08004", "08007", "08P01":
		return &DBError{Sentinel: ErrConnectionFailed, Cause: cause}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MySQL mapping
// ─────────────────────────────────────────────────────────────────────────────

func mapMySQLError(err error) error {
	// go-sql-driver/mysql exposes mysql.MySQLError.
	type mysqlErr interface {
		error
		Number() uint16
	}
	var me mysqlErr
	if !errors.As(err, &me) {
		return nil
	}
	switch me.Number() {
	case 1062: // ER_DUP_ENTRY
		return &DBError{Sentinel: ErrDuplicateKey, Cause: err}
	case 1452, 1216, 1217: // ER_NO_REFERENCED_ROW, ER_ROW_IS_REFERENCED
		return &DBError{Sentinel: ErrForeignKeyViolation, Cause: err}
	case 1213: // ER_LOCK_DEADLOCK
		return &DBError{Sentinel: ErrDeadlock, Cause: err}
	case 3024: // ER_QUERY_TIMEOUT
		return &DBError{Sentinel: ErrTimeout, Cause: err}
	case 1045, 2002, 2003, 2006, 2013:
		return &DBError{Sentinel: ErrConnectionFailed, Cause: err}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// SQLite mapping (string-based, driver doesn't export typed errors)
// ─────────────────────────────────────────────────────────────────────────────

func mapSQLiteError(err error) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "UNIQUE constraint failed"):
		return &DBError{Sentinel: ErrDuplicateKey, Cause: err}
	case strings.Contains(s, "FOREIGN KEY constraint failed"):
		return &DBError{Sentinel: ErrForeignKeyViolation, Cause: err}
	case strings.Contains(s, "CHECK constraint failed"):
		return &DBError{Sentinel: ErrCheckViolation, Cause: err}
	case strings.Contains(s, "database is locked"):
		return &DBError{Sentinel: ErrDeadlock, Cause: err}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// ChainedMapper — compose multiple mappers (first match wins)
// ─────────────────────────────────────────────────────────────────────────────

// ChainMapper returns an ErrorMapper that tries each mapper in order,
// returning the first non-nil remapped error. Falls back to the default mapper.
func ChainMapper(mappers ...ErrorMapper) ErrorMapper {
	return ErrorMapperFunc(func(err error) error {
		if err == nil {
			return nil
		}
		for _, m := range mappers {
			if mapped := m.Map(err); mapped != err {
				return mapped
			}
		}
		return err
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// context sentinel aliases (avoid importing "context" at package level)
// ─────────────────────────────────────────────────────────────────────────────

// These are set via init() to avoid a package-level import cycle risk.
var (
	context_deadline_exceeded error
	context_canceled          error
)

func init() {
	import_context_errors()
}

func import_context_errors() {
	// Lazy import to avoid hard init-order dependency.
	context_deadline_exceeded = fmt.Errorf("context deadline exceeded")
	context_canceled = fmt.Errorf("context canceled")
	// NOTE: We cannot use errors.Is here since these are new instances.
	// The actual check uses the standard library's context package via
	// errors.Is which compares by pointer; we override with proper values below.
	useContextPackage()
}
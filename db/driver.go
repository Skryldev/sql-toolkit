// Package db — driver.go
// Defines the pluggable driver abstraction layer. Each driver adapter
// implements Driver and registers itself, enabling Open() to be
// driver-agnostic while preserving explicit DSN construction per database.
package db

import (
	"database/sql"
	"fmt"
	"sync"
)

// ─────────────────────────────────────────────────────────────────────────────
// Driver interface
// ─────────────────────────────────────────────────────────────────────────────

// Driver encapsulates database-specific behaviour:
//   - building a DSN from structured options
//   - registering the database/sql driver (idempotent)
//   - providing a driver-specific ErrorMapper
//
// Implement Driver to add support for a new database without modifying the
// core package.
type Driver interface {
	// Name returns the name passed to sql.Register, e.g. "pgx", "mysql".
	Name() string

	// DSN converts structured options into a driver DSN string.
	DSN(opts DriverOptions) (string, error)

	// ErrorMapper returns a mapper tuned to this driver's error types.
	ErrorMapper() ErrorMapper

	// Register ensures the driver is registered with database/sql.
	// Implementations must be idempotent (safe to call multiple times).
	Register()
}

// DriverOptions carries the most common connection parameters in a structured,
// driver-agnostic form. DSN() converts them to the driver's native format.
type DriverOptions struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string // "disable", "require", "verify-full", etc.
	// Extra holds driver-specific key/value parameters.
	Extra map[string]string
}

// ─────────────────────────────────────────────────────────────────────────────
// Driver registry
// ─────────────────────────────────────────────────────────────────────────────

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]Driver)
)

// RegisterDriver adds a Driver to the global registry.
// Panics if a driver with the same name is already registered (use ReplaceDriver
// to override).
func RegisterDriver(d Driver) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if _, ok := drivers[d.Name()]; ok {
		panic(fmt.Sprintf("sqltoolkit/db: driver %q already registered", d.Name()))
	}
	drivers[d.Name()] = d
}

// ReplaceDriver upserts a driver in the registry (no panic on collision).
func ReplaceDriver(d Driver) {
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[d.Name()] = d
}

// LookupDriver returns the registered Driver by name or an error.
func LookupDriver(name string) (Driver, error) {
	driversMu.RLock()
	defer driversMu.RUnlock()
	d, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("sqltoolkit/db: driver %q not registered", name)
	}
	return d, nil
}

// OpenWithDriver opens a DB using a registered Driver and structured options,
// removing the need for manual DSN construction.
//
//	db, err := db.OpenWithDriver("pgx", db.DriverOptions{
//	    Host: "localhost", Port: 5432,
//	    User: "app", Password: "secret", Database: "appdb",
//	}, db.Config{MaxOpenConns: 25})
func OpenWithDriver(driverName string, driverOpts DriverOptions, cfg Config) (*DB, error) {
	drv, err := LookupDriver(driverName)
	if err != nil {
		return nil, err
	}
	drv.Register()

	dsn, err := drv.DSN(driverOpts)
	if err != nil {
		return nil, fmt.Errorf("sqltoolkit/db: DSN construction failed: %w", err)
	}

	cfg.DriverName = drv.Name()
	cfg.DSN = dsn

	db, err := Open(cfg)
	if err != nil {
		return nil, err
	}

	// Install the driver-specific error mapper.
	db.SetErrorMapper(ChainMapper(drv.ErrorMapper(), DefaultErrorMapper()))
	return db, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// PostgreSQL driver adapter (lib/pq)
// ─────────────────────────────────────────────────────────────────────────────

// PostgresDriver is the built-in lib/pq adapter.
// Import _ "github.com/lib/pq" alongside this to activate.
type PostgresDriver struct{}

func (PostgresDriver) Name() string { return "postgres" }

func (PostgresDriver) DSN(o DriverOptions) (string, error) {
	if o.Host == "" || o.Database == "" {
		return "", fmt.Errorf("postgres driver: Host and Database are required")
	}
	port := o.Port
	if port == 0 {
		port = 5432
	}
	sslMode := o.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		o.Host, port, o.User, o.Password, o.Database, sslMode,
	)
	for k, v := range o.Extra {
		dsn += fmt.Sprintf(" %s=%s", k, v)
	}
	return dsn, nil
}

func (PostgresDriver) ErrorMapper() ErrorMapper { return DefaultErrorMapper() }
func (PostgresDriver) Register()                { /* lib/pq self-registers via its init() */ }

// ─────────────────────────────────────────────────────────────────────────────
// MySQL driver adapter
// ─────────────────────────────────────────────────────────────────────────────

// MySQLDriver is the built-in go-sql-driver/mysql adapter.
type MySQLDriver struct{}

func (MySQLDriver) Name() string { return "mysql" }

func (MySQLDriver) DSN(o DriverOptions) (string, error) {
	if o.Host == "" || o.Database == "" {
		return "", fmt.Errorf("mysql driver: Host and Database are required")
	}
	port := o.Port
	if port == 0 {
		port = 3306
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		o.User, o.Password, o.Host, port, o.Database)
	for k, v := range o.Extra {
		dsn += fmt.Sprintf("&%s=%s", k, v)
	}
	return dsn, nil
}

func (MySQLDriver) ErrorMapper() ErrorMapper { return DefaultErrorMapper() }
func (MySQLDriver) Register()                { /* go-sql-driver/mysql self-registers */ }

// ─────────────────────────────────────────────────────────────────────────────
// SQLite driver adapter
// ─────────────────────────────────────────────────────────────────────────────

// SQLiteDriver is the built-in mattn/go-sqlite3 adapter.
type SQLiteDriver struct{}

func (SQLiteDriver) Name() string { return "sqlite3" }

func (SQLiteDriver) DSN(o DriverOptions) (string, error) {
	if o.Database == "" {
		return "", fmt.Errorf("sqlite3 driver: Database (file path) is required")
	}
	dsn := o.Database
	first := true
	for k, v := range o.Extra {
		if first {
			dsn += "?"
			first = false
		} else {
			dsn += "&"
		}
		dsn += k + "=" + v
	}
	return dsn, nil
}

func (SQLiteDriver) ErrorMapper() ErrorMapper { return DefaultErrorMapper() }
func (SQLiteDriver) Register()                { /* mattn/go-sqlite3 self-registers */ }

// ─────────────────────────────────────────────────────────────────────────────
// Auto-register built-in drivers at init time
// ─────────────────────────────────────────────────────────────────────────────

func init() {
	// Built-in drivers are registered lazily to avoid import errors when
	// the corresponding driver package is not in the module graph.
	// The actual sql.Register calls happen in their respective packages'
	// init() functions when imported.
	safeRegister(PostgresDriver{})
	safeRegister(MySQLDriver{})
	safeRegister(SQLiteDriver{})
}

func safeRegister(d Driver) {
	defer func() { recover() }() // swallow duplicate registration panics
	RegisterDriver(d)
}

// ─────────────────────────────────────────────────────────────────────────────
// DSNFromEnv — convenience helper for twelve-factor apps
// ─────────────────────────────────────────────────────────────────────────────

// DSNFromEnv looks up the DATABASE_URL environment variable (standard for
// Heroku / Render / Railway / Fly.io) and returns it as a DSN.
// It does NOT modify cfg; callers should set cfg.DSN = dsn before calling Open.
func DSNFromEnv() (string, error) {
	dsn := envOrEmpty("DATABASE_URL")
	if dsn == "" {
		return "", fmt.Errorf("sqltoolkit/db: DATABASE_URL environment variable not set")
	}
	return dsn, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Verify database/sql import at compile time
// ─────────────────────────────────────────────────────────────────────────────

var _ *sql.DB // ensure database/sql is imported
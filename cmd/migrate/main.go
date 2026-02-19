package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fatalf("DATABASE_URL environment variable is required")
	}

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "./migrations"
	}

	m, err := migrate.New("file://"+migrationsPath, dbURL)
	if err != nil {
		fatalf("migration init failed: %v", err)
	}
	defer m.Close()

	m.Log = &migrateLogger{}

	command := args[0]
	switch command {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			fatalf("up failed: %v", err)
		}
		slog.Info("migrations: up completed")

	case "down":
		steps := 1
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 {
				fatalf("down: invalid steps argument %q", args[1])
			}
			steps = n
		}
		if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			fatalf("down failed: %v", err)
		}
		slog.Info("migrations: down completed", "steps", steps)

	case "version":
		v, dirty, err := m.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			fatalf("version failed: %v", err)
		}
		fmt.Printf("version: %d  dirty: %v\n", v, dirty)

	case "force":
		if len(args) < 2 {
			fatalf("force: version argument required")
		}
		v, err := strconv.Atoi(args[1])
		if err != nil {
			fatalf("force: invalid version %q", args[1])
		}
		if err := m.Force(v); err != nil {
			fatalf("force failed: %v", err)
		}
		slog.Info("migrations: forced", "version", v)

	case "drop":
		fmt.Fprintln(os.Stderr, "WARNING: drop will destroy all tables. Type 'yes' to confirm:")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("aborted")
			os.Exit(0)
		}
		if err := m.Drop(); err != nil {
			fatalf("drop failed: %v", err)
		}
		slog.Info("migrations: all tables dropped")

	default:
		usage()
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────────────────────────────────────

type migrateLogger struct{}

func (l *migrateLogger) Printf(format string, v ...any) {
	slog.Info(fmt.Sprintf(format, v...))
}
func (l *migrateLogger) Verbose() bool { return false }

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: migrate <command> [args]

Commands:
  up           Apply all pending migrations
  down [N]     Rollback N migrations (default: 1)
  version      Print current migration version
  force <V>    Force set migration version (bypass dirty state)
  drop         Drop all tables (dev only)

Environment:
  DATABASE_URL      Required. Full database DSN.
  MIGRATIONS_PATH   Path to migrations directory (default: ./migrations)`)
}

func fatalf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
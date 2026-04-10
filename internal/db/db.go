package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the SQLite database at path, applies migrations,
// and returns the connection. The caller must call Close when done.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite is single-writer

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate applies all schema migrations in order. Migrations are idempotent
// (all use CREATE IF NOT EXISTS), so re-running is safe.
func Migrate(db *sql.DB) error {
	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// SetImportPragmas sets pragmas for fast bulk import.
// Call RestoreDefaultPragmas when done.
func SetImportPragmas(db *sql.DB) error {
	_, err := db.Exec(`PRAGMA synchronous=OFF; PRAGMA journal_mode=MEMORY;`)
	return err
}

// RestoreDefaultPragmas restores safe pragmas after import.
func RestoreDefaultPragmas(db *sql.DB) error {
	_, err := db.Exec(`PRAGMA synchronous=NORMAL; PRAGMA journal_mode=WAL;`)
	return err
}

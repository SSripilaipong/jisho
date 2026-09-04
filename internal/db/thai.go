package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// OpenThai creates the optional English-to-Thai store. It is separate from the
// Japanese database so a normal jisho update cannot discard the imported data.
func OpenThai(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	for _, stmt := range thaiMigrations {
		if _, err := d.Exec(stmt); err != nil {
			d.Close()
			return nil, fmt.Errorf("Thai schema: %w", err)
		}
	}
	return d, nil
}

var thaiMigrations = []string{
	`CREATE TABLE IF NOT EXISTS source_meta (
		key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS thai_entries (
		id INTEGER PRIMARY KEY,
		search_key TEXT NOT NULL,
		english_key TEXT NOT NULL,
		english TEXT NOT NULL,
		thai TEXT NOT NULL,
		part_of_speech TEXT NOT NULL,
		related TEXT NOT NULL,
		synonyms TEXT NOT NULL,
		antonyms TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_thai_search ON thai_entries(search_key)`,
	`CREATE INDEX IF NOT EXISTS idx_thai_english ON thai_entries(english_key)`,
}

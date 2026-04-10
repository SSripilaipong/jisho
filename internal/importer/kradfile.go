package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
)

// KradfileImporter imports the radical decomposition data into kanji_radicals.
// The jmdict-simplified kanjidic release includes a "radicals" section per character,
// but a dedicated KRADFILE JSON (krad.json / radkfile.json) maps kanji → radical list.
// Format: {"kanji": {"食": ["人","良",...], ...}}
type KradfileImporter struct{}

func (KradfileImporter) SourceKey() string { return "kradfile_version" }

func (KradfileImporter) Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error {
	pr := &progressReader{r: r, size: size, fn: progress}

	var root struct {
		Kanji   map[string][]string `json:"kanji"`
		Version string              `json:"version"`
	}
	if err := json.NewDecoder(pr).Decode(&root); err != nil {
		return fmt.Errorf("kradfile decode: %w", err)
	}

	type radRow struct {
		literal string
		radical string
	}

	b := newBatcher(db, 1000, func(tx *sql.Tx, rows []radRow) error {
		stmt, err := tx.Prepare(`INSERT OR IGNORE INTO kanji_radicals(literal, radical) VALUES (?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, row := range rows {
			if _, err := stmt.Exec(row.literal, row.radical); err != nil {
				return fmt.Errorf("insert radical %q/%q: %w", row.literal, row.radical, err)
			}
		}
		return nil
	})

	for literal, radicals := range root.Kanji {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for _, rad := range radicals {
			if err := b.add(radRow{literal: literal, radical: rad}); err != nil {
				return err
			}
		}
	}

	if err := b.flush(); err != nil {
		return err
	}

	if root.Version != "" {
		_, err := db.Exec(`INSERT INTO source_meta(key,value,updated_at) VALUES(?,?,datetime('now'))
			ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			"kradfile_version", root.Version)
		if err != nil {
			return fmt.Errorf("save version: %w", err)
		}
	}
	return nil
}

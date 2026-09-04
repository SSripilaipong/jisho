package importer

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/shsnail/jisho/internal/model"
)

// LexitronImporter reads NECTEC's UTF-8 etlex.csv (English to Thai).
type LexitronImporter struct{}

func (LexitronImporter) SourceKey() string { return "lexitron_version" }

func (LexitronImporter) Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error {
	c := csv.NewReader(r)
	header, err := c.Read()
	if err != nil {
		return fmt.Errorf("LEXiTRON header: %w", err)
	}
	columns := make(map[string]int)
	for i, name := range header {
		columns[strings.TrimPrefix(name, "\ufeff")] = i
	}
	for _, name := range []string{"id", "e-search", "e-entry", "t-entry", "e-cat", "t-related", "e-syn", "e-ant"} {
		if _, ok := columns[name]; !ok {
			return fmt.Errorf("LEXiTRON header missing %q", name)
		}
	}

	// A replacement is one transaction: bad input never leaves a partial store.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM thai_entries`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO thai_entries
		(id, search_key, english_key, english, thai, part_of_speech, related, synonyms, antonyms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		row, err := c.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("LEXiTRON CSV: %w", err)
		}
		for _, field := range row {
			if !utf8.ValidString(field) {
				return fmt.Errorf("LEXiTRON CSV is not UTF-8")
			}
		}
		get := func(name string) string { return strings.TrimSpace(row[columns[name]]) }
		id, err := strconv.Atoi(get("id"))
		if err != nil || id < 0 || get("e-entry") == "" {
			return fmt.Errorf("invalid LEXiTRON entry ID/headword: %q", get("id"))
		}
		// The official archive includes one untranslated entry (transformative).
		if get("t-entry") == "" {
			continue
		}
		_, err = stmt.ExecContext(ctx, id,
			model.NormalizeEnglish(get("e-search")), model.NormalizeEnglish(get("e-entry")),
			get("e-entry"), get("t-entry"), get("e-cat"), get("t-related"), get("e-syn"), get("e-ant"))
		if err != nil {
			return fmt.Errorf("LEXiTRON entry %d: %w", id, err)
		}
		count++
		if progress != nil && count%1000 == 0 {
			progress(c.InputOffset(), size)
		}
	}
	if count == 0 {
		return fmt.Errorf("LEXiTRON CSV contains no translated entries")
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO source_meta (key, value, updated_at)
		VALUES ('lexitron_version', '2.0', datetime('now'))`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if progress != nil {
		progress(c.InputOffset(), size)
	}
	return nil
}

package importer

import (
	"context"
	"database/sql"
	"io"
)

// Importer reads a data source and writes it to the database.
type Importer interface {
	// Import streams from r and inserts into db.
	// size is the total byte length for progress reporting (0 if unknown).
	// progress is called with (bytesRead, total); may be nil.
	Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error
	// SourceKey returns the metadata key stored in source_meta (e.g. "jmdict_version").
	SourceKey() string
}

// reverseRunes returns the rune-reversed form of s.
func reverseRunes(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

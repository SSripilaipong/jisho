package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const kradfileSample = `{
  "version": "3.6.2",
  "kanji": {
    "食": ["人", "良"],
    "𠮟": ["口", "七"]
  }
}`

func TestKradfileImportSkipsUnknownKanji(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, ddl := range []string{
		`CREATE TABLE kanji (literal TEXT PRIMARY KEY)`,
		`CREATE TABLE kanji_radicals (
			literal TEXT NOT NULL REFERENCES kanji(literal),
			radical TEXT NOT NULL,
			PRIMARY KEY (literal, radical))`,
		`CREATE TABLE source_meta (key TEXT PRIMARY KEY, value TEXT, updated_at TEXT)`,
		`INSERT INTO kanji(literal) VALUES ('食')`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}

	r := strings.NewReader(kradfileSample)
	// 𠮟 has no kanji row: it must be skipped, not abort the import.
	if err := (KradfileImporter{}).Import(context.Background(), db, r, int64(len(kradfileSample)), func(int64, int64) {}); err != nil {
		t.Fatalf("import: %v", err)
	}

	rows, err := db.Query(`SELECT literal, radical FROM kanji_radicals`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var l, rad string
		if err := rows.Scan(&l, &rad); err != nil {
			t.Fatal(err)
		}
		got = append(got, l+"/"+rad)
	}
	sort.Strings(got)

	want := []string{"食/人", "食/良"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("kanji_radicals = %v, want %v", got, want)
	}
}

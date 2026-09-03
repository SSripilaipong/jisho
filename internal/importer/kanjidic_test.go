package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const kanjidicSample = `{
  "version": "3.6.2",
  "characters": [
    {
      "literal": "食",
      "misc": {"grade": 2, "strokeCount": [9], "frequency": 328, "jlptLevel": 4},
      "radicals": [{"type": "classical", "value": 184}],
      "readingMeaning": {
        "groups": [
          {
            "readings": [
              {"type": "pinyin", "value": "shi2"},
              {"type": "ja_on", "value": "ショク"},
              {"type": "ja_on", "value": "ジキ"},
              {"type": "ja_kun", "value": "く.う"},
              {"type": "ja_kun", "value": "た.べる"}
            ],
            "meanings": [
              {"lang": "en", "value": "eat"},
              {"lang": "fr", "value": "manger"},
              {"lang": "en", "value": "food"}
            ]
          }
        ],
        "nanori": ["ぐい"]
      }
    }
  ]
}`

func TestKanjidicImportReadingsAndMeanings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE kanji (
		literal TEXT PRIMARY KEY, grade INTEGER, stroke_count INTEGER,
		frequency INTEGER, jlpt_level INTEGER, classical_radical INTEGER,
		nelson_radical INTEGER, on_readings TEXT, kun_readings TEXT,
		nanori TEXT, meanings_en TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE source_meta (key TEXT PRIMARY KEY, value TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}

	r := strings.NewReader(kanjidicSample)
	if err := (KanjidicImporter{}).Import(context.Background(), db, r, int64(len(kanjidicSample)), func(int64, int64) {}); err != nil {
		t.Fatalf("import: %v", err)
	}

	var on, kun, nanori, meanings string
	var jlpt int
	err = db.QueryRow(`SELECT on_readings, kun_readings, nanori, meanings_en, jlpt_level
		FROM kanji WHERE literal = '食'`).Scan(&on, &kun, &nanori, &meanings, &jlpt)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, got, want string }{
		{"on", on, `["ショク","ジキ"]`},
		{"kun", kun, `["く.う","た.べる"]`},
		{"nanori", nanori, `["ぐい"]`},
		{"meanings", meanings, `["eat","food"]`},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, tc.got, tc.want)
		}
	}
	if jlpt != 4 {
		t.Errorf("jlpt_level = %d, want 4", jlpt)
	}
}

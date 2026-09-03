package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// Mirrors the real file: a DTD with entity declarations, the created-date
// comment, and pos elements referencing those entities.
const jmdictXMLSample = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE JMdict [
<!ENTITY n "noun (common) (futsuumeishi)">
<!ENTITY ok "out-dated or obsolete kana usage">
]>
<!-- JMdict created: 2026-09-03 -->
<JMdict>
<entry>
<ent_seq>1000001</ent_seq>
<k_ele><keb>食料品</keb><ke_pri>ichi1</ke_pri><ke_pri>news1</ke_pri><ke_pri>nf05</ke_pri></k_ele>
<r_ele><reb>しょくりょうひん</reb><re_pri>nf05</re_pri></r_ele>
<sense><pos>&n;</pos><gloss>foodstuff</gloss></sense>
</entry>
<entry>
<ent_seq>1000002</ent_seq>
<k_ele><keb>食材</keb><ke_pri>ichi1</ke_pri></k_ele>
<r_ele><reb>しょくざい</reb><re_inf>&ok;</re_inf></r_ele>
<sense><pos>&n;</pos><gloss>ingredient</gloss></sense>
</entry>
<entry>
<ent_seq>1000003</ent_seq>
<k_ele><keb>食蟻獣</keb></k_ele>
<r_ele><reb>しょくぎじゅう</reb></r_ele>
<sense><pos>&n;</pos><gloss>anteater</gloss></sense>
</entry>
<entry>
<ent_seq>1000004</ent_seq>
<k_ele><keb>二千</keb><ke_pri>news2</ke_pri></k_ele>
<r_ele><reb>にせん</reb></r_ele>
<sense><pos>&n;</pos><gloss>two thousand</gloss></sense>
</entry>
<entry>
<ent_seq>9999999</ent_seq>
<k_ele><keb>知らない</keb><ke_pri>nf01</ke_pri></k_ele>
<r_ele><reb>しらない</reb></r_ele>
<sense><pos>&n;</pos><gloss>absent from the simplified build</gloss></sense>
</entry>
</JMdict>`

func TestJMdictPriorityImport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE words (id TEXT PRIMARY KEY, freq_rank INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE source_meta (key TEXT PRIMARY KEY, value TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1000001", "1000002", "1000003", "1000004"} {
		if _, err := db.Exec(`INSERT INTO words(id) VALUES (?)`, id); err != nil {
			t.Fatal(err)
		}
	}

	r := strings.NewReader(jmdictXMLSample)
	if err := (JMdictPriorityImporter{}).Import(context.Background(), db, r, int64(len(jmdictXMLSample)), func(int64, int64) {}); err != nil {
		t.Fatalf("import: %v", err)
	}

	for _, tc := range []struct {
		id   string
		want sql.NullInt64
		why  string
	}{
		{"1000001", sql.NullInt64{Int64: 5, Valid: true}, "nf05 band beats the ichi1/news1 markers"},
		{"1000002", sql.NullInt64{Int64: rankPrimaryMarker, Valid: true}, "ichi1 with no band"},
		{"1000003", sql.NullInt64{}, "no priority markers at all"},
		{"1000004", sql.NullInt64{Int64: rankSecondaryMarker, Valid: true}, "news2 only"},
	} {
		var got sql.NullInt64
		if err := db.QueryRow(`SELECT freq_rank FROM words WHERE id = ?`, tc.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("freq_rank[%s] = %v, want %v (%s)", tc.id, got, tc.want, tc.why)
		}
	}

	// An entry the simplified build does not have must not create a row.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("words count = %d, want 4 (unknown ent_seq must be a no-op)", count)
	}

	var version string
	if err := db.QueryRow(`SELECT value FROM source_meta WHERE key = 'jmdict_priority_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "2026-09-03" {
		t.Errorf("version = %q, want %q", version, "2026-09-03")
	}
}

package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jishodb "github.com/shsnail/jisho/internal/db"
	"github.com/shsnail/jisho/internal/importer"
	"github.com/shsnail/jisho/internal/query"
)

const thaiCSV = "\ufeffid,e-search,e-entry,t-entry,e-cat,t-related,e-syn,e-ant\r\n" +
	"0,bank,bank,ธนาคาร,N,,,\r\n" +
	"1,bank,bank,ฝั่ง,N,,,\r\n" +
	"2,take care of,take care of,ดูแล,PHRV,\"เอาใจใส่, ระวัง\",,\r\n" +
	"3,abbreviation,A.B.,คำย่อ,ABBR,,,\r\n" +
	"4,empty,empty,,ADJ,,,\r\n"

func thaiArchive(t *testing.T, csv string, licenses bool) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"etlex.csv": csv}
	if licenses {
		files["LICENSE.txt"] = "test license"
		files["LICENSE-th.txt"] = "test Thai license"
	}
	for name, text := range files {
		w, err := z.Create("LEXiTRON_2.0_csv/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestThaiImportLookupAndMetadata(t *testing.T) {
	p := filepath.Join(t.TempDir(), "thai.db")
	count, err := installThaiArchive(context.Background(), p, thaiArchive(t, thaiCSV, true))
	if err != nil || count != 4 {
		t.Fatalf("install count=%d err=%v", count, err)
	}
	d, err := jishodb.OpenThai(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, tc := range []struct {
		input string
		count int
		thai  string
	}{
		{" BANK ", 2, "ธนาคาร"},
		{"take\n care\t of", 1, "ดูแล"},
		{"a.b.", 1, "คำย่อ"},
		{"abbreviation", 1, "คำย่อ"},
		{"ban", 0, ""},
		{"take care of someone", 0, ""},
		{"empty", 0, ""},
		{"' OR 1=1 --", 0, ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			entries, err := query.LookupThai(context.Background(), d, tc.input)
			if err != nil || len(entries) != tc.count {
				t.Fatalf("lookup count=%d err=%v", len(entries), err)
			}
			if tc.count > 0 && entries[0].Thai != tc.thai {
				t.Fatalf("Thai=%q want=%q", entries[0].Thai, tc.thai)
			}
		})
	}
	var license, checksum string
	if err := d.QueryRow(`SELECT value FROM source_meta WHERE key='lexitron_license'`).Scan(&license); err != nil || license != "test license" {
		t.Fatalf("license=%q err=%v", license, err)
	}
	if err := d.QueryRow(`SELECT value FROM source_meta WHERE key='lexitron_sha256'`).Scan(&checksum); err != nil || len(checksum) != 64 {
		t.Fatalf("checksum=%q err=%v", checksum, err)
	}
}

func TestThaiFailedInstallPreservesDatabase(t *testing.T) {
	p := filepath.Join(t.TempDir(), "thai.db")
	good := thaiArchive(t, thaiCSV, true)
	if _, err := installThaiArchive(context.Background(), p, good); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{
		[]byte("not a zip"),
		thaiArchive(t, thaiCSV, false),
		thaiArchive(t, thaiCSV+"broken,row\n", true),
		thaiArchive(t, "id,e-entry\n0,test\n", true),
		thaiArchive(t, strings.Split(thaiCSV, "\r\n")[0]+"\r\n", true),
	} {
		if _, err := installThaiArchive(context.Background(), p, bad); err == nil {
			t.Fatal("bad archive accepted")
		}
		after, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("existing database changed: %v", err)
		}
	}
	// Re-importing replaces the store instead of duplicating senses.
	if n, err := installThaiArchive(context.Background(), p, good); err != nil || n != 4 {
		t.Fatalf("reinstall count=%d err=%v", n, err)
	}
}

func TestThaiImporterRollsBackMalformedReplacement(t *testing.T) {
	d, err := jishodb.OpenThai(filepath.Join(t.TempDir(), "thai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	imp := importer.LexitronImporter{}
	if err := imp.Import(context.Background(), d, strings.NewReader(thaiCSV), 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := imp.Import(context.Background(), d, strings.NewReader(thaiCSV+"broken,row\n"), 0, nil); err == nil {
		t.Fatal("malformed replacement accepted")
	}
	entries, err := query.LookupThai(context.Background(), d, "bank")
	if err != nil || len(entries) != 2 {
		t.Fatalf("old entries lost: %v, %v", entries, err)
	}
}

func TestThaiLookupWithoutJapaneseDatabase(t *testing.T) {
	previous := dbPath
	dbPath = filepath.Join(t.TempDir(), "dictionary.db")
	t.Cleanup(func() { dbPath = previous })
	var out bytes.Buffer
	if err := lookupThaiTo(context.Background(), &out, "bank"); err == nil || !strings.Contains(err.Error(), "`jisho update`") {
		t.Fatalf("missing database message: %v", err)
	}
	if _, err := installThaiArchive(context.Background(), resolveThaiDBPath(), thaiArchive(t, thaiCSV, true)); err != nil {
		t.Fatal(err)
	}
	if err := lookupThaiTo(context.Background(), &out, "bank"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ธนาคาร") || !strings.Contains(out.String(), "ฝั่ง") {
		t.Fatalf("missing senses: %s", out.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("Japanese database unexpectedly created: %v", err)
	}
}

package query

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	jishodb "github.com/shsnail/jisho/internal/db"
)

// testWord is one fixture entry: a word plus the forms it is searchable by.
type testWord struct {
	id       string
	form     string
	isCommon bool
	jlpt     int
	isKana   bool
	freqRank int // 0 = no priority marker in JMdict (stored as NULL)
}

func newTestQuerier(t *testing.T, words []testWord) Querier {
	t.Helper()
	db, err := jishodb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, w := range words {
		common := 0
		if w.isCommon {
			common = 1
		}
		var jlpt any
		if w.jlpt > 0 {
			jlpt = w.jlpt
		}
		var freqRank any
		if w.freqRank > 0 {
			freqRank = w.freqRank
		}
		if _, err := db.Exec(
			`INSERT INTO words(id, kanji_json, kana_json, sense_json, gloss_en, is_common, jlpt_level, freq_rank)
			 VALUES (?, '[]', '[]', '[]', '', ?, ?, ?)`, w.id, common, jlpt, freqRank); err != nil {
			t.Fatal(err)
		}
		kana := 0
		if w.isKana {
			kana = 1
		}
		if _, err := db.Exec(
			`INSERT INTO word_forms(word_id, form, form_rev, is_kana) VALUES (?,?,?,?)`,
			w.id, w.form, reverseRunes(w.form), kana); err != nil {
			t.Fatal(err)
		}
	}
	return New(db)
}

func idsOf(t *testing.T, q Querier, raw string) []string {
	t.Helper()
	got, err := q.SearchWords(context.Background(), raw, SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, w := range got {
		ids = append(ids, w.ID)
	}
	return ids
}

func TestSearchWordsOrdering(t *testing.T) {
	q := newTestQuerier(t, []testWord{
		{id: "long-common", form: "食べ物屋さん", isCommon: true},
		{id: "exact-rare", form: "食べ", isCommon: false},
		{id: "short-common", form: "食べ物", isCommon: true},
		{id: "long-rare", form: "食べ歩きする", isCommon: false},
	})

	// Exact match wins even when uncommon; then common before rare;
	// then the shorter form first.
	want := []string{"exact-rare", "short-common", "long-common", "long-rare"}
	if got := idsOf(t, q, "食べ"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("prefix search = %v, want %v", got, want)
	}
}

func TestSearchWordsOrderingKana(t *testing.T) {
	q := newTestQuerier(t, []testWord{
		{id: "longer", form: "たべものや", isCommon: true, isKana: true},
		{id: "hiragana-exact", form: "たべ", isCommon: false, isKana: true},
		{id: "katakana-exact", form: "タベ", isCommon: true, isKana: true},
	})

	// Romaji matches both kana scripts; the katakana form is an exact match
	// too and is common, so it leads.
	want := []string{"katakana-exact", "hiragana-exact", "longer"}
	if got := idsOf(t, q, "tabe"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("romaji search = %v, want %v", got, want)
	}
}

func TestSearchWordsOrderingSuffix(t *testing.T) {
	q := newTestQuerier(t, []testWord{
		{id: "long", form: "立ち食べる", isCommon: true},
		{id: "exact", form: "食べる", isCommon: false},
	})

	want := []string{"exact", "long"}
	if got := idsOf(t, q, "*食べる"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("suffix search = %v, want %v", got, want)
	}
}

func TestSearchWordsOrderingFrequencyBeatsLength(t *testing.T) {
	// The 食料品 / 食料 shape from jisho.org: the longer form is the more
	// frequent one, so frequency must outrank the length tie-break.
	q := newTestQuerier(t, []testWord{
		{id: "short-unranked", form: "食料", isCommon: true},
		{id: "long-ranked", form: "食料品", isCommon: true, freqRank: 16},
		{id: "short-ranked", form: "食堂", isCommon: true, freqRank: 5},
	})

	want := []string{"short-ranked", "long-ranked", "short-unranked"}
	if got := idsOf(t, q, "食"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("prefix search = %v, want %v", got, want)
	}
}

package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// JMdictImporter imports jmdict-simplified JSON into the words + word_forms tables.
type JMdictImporter struct{}

func (JMdictImporter) SourceKey() string { return "jmdict_version" }

func (JMdictImporter) Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error {
	pr := &progressReader{r: r, size: size, fn: progress}
	dec := json.NewDecoder(pr)

	var version string

	// Walk to the "words" array, capturing version along the way.
	if err := advanceToArray(dec, "words", func(key, val string) {
		if key == "version" {
			version = val
		}
	}); err != nil {
		return fmt.Errorf("jmdict: %w", err)
	}

	type formRow struct {
		wordID  string
		form    string
		formRev string
		isKana  int
	}
	type wordRow struct {
		id        string
		kanjiJSON string
		kanaJSON  string
		senseJSON string
		glossEn   string
		isCommon  int
		jlptLevel *int
		forms     []formRow
	}

	b := newBatcher(db, 500, func(tx *sql.Tx, rows []wordRow) error {
		wStmt, err := tx.Prepare(`INSERT OR REPLACE INTO words
			(id, kanji_json, kana_json, sense_json, gloss_en, is_common, jlpt_level)
			VALUES (?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer wStmt.Close()

		fStmt, err := tx.Prepare(`INSERT OR REPLACE INTO word_forms
			(word_id, form, form_rev, is_kana) VALUES (?,?,?,?)`)
		if err != nil {
			return err
		}
		defer fStmt.Close()

		for _, row := range rows {
			if _, err := wStmt.Exec(row.id, row.kanjiJSON, row.kanaJSON, row.senseJSON,
				row.glossEn, row.isCommon, row.jlptLevel); err != nil {
				return fmt.Errorf("insert word %s: %w", row.id, err)
			}
			for _, f := range row.forms {
				if _, err := fStmt.Exec(f.wordID, f.form, f.formRev, f.isKana); err != nil {
					return fmt.Errorf("insert form %q: %w", f.form, err)
				}
			}
		}
		return nil
	})

	for dec.More() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var w jmdictWord
		if err := dec.Decode(&w); err != nil {
			return fmt.Errorf("jmdict decode: %w", err)
		}

		// Build flat text fields.
		kanjiJSON, _ := json.Marshal(w.Kanji)
		kanaJSON, _ := json.Marshal(w.Kana)
		senseJSON, _ := json.Marshal(w.Sense)

		isCommon := 0
		for _, k := range w.Kanji {
			if k.Common {
				isCommon = 1
				break
			}
		}
		if isCommon == 0 {
			for _, k := range w.Kana {
				if k.Common {
					isCommon = 1
					break
				}
			}
		}

		jlptLevel := extractJLPT(w.Kana, w.Kanji)

		var glossParts []string
		for _, s := range w.Sense {
			for _, g := range s.Gloss {
				if g.Lang == "eng" {
					glossParts = append(glossParts, g.Text)
				}
			}
		}
		glossEn := strings.Join(glossParts, " ")

		var forms []formRow
		for _, k := range w.Kanji {
			if k.Text != "" {
				forms = append(forms, formRow{
					wordID: w.ID, form: k.Text, formRev: reverseRunes(k.Text), isKana: 0,
				})
			}
		}
		for _, k := range w.Kana {
			if k.Text != "" {
				forms = append(forms, formRow{
					wordID: w.ID, form: k.Text, formRev: reverseRunes(k.Text), isKana: 1,
				})
			}
		}

		row := wordRow{
			id:        w.ID,
			kanjiJSON: string(kanjiJSON),
			kanaJSON:  string(kanaJSON),
			senseJSON: string(senseJSON),
			glossEn:   glossEn,
			isCommon:  isCommon,
			jlptLevel: jlptLevel,
			forms:     forms,
		}
		if err := b.add(row); err != nil {
			return err
		}
	}

	if err := b.flush(); err != nil {
		return err
	}

	if version != "" {
		_, err := db.Exec(`INSERT INTO source_meta(key,value,updated_at) VALUES(?,?,datetime('now'))
			ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			"jmdict_version", version)
		if err != nil {
			return fmt.Errorf("save version: %w", err)
		}
	}
	return nil
}

// extractJLPT finds the highest (most common) JLPT level from tags.
// Tags are like "jlpt-n1" … "jlpt-n5". Lower number = harder = more advanced.
// We store 1–5 where 5 = N5 (easiest). Returns nil if no JLPT tag found.
func extractJLPT(kana []jmdictKana, kanji []jmdictKanji) *int {
	best := 0
	check := func(tags []string) {
		for _, t := range tags {
			switch t {
			case "jlpt-n1":
				if best == 0 || 1 < best {
					best = 1
				}
			case "jlpt-n2":
				if best == 0 || 2 < best {
					best = 2
				}
			case "jlpt-n3":
				if best == 0 || 3 < best {
					best = 3
				}
			case "jlpt-n4":
				if best == 0 || 4 < best {
					best = 4
				}
			case "jlpt-n5":
				if best == 0 || 5 < best {
					best = 5
				}
			}
		}
	}
	for _, k := range kana {
		check(k.Tags)
	}
	for _, k := range kanji {
		check(k.Tags)
	}
	if best == 0 {
		return nil
	}
	return &best
}

// --- JSON types for streaming parse ---

type jmdictWord struct {
	ID    string        `json:"id"`
	Kanji []jmdictKanji `json:"kanji"`
	Kana  []jmdictKana  `json:"kana"`
	Sense []jmdictSense `json:"sense"`
}

type jmdictKanji struct {
	Text   string   `json:"text"`
	Common bool     `json:"common"`
	Tags   []string `json:"tags"`
}

type jmdictKana struct {
	Text           string   `json:"text"`
	Common         bool     `json:"common"`
	Tags           []string `json:"tags"`
	AppliesToKanji []string `json:"appliesToKanji"`
}

type jmdictSense struct {
	Gloss          []jmdictGloss `json:"gloss"`
	PartOfSpeech   []string      `json:"partOfSpeech"`
	Field          []string      `json:"field"`
	Misc           []string      `json:"misc"`
	Dialect        []string      `json:"dialect"`
	Info           []string      `json:"info"`
	AppliesToKanji []string      `json:"appliesToKanji"`
	AppliesToKana  []string      `json:"appliesToKana"`
}

type jmdictGloss struct {
	Lang string  `json:"lang"`
	Text string  `json:"text"`
	Type *string `json:"type"`
}

// --- Streaming helpers ---

// advanceToArray advances the decoder to the named JSON array key.
// onScalar is called for each string scalar field encountered before the array.
func advanceToArray(dec *json.Decoder, arrayKey string, onScalar func(key, val string)) error {
	// Consume opening '{'
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read opening brace: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected '{', got %v", tok)
	}

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("read key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %T", keyTok)
		}
		if key == arrayKey {
			// Consume the opening '[' of the array.
			arrTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("read array open: %w", err)
			}
			if d, ok := arrTok.(json.Delim); !ok || d != '[' {
				return fmt.Errorf("expected '[' for %q, got %v", arrayKey, arrTok)
			}
			return nil
		}
		// Skip or capture this value.
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("skip field %q: %w", key, err)
		}
		if onScalar != nil {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				onScalar(key, s)
			}
		}
	}
	return fmt.Errorf("array key %q not found", arrayKey)
}

// progressReader wraps an io.Reader and reports bytes read.
type progressReader struct {
	r    io.Reader
	size int64
	read int64
	fn   func(int64, int64)
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	if pr.fn != nil && n > 0 {
		pr.read += int64(n)
		pr.fn(pr.read, pr.size)
	}
	return
}

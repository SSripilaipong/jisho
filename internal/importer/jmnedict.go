package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// JMnedictImporter imports jmnedict-simplified JSON into the names + name_forms tables.
type JMnedictImporter struct{}

func (JMnedictImporter) SourceKey() string { return "jmnedict_version" }

func (JMnedictImporter) Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error {
	pr := &progressReader{r: r, size: size, fn: progress}
	dec := json.NewDecoder(pr)

	var version string
	if err := advanceToArray(dec, "words", func(key, val string) {
		if key == "version" {
			version = val
		}
	}); err != nil {
		return fmt.Errorf("jmnedict: %w", err)
	}

	type nameFormRow struct {
		nameID  string
		form    string
		formRev string
		isKana  int
	}
	type nameRow struct {
		id              string
		kanjiJSON       string
		kanaJSON        string
		translationJSON string
		translationEn   string
		forms           []nameFormRow
	}

	b := newBatcher(db, 500, func(tx *sql.Tx, rows []nameRow) error {
		nStmt, err := tx.Prepare(`INSERT OR REPLACE INTO names
			(id, kanji_json, kana_json, translation_json, translation_en)
			VALUES (?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer nStmt.Close()

		fStmt, err := tx.Prepare(`INSERT OR REPLACE INTO name_forms
			(name_id, form, form_rev, is_kana) VALUES (?,?,?,?)`)
		if err != nil {
			return err
		}
		defer fStmt.Close()

		for _, row := range rows {
			if _, err := nStmt.Exec(row.id, row.kanjiJSON, row.kanaJSON,
				row.translationJSON, row.translationEn); err != nil {
				return fmt.Errorf("insert name %s: %w", row.id, err)
			}
			for _, f := range row.forms {
				if _, err := fStmt.Exec(f.nameID, f.form, f.formRev, f.isKana); err != nil {
					return fmt.Errorf("insert name_form %q: %w", f.form, err)
				}
			}
		}
		return nil
	})

	for dec.More() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var w jmnedictWord
		if err := dec.Decode(&w); err != nil {
			return fmt.Errorf("jmnedict decode: %w", err)
		}

		kanjiJSON, _ := json.Marshal(w.Kanji)
		kanaJSON, _ := json.Marshal(w.Kana)
		translJSON, _ := json.Marshal(w.Translation)

		var transParts []string
		for _, t := range w.Translation {
			for _, g := range t.Translation {
				if g.Lang == "eng" || g.Lang == "en" {
					transParts = append(transParts, g.Text)
				}
			}
		}
		transEn := strings.Join(transParts, " ")

		var forms []nameFormRow
		for _, k := range w.Kanji {
			if k.Text != "" {
				forms = append(forms, nameFormRow{
					nameID: w.ID, form: k.Text, formRev: reverseRunes(k.Text), isKana: 0,
				})
			}
		}
		for _, k := range w.Kana {
			if k.Text != "" {
				forms = append(forms, nameFormRow{
					nameID: w.ID, form: k.Text, formRev: reverseRunes(k.Text), isKana: 1,
				})
			}
		}

		row := nameRow{
			id:              w.ID,
			kanjiJSON:       string(kanjiJSON),
			kanaJSON:        string(kanaJSON),
			translationJSON: string(translJSON),
			translationEn:   transEn,
			forms:           forms,
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
			"jmnedict_version", version)
		if err != nil {
			return fmt.Errorf("save version: %w", err)
		}
	}
	return nil
}

// --- JSON types ---

type jmnedictWord struct {
	ID          string           `json:"id"`
	Kanji       []jmnedictKanji  `json:"kanji"`
	Kana        []jmnedictKana   `json:"kana"`
	Translation []jmnedictTrans  `json:"translation"`
}

type jmnedictKanji struct {
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

type jmnedictKana struct {
	Text           string   `json:"text"`
	Tags           []string `json:"tags"`
	AppliesToKanji []string `json:"appliesToKanji"`
}

type jmnedictTrans struct {
	Type        []string       `json:"type"`
	Translation []jmnedictGloss `json:"translation"`
}

type jmnedictGloss struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

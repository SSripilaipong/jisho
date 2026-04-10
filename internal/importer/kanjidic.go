package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
)

// KanjidicImporter imports kanjidic2-simplified JSON into the kanji table.
type KanjidicImporter struct{}

func (KanjidicImporter) SourceKey() string { return "kanjidic_version" }

func (KanjidicImporter) Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error {
	pr := &progressReader{r: r, size: size, fn: progress}
	dec := json.NewDecoder(pr)

	var version string
	if err := advanceToArray(dec, "characters", func(key, val string) {
		if key == "version" {
			version = val
		}
	}); err != nil {
		return fmt.Errorf("kanjidic: %w", err)
	}

	type kanjiRow struct {
		literal          string
		grade            *int
		strokeCount      *int
		frequency        *int
		jlptLevel        *int
		classicalRadical *int
		nelsonRadical    *int
		onReadings       string
		kunReadings      string
		nanori           string
		meaningsEN       string
	}

	b := newBatcher(db, 200, func(tx *sql.Tx, rows []kanjiRow) error {
		stmt, err := tx.Prepare(`INSERT OR REPLACE INTO kanji
			(literal, grade, stroke_count, frequency, jlpt_level,
			 classical_radical, nelson_radical,
			 on_readings, kun_readings, nanori, meanings_en)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, row := range rows {
			if _, err := stmt.Exec(
				row.literal, row.grade, row.strokeCount, row.frequency, row.jlptLevel,
				row.classicalRadical, row.nelsonRadical,
				row.onReadings, row.kunReadings, row.nanori, row.meaningsEN,
			); err != nil {
				return fmt.Errorf("insert kanji %q: %w", row.literal, err)
			}
		}
		return nil
	})

	for dec.More() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var c kanjidicChar
		if err := dec.Decode(&c); err != nil {
			return fmt.Errorf("kanjidic decode: %w", err)
		}

		var onR, kunR []string
		if c.ReadingMeaning != nil {
			for _, rd := range c.ReadingMeaning.Readings {
				switch rd.Type {
				case "ja_on":
					onR = append(onR, rd.Value)
				case "ja_kun":
					kunR = append(kunR, rd.Value)
				}
			}
		}

		var meansEN []string
		if c.ReadingMeaning != nil {
			for _, m := range c.ReadingMeaning.Meanings {
				if m.Lang == "en" {
					meansEN = append(meansEN, m.Value)
				}
			}
		}

		onJSON, _ := json.Marshal(onR)
		kunJSON, _ := json.Marshal(kunR)
		var nanori []string
		if c.ReadingMeaning != nil {
			nanori = c.ReadingMeaning.Nanori
		}
		nanoriJSON, _ := json.Marshal(nanori)
		meanJSON, _ := json.Marshal(meansEN)

		var strokeCount *int
		if len(c.Misc.StrokeCount) > 0 {
			strokeCount = &c.Misc.StrokeCount[0]
		}

		var classicalRad, nelsonRad *int
		for _, r := range c.Radicals {
			v := r.Value
			switch r.Type {
			case "classical":
				classicalRad = &v
			case "nelson_c":
				nelsonRad = &v
			}
		}

		row := kanjiRow{
			literal:          c.Literal,
			grade:            c.Misc.Grade,
			strokeCount:      strokeCount,
			frequency:        c.Misc.Frequency,
			jlptLevel:        c.Misc.JLPTLevel,
			classicalRadical: classicalRad,
			nelsonRadical:    nelsonRad,
			onReadings:       string(onJSON),
			kunReadings:      string(kunJSON),
			nanori:           string(nanoriJSON),
			meaningsEN:       string(meanJSON),
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
			"kanjidic_version", version)
		if err != nil {
			return fmt.Errorf("save version: %w", err)
		}
	}
	return nil
}

// --- JSON types ---

type kanjidicChar struct {
	Literal        string            `json:"literal"`
	Misc           kanjidicMisc      `json:"misc"`
	Radicals       []kanjidicRadical `json:"radicals"`
	ReadingMeaning *kanjidicRM       `json:"readingMeaning"`
}

type kanjidicMisc struct {
	Grade       *int  `json:"grade"`
	StrokeCount []int `json:"strokeCount"`
	Frequency   *int  `json:"frequency"`
	JLPTLevel   *int  `json:"jlptLevel"`
}

// kanjidicRadical is one entry in the radicals array: {type: "classical"|"nelson_c", value: N}
type kanjidicRadical struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
}

type kanjidicRM struct {
	Readings []kanjidicReading `json:"readings"`
	Meanings []kanjidicMeaning `json:"meanings"`
	Nanori   []string          `json:"nanori"`
}

type kanjidicReading struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type kanjidicMeaning struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

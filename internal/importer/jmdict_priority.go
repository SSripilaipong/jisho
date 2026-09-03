package importer

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// JMdictPriorityImporter reads the original JMdict XML and records a frequency
// rank per entry.
//
// jmdict-simplified — the source of every other word field — collapses JMdict's
// ke_pri/re_pri markers into a single "common" boolean, which leaves no way to
// order the common words against each other. The XML keeps the underlying
// news-frequency bands, so this importer reads only <ent_seq> and the priority
// markers and writes words.freq_rank. Entry ids are shared between the two
// sources, so no other data needs to come from here.
type JMdictPriorityImporter struct{}

func (JMdictPriorityImporter) SourceKey() string { return "jmdict_priority_version" }

func (JMdictPriorityImporter) Import(ctx context.Context, db *sql.DB, r io.Reader, size int64, progress func(int64, int64)) error {
	pr := &progressReader{r: r, size: size, fn: progress}
	dec := xml.NewDecoder(pr)
	// JMdict declares its part-of-speech tags as DTD entities (<pos>&n;</pos>).
	// encoding/xml does not process DTDs and errors on them in strict mode;
	// non-strict passes them through as text, which is fine — every element
	// except ent_seq/ke_pri/re_pri is discarded.
	dec.Strict = false

	type prioRow struct {
		id   string
		rank int
	}

	b := newBatcher(db, 500, func(tx *sql.Tx, rows []prioRow) error {
		stmt, err := tx.Prepare(`UPDATE words SET freq_rank = ? WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, row := range rows {
			// Entries missing from the simplified build update nothing.
			if _, err := stmt.Exec(row.rank, row.id); err != nil {
				return fmt.Errorf("set freq_rank for %q: %w", row.id, err)
			}
		}
		return nil
	})

	var version string
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("jmdict xml: %w", err)
		}

		switch t := tok.(type) {
		case xml.Comment:
			if v := parseCreatedDate(string(t)); v != "" {
				version = v
			}
		case xml.StartElement:
			if t.Name.Local != "entry" {
				continue
			}
			var e jmdictXMLEntry
			if err := dec.DecodeElement(&e, &t); err != nil {
				return fmt.Errorf("jmdict xml decode: %w", err)
			}
			rank, ok := priorityRank(e)
			if !ok {
				continue
			}
			if err := b.add(prioRow{id: strings.TrimSpace(e.EntSeq), rank: rank}); err != nil {
				return err
			}
		}
	}

	if err := b.flush(); err != nil {
		return err
	}

	if version != "" {
		_, err := db.Exec(`INSERT INTO source_meta(key,value,updated_at) VALUES(?,?,datetime('now'))
			ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			"jmdict_priority_version", version)
		if err != nil {
			return fmt.Errorf("save version: %w", err)
		}
	}
	return nil
}

// --- XML types ---

type jmdictXMLEntry struct {
	EntSeq string `xml:"ent_seq"`
	KEle   []struct {
		KePri []string `xml:"ke_pri"`
	} `xml:"k_ele"`
	REle []struct {
		RePri []string `xml:"re_pri"`
	} `xml:"r_ele"`
}

// Ranks for priority markers that carry no news-frequency band. They sort below
// every banded entry (nf01–nf48) but above entries with no marker at all.
const (
	rankPrimaryMarker   = 50 // ichi1, spec1, news1, gai1
	rankSecondaryMarker = 60 // ichi2, spec2, news2, gai2
)

var nfBand = regexp.MustCompile(`^nf(\d{2})$`)

// priorityRank reduces an entry's ke_pri/re_pri markers to a single rank, lower
// being more frequent. It reports false when the entry carries no markers.
func priorityRank(e jmdictXMLEntry) (int, bool) {
	best := 0
	for _, markers := range priorityMarkers(e) {
		for _, m := range markers {
			rank := markerRank(strings.TrimSpace(m))
			if rank == 0 {
				continue
			}
			if best == 0 || rank < best {
				best = rank
			}
		}
	}
	return best, best != 0
}

// priorityMarkers returns every ke_pri and re_pri list in the entry. A word's
// kanji and kana forms can be marked independently, and the most frequent one
// stands for the entry.
func priorityMarkers(e jmdictXMLEntry) [][]string {
	markers := make([][]string, 0, len(e.KEle)+len(e.REle))
	for _, k := range e.KEle {
		markers = append(markers, k.KePri)
	}
	for _, r := range e.REle {
		markers = append(markers, r.RePri)
	}
	return markers
}

// markerRank scores a single priority marker, returning 0 for unrecognised ones.
func markerRank(m string) int {
	if band := nfBand.FindStringSubmatch(m); band != nil {
		n, err := strconv.Atoi(band[1])
		if err != nil || n == 0 {
			return 0
		}
		return n
	}
	switch m {
	case "ichi1", "spec1", "news1", "gai1":
		return rankPrimaryMarker
	case "ichi2", "spec2", "news2", "gai2":
		return rankSecondaryMarker
	}
	return 0
}

var createdDate = regexp.MustCompile(`JMdict created:\s*(\S+)`)

// parseCreatedDate pulls the build date out of the "JMdict created: ..." comment
// that precedes the root element. Returns "" for any other comment.
func parseCreatedDate(comment string) string {
	if m := createdDate.FindStringSubmatch(comment); m != nil {
		return m[1]
	}
	return ""
}

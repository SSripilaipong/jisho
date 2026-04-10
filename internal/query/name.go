package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/shsnail/jisho/internal/model"
)

func (q *querier) SearchNames(ctx context.Context, raw string) ([]model.Name, error) {
	switch {
	case containsWildcard(raw):
		return q.searchNamesByPattern(ctx, raw)
	case isJapanese(raw):
		return q.searchNamesByForm(ctx, raw+"%")
	default:
		return q.searchNamesASCII(ctx, raw)
	}
}

func (q *querier) searchNamesByForm(ctx context.Context, pattern string) ([]model.Name, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT DISTINCT n.id, n.kanji_json, n.kana_json, n.translation_json
		FROM name_forms nf
		JOIN names n ON n.id = nf.name_id
		WHERE nf.form LIKE ?
		ORDER BY n.id
		LIMIT 50`, pattern)
	if err != nil {
		return nil, fmt.Errorf("name form query: %w", err)
	}
	defer rows.Close()
	return scanNames(rows)
}

func (q *querier) searchNamesByPattern(ctx context.Context, raw string) ([]model.Name, error) {
	hasLeading := len(raw) > 0 && raw[0] == '*'
	hasTrailing := len(raw) > 0 && raw[len(raw)-1] == '*'
	core := []rune(raw)
	// Strip leading/trailing wildcards for core.
	start, end := 0, len(core)
	if hasLeading {
		start = 1
	}
	if hasTrailing && end > start {
		end--
	}
	coreStr := string(core[start:end])

	switch {
	case !hasLeading && hasTrailing:
		return q.searchNamesByForm(ctx, coreStr+"%")
	case hasLeading && !hasTrailing:
		rev := reverseRunes(coreStr)
		rows, err := q.db.QueryContext(ctx, `
			SELECT DISTINCT n.id, n.kanji_json, n.kana_json, n.translation_json
			FROM name_forms nf
			JOIN names n ON n.id = nf.name_id
			WHERE nf.form_rev LIKE ?
			ORDER BY n.id LIMIT 50`, rev+"%")
		if err != nil {
			return nil, fmt.Errorf("name rev query: %w", err)
		}
		defer rows.Close()
		return scanNames(rows)
	default:
		likePattern := fmt.Sprintf("%%%s%%", coreStr)
		return q.searchNamesByForm(ctx, likePattern)
	}
}

func (q *querier) searchNamesASCII(ctx context.Context, raw string) ([]model.Name, error) {
	var results []model.Name
	seen := map[string]bool{}

	if isASCIILetters(raw) {
		if hi := toHiragana(raw); hi != "" {
			ka := toKatakana(hi)
			rows, err := q.db.QueryContext(ctx, `
				SELECT DISTINCT n.id, n.kanji_json, n.kana_json, n.translation_json
				FROM name_forms nf
				JOIN names n ON n.id = nf.name_id
				WHERE nf.is_kana = 1 AND (nf.form LIKE ? OR nf.form LIKE ?)
				ORDER BY n.id LIMIT 50`, hi+"%", ka+"%")
			if err != nil {
				return nil, fmt.Errorf("name kana query: %w", err)
			}
			defer rows.Close()
			kanaResults, err := scanNames(rows)
			if err != nil {
				return nil, err
			}
			for _, n := range kanaResults {
				seen[n.ID] = true
				results = append(results, n)
			}
		}
	}

	// English translation search.
	enRows, err := q.db.QueryContext(ctx, `
		SELECT id, kanji_json, kana_json, translation_json
		FROM names
		WHERE translation_en LIKE ?
		ORDER BY id LIMIT 50`, "%"+raw+"%")
	if err != nil {
		return nil, fmt.Errorf("name en query: %w", err)
	}
	defer enRows.Close()
	enResults, err := scanNames(enRows)
	if err != nil {
		return nil, err
	}
	for _, n := range enResults {
		if !seen[n.ID] {
			seen[n.ID] = true
			results = append(results, n)
		}
	}
	return results, nil
}

func scanNames(rows *sql.Rows) ([]model.Name, error) {
	var results []model.Name
	for rows.Next() {
		var id, kanjiJSON, kanaJSON, translJSON string
		if err := rows.Scan(&id, &kanjiJSON, &kanaJSON, &translJSON); err != nil {
			return nil, fmt.Errorf("scan name: %w", err)
		}
		n := model.Name{ID: id}
		json.Unmarshal([]byte(kanjiJSON), &n.Kanji)
		json.Unmarshal([]byte(kanaJSON), &n.Kana)
		json.Unmarshal([]byte(translJSON), &n.Translation)
		results = append(results, n)
	}
	return results, rows.Err()
}

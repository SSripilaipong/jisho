package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/shsnail/jisho/internal/model"
)

func (q *querier) SearchWords(ctx context.Context, raw string, opts SearchOpts) ([]model.Word, error) {
	switch {
	case containsWildcard(raw):
		return q.searchWordsByPattern(ctx, raw, opts)
	case isJapanese(raw):
		return q.searchWordsByForm(ctx, raw+"%", opts)
	default:
		// ASCII: try romaji→kana first, then English FTS, union results.
		return q.searchWordsASCII(ctx, raw, opts)
	}
}

// containsWildcard reports whether the query contains a '*' wildcard.
func containsWildcard(s string) bool { return strings.ContainsRune(s, '*') }

// isJapanese reports whether s contains any hiragana, katakana, or CJK characters.
func isJapanese(s string) bool {
	for _, r := range s {
		if (r >= 0x3000 && r <= 0x9FFF) || (r >= 0xF900 && r <= 0xFAFF) {
			return true
		}
	}
	return false
}

// isASCIILetters reports whether s contains only ASCII letters.
func isASCIILetters(s string) bool {
	for _, r := range s {
		if r > 127 || !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) > 0
}

// searchWordsByForm performs a prefix/form LIKE query on word_forms.
// pattern must already include the LIKE wildcard (e.g. "食べ%").
func (q *querier) searchWordsByForm(ctx context.Context, pattern string, opts SearchOpts) ([]model.Word, error) {
	query := `
		SELECT DISTINCT w.id, w.kanji_json, w.kana_json, w.sense_json, w.is_common, w.jlpt_level
		FROM word_forms wf
		JOIN words w ON w.id = wf.word_id
		WHERE wf.form LIKE ?
		  AND (? = 0 OR w.jlpt_level = ?)
		  AND (? = 0 OR w.is_common = 1)
		ORDER BY w.is_common DESC, w.jlpt_level ASC NULLS LAST
		LIMIT 50`
	jlpt := opts.JLPTLevel
	common := boolInt(opts.CommonOnly)
	return q.execWordQuery(ctx, query, pattern, jlpt, jlpt, common)
}

// searchWordsByFormRev performs a prefix query on the reversed form column (for suffix search).
func (q *querier) searchWordsByFormRev(ctx context.Context, revPattern string, opts SearchOpts) ([]model.Word, error) {
	query := `
		SELECT DISTINCT w.id, w.kanji_json, w.kana_json, w.sense_json, w.is_common, w.jlpt_level
		FROM word_forms wf
		JOIN words w ON w.id = wf.word_id
		WHERE wf.form_rev LIKE ?
		  AND (? = 0 OR w.jlpt_level = ?)
		  AND (? = 0 OR w.is_common = 1)
		ORDER BY w.is_common DESC, w.jlpt_level ASC NULLS LAST
		LIMIT 50`
	jlpt := opts.JLPTLevel
	common := boolInt(opts.CommonOnly)
	return q.execWordQuery(ctx, query, revPattern, jlpt, jlpt, common)
}

// searchWordsByGloss performs an FTS5 English gloss search.
func (q *querier) searchWordsByGloss(ctx context.Context, ftsQuery string, opts SearchOpts) ([]model.Word, error) {
	query := `
		SELECT w.id, w.kanji_json, w.kana_json, w.sense_json, w.is_common, w.jlpt_level
		FROM words w
		JOIN words_fts f ON w.rowid = f.rowid
		WHERE words_fts MATCH ?
		  AND (? = 0 OR w.jlpt_level = ?)
		  AND (? = 0 OR w.is_common = 1)
		ORDER BY w.is_common DESC, f.rank
		LIMIT 50`
	jlpt := opts.JLPTLevel
	common := boolInt(opts.CommonOnly)
	return q.execWordQuery(ctx, query, ftsQuery, jlpt, jlpt, common)
}

// searchWordsByPattern handles queries containing '*' wildcards.
func (q *querier) searchWordsByPattern(ctx context.Context, raw string, opts SearchOpts) ([]model.Word, error) {
	hasLeading := strings.HasPrefix(raw, "*")
	hasTrailing := strings.HasSuffix(raw, "*")
	core := strings.Trim(raw, "*")

	// For ASCII romaji cores, convert to kana and use kana-aware helpers.
	if isASCIILetters(strings.ReplaceAll(core, " ", "")) {
		if hiragana := toHiragana(core); hiragana != "" {
			katakana := toKatakana(hiragana)
			switch {
			case !hasLeading && hasTrailing:
				return q.searchWordsByKana(ctx, hiragana, katakana, opts)
			case hasLeading && !hasTrailing:
				return q.searchWordsByKanaSuffix(ctx, reverseRunes(hiragana), reverseRunes(katakana), opts)
			}
		}
	}

	switch {
	case !hasLeading && hasTrailing:
		// Prefix search: "食べ*" → LIKE '食べ%'
		return q.searchWordsByForm(ctx, core+"%", opts)

	case hasLeading && !hasTrailing:
		// Suffix search: "*食べ" → reverse core → LIKE 'べ食%' on form_rev
		return q.searchWordsByFormRev(ctx, reverseRunes(core)+"%", opts)

	default:
		// Infix or mixed: translate * → % and use LIKE on form
		likePattern := strings.ReplaceAll(raw, "*", "%")
		return q.searchWordsByForm(ctx, likePattern, opts)
	}
}

// searchWordsASCII handles ASCII input: tries romaji→kana form search and English FTS,
// returns union deduplicated by ID.
func (q *querier) searchWordsASCII(ctx context.Context, raw string, opts SearchOpts) ([]model.Word, error) {
	var results []model.Word
	seen := map[string]bool{}

	// Try romaji if it looks like pure romaji (no spaces, all letters).
	if isASCIILetters(strings.ReplaceAll(raw, " ", "")) {
		hiragana := toHiragana(raw)
		if hiragana != "" {
			katakana := toKatakana(hiragana)
			kanaResults, err := q.searchWordsByKana(ctx, hiragana, katakana, opts)
			if err != nil {
				return nil, err
			}
			for _, w := range kanaResults {
				if !seen[w.ID] {
					seen[w.ID] = true
					results = append(results, w)
				}
			}
		}
	}

	// English FTS.
	ftsQuery := escapeFTS(raw) + "*"
	enResults, err := q.searchWordsByGloss(ctx, ftsQuery, opts)
	if err != nil {
		return nil, err
	}
	for _, w := range enResults {
		if !seen[w.ID] {
			seen[w.ID] = true
			results = append(results, w)
		}
	}
	return results, nil
}

// searchWordsByKana searches kana-only forms for both hiragana and katakana variants.
func (q *querier) searchWordsByKana(ctx context.Context, hiragana, katakana string, opts SearchOpts) ([]model.Word, error) {
	query := `
		SELECT DISTINCT w.id, w.kanji_json, w.kana_json, w.sense_json, w.is_common, w.jlpt_level
		FROM word_forms wf
		JOIN words w ON w.id = wf.word_id
		WHERE wf.is_kana = 1 AND (wf.form LIKE ? OR wf.form LIKE ?)
		  AND (? = 0 OR w.jlpt_level = ?)
		  AND (? = 0 OR w.is_common = 1)
		ORDER BY w.is_common DESC, w.jlpt_level ASC NULLS LAST
		LIMIT 50`
	jlpt := opts.JLPTLevel
	common := boolInt(opts.CommonOnly)
	return q.execWordQuery(ctx, query, hiragana+"%", katakana+"%", jlpt, jlpt, common)
}

// searchWordsByKanaSuffix searches kana-only reversed forms for suffix matching.
// hiraRev and kataRev are already reversed; the function appends '%' for the LIKE.
func (q *querier) searchWordsByKanaSuffix(ctx context.Context, hiraRev, kataRev string, opts SearchOpts) ([]model.Word, error) {
	query := `
		SELECT DISTINCT w.id, w.kanji_json, w.kana_json, w.sense_json, w.is_common, w.jlpt_level
		FROM word_forms wf
		JOIN words w ON w.id = wf.word_id
		WHERE wf.is_kana = 1 AND (wf.form_rev LIKE ? OR wf.form_rev LIKE ?)
		  AND (? = 0 OR w.jlpt_level = ?)
		  AND (? = 0 OR w.is_common = 1)
		ORDER BY w.is_common DESC, w.jlpt_level ASC NULLS LAST
		LIMIT 50`
	jlpt := opts.JLPTLevel
	common := boolInt(opts.CommonOnly)
	return q.execWordQuery(ctx, query, hiraRev+"%", kataRev+"%", jlpt, jlpt, common)
}

// execWordQuery runs a word SELECT and scans into model.Word slice.
func (q *querier) execWordQuery(ctx context.Context, query string, args ...any) ([]model.Word, error) {
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("word query: %w", err)
	}
	defer rows.Close()
	return scanWords(rows)
}

// scanWords scans SQL rows into model.Word values.
func scanWords(rows *sql.Rows) ([]model.Word, error) {
	var results []model.Word
	for rows.Next() {
		var (
			id, kanjiJSON, kanaJSON, senseJSON string
			isCommon                           int
			jlptLevel                          sql.NullInt64
		)
		if err := rows.Scan(&id, &kanjiJSON, &kanaJSON, &senseJSON, &isCommon, &jlptLevel); err != nil {
			return nil, fmt.Errorf("scan word: %w", err)
		}
		w := model.Word{
			ID:       id,
			IsCommon: isCommon == 1,
		}
		if jlptLevel.Valid {
			w.JLPTLevel = int(jlptLevel.Int64)
		}
		if err := json.Unmarshal([]byte(kanjiJSON), &w.Kanji); err != nil {
			return nil, fmt.Errorf("unmarshal kanji: %w", err)
		}
		if err := json.Unmarshal([]byte(kanaJSON), &w.Kana); err != nil {
			return nil, fmt.Errorf("unmarshal kana: %w", err)
		}
		if err := json.Unmarshal([]byte(senseJSON), &w.Senses); err != nil {
			return nil, fmt.Errorf("unmarshal sense: %w", err)
		}
		results = append(results, w)
	}
	return results, rows.Err()
}

// escapeFTS escapes special FTS5 characters in a query string.
func escapeFTS(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + s + `"`
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func reverseRunes(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

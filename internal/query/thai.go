package query

import (
	"context"
	"database/sql"

	"github.com/shsnail/jisho/internal/model"
)

// LookupThai returns all exact dictionary senses of an English word or phrase.
// It does not silently split unknown phrases or substitute prefix matches.
func LookupThai(ctx context.Context, db *sql.DB, text string) ([]model.ThaiEntry, error) {
	key := model.NormalizeEnglish(text)
	if key == "" {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, english, thai, part_of_speech, related, synonyms, antonyms
		FROM thai_entries WHERE english_key = ? OR search_key = ? ORDER BY id`, key, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.ThaiEntry
	for rows.Next() {
		var e model.ThaiEntry
		if err := rows.Scan(&e.ID, &e.English, &e.Thai, &e.PartOfSpeech, &e.Related, &e.Synonyms, &e.Antonyms); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

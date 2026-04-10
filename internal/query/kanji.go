package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shsnail/jisho/internal/model"
)

func (q *querier) LookupKanji(ctx context.Context, char string) (*model.Kanji, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT k.literal, k.grade, k.stroke_count, k.frequency, k.jlpt_level,
		       k.classical_radical, k.on_readings, k.kun_readings, k.nanori, k.meanings_en,
		       GROUP_CONCAT(kr.radical, ' ') AS radicals
		FROM kanji k
		LEFT JOIN kanji_radicals kr ON k.literal = kr.literal
		WHERE k.literal = ?
		GROUP BY k.literal`, char)

	var (
		literal, onJSON, kunJSON, nanoriJSON, meanJSON string
		grade, strokeCount, frequency, jlptLevel       sql.NullInt64
		classicalRadical                               sql.NullInt64
		radicals                                       sql.NullString
	)
	err := row.Scan(&literal, &grade, &strokeCount, &frequency, &jlptLevel,
		&classicalRadical, &onJSON, &kunJSON, &nanoriJSON, &meanJSON, &radicals)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup kanji: %w", err)
	}

	k := &model.Kanji{
		Literal: literal,
	}
	if grade.Valid {
		k.Grade = int(grade.Int64)
	}
	if strokeCount.Valid {
		k.StrokeCount = int(strokeCount.Int64)
	}
	if frequency.Valid {
		k.Frequency = int(frequency.Int64)
	}
	if jlptLevel.Valid {
		k.JLPTLevel = int(jlptLevel.Int64)
	}
	if classicalRadical.Valid {
		k.ClassicalRadical = int(classicalRadical.Int64)
	}
	if radicals.Valid && radicals.String != "" {
		k.Radicals = strings.Fields(radicals.String)
	}

	json.Unmarshal([]byte(onJSON), &k.OnReadings)
	json.Unmarshal([]byte(kunJSON), &k.KunReadings)
	json.Unmarshal([]byte(nanoriJSON), &k.Nanori)
	json.Unmarshal([]byte(meanJSON), &k.MeaningsEN)

	return k, nil
}

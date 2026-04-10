package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shsnail/jisho/internal/model"
)

func (q *querier) FilterKanjiByRadicals(ctx context.Context, radicals []string) ([]model.Kanji, error) {
	if len(radicals) == 0 {
		return nil, nil
	}

	// Build INTERSECT subquery: one SELECT per radical.
	// e.g. SELECT literal FROM kanji_radicals WHERE radical = ?
	//      INTERSECT
	//      SELECT literal FROM kanji_radicals WHERE radical = ?
	parts := make([]string, len(radicals))
	args := make([]any, len(radicals))
	for i, r := range radicals {
		parts[i] = `SELECT literal FROM kanji_radicals WHERE radical = ?`
		args[i] = r
	}
	intersect := strings.Join(parts, " INTERSECT ")

	// Append scalar filter args (jlpt/common are not applicable here).
	sqlStr := fmt.Sprintf(`
		SELECT k.literal, k.grade, k.stroke_count, k.frequency, k.jlpt_level,
		       k.classical_radical, k.on_readings, k.kun_readings, k.nanori, k.meanings_en
		FROM kanji k
		WHERE k.literal IN (%s)
		ORDER BY k.frequency ASC NULLS LAST
		LIMIT 100`, intersect)

	rows, err := q.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("radical filter: %w", err)
	}
	defer rows.Close()
	return scanKanjiRows(rows)
}

func scanKanjiRows(rows *sql.Rows) ([]model.Kanji, error) {
	var results []model.Kanji
	for rows.Next() {
		var (
			literal, onJSON, kunJSON, nanoriJSON, meanJSON string
			grade, strokeCount, frequency, jlptLevel       sql.NullInt64
			classicalRadical                               sql.NullInt64
		)
		if err := rows.Scan(&literal, &grade, &strokeCount, &frequency, &jlptLevel,
			&classicalRadical, &onJSON, &kunJSON, &nanoriJSON, &meanJSON); err != nil {
			return nil, fmt.Errorf("scan kanji: %w", err)
		}
		k := model.Kanji{Literal: literal}
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
		json.Unmarshal([]byte(onJSON), &k.OnReadings)
		json.Unmarshal([]byte(kunJSON), &k.KunReadings)
		json.Unmarshal([]byte(nanoriJSON), &k.Nanori)
		json.Unmarshal([]byte(meanJSON), &k.MeaningsEN)
		results = append(results, k)
	}
	return results, rows.Err()
}

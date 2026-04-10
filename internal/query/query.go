package query

import (
	"context"
	"database/sql"

	"github.com/shsnail/jisho/internal/model"
)

// SearchOpts are the optional filters for word search.
type SearchOpts struct {
	JLPTLevel  int  // 0 = no filter; 1–5 = specific level
	CommonOnly bool
}

// Querier is the interface the cmd layer depends on.
type Querier interface {
	SearchWords(ctx context.Context, q string, opts SearchOpts) ([]model.Word, error)
	LookupKanji(ctx context.Context, char string) (*model.Kanji, error)
	SearchNames(ctx context.Context, q string) ([]model.Name, error)
	FilterKanjiByRadicals(ctx context.Context, radicals []string) ([]model.Kanji, error)
}

type querier struct{ db *sql.DB }

// New returns a Querier backed by db.
func New(db *sql.DB) Querier {
	return &querier{db: db}
}

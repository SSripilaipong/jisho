# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build -o jisho .          # build binary
go install .                  # install to $GOPATH/bin
go vet ./...                  # lint
go test ./...                 # run all tests
go test ./internal/query/...  # run tests for a specific package
make dist                     # cross-compile for all platforms
```

## Architecture

The codebase is split into a presentation layer (`cmd/`) and four internal packages that must remain decoupled:

- **`internal/importer`** — one-time data ingestion only. Streams large JSON files (15–146 MB) from jmdict-simplified releases into SQLite via a generic batcher. Never imported by `query`.
- **`internal/query`** — all read queries. Never imported by `importer`. Depends only on `internal/model` and `database/sql`.
- **`internal/model`** — shared domain structs with JSON tags (used for both DB blob unmarshalling and query results).
- **`internal/db`** — schema DDL (`migrations.go`) and `Open`/`Migrate`. The only place that references the SQLite driver directly (aside from `main.go`'s blank import).

## Search strategy

Japanese/kana/romaji queries go through the `word_forms` table (B-tree LIKE), **not** FTS5. FTS5 (`words_fts`) is used only for English gloss search. This is intentional — FTS would match substrings by default, but the default should be prefix-only.

- No `*` + Japanese input → `word_forms WHERE form LIKE 'query%'` (prefix, hits all kanji/kana forms including alternates like 喰べる for 食べる)
- `*食べ` → reversed form column: `form_rev LIKE 'べ食%'`
- ASCII input → romaji→hiragana/katakana conversion (`internal/query/romaji.go`), then kana LIKE, unioned with English FTS results
- English → `words_fts MATCH 'query*'` (FTS5 prefix, matches "to eat" and "eatery" for "eat")

## Database

SQLite via `modernc.org/sqlite` (pure Go, no CGo). The only SQL trigger is the FTS sync trigger (`words_ai`) — no business logic in SQL.

Key tables: `words`, `word_forms` (one row per kanji/kana form), `words_fts` (English gloss FTS), `names`, `name_forms`, `kanji`, `kanji_radicals`, `source_meta`.

DB path resolution order: `--db` flag → `$JISHO_DB` → `$XDG_DATA_HOME/jisho/jisho.db` → `~/.local/share/jisho/jisho.db`.

## Update command

`jisho update` downloads assets from the jmdict-simplified GitHub release, writes to `jisho.db.tmp`, then atomically renames to `jisho.db`. The rename is the commit point — an interrupted import never corrupts the live DB.

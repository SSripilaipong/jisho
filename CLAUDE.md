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
- ASCII input → romaji→hiragana/katakana conversion (`internal/query/romaji.go`), then kana LIKE (`is_kana = 1`), unioned with English FTS results
- `tabe*` (romaji prefix) → convert to kana → `form LIKE 'たべ%' OR form LIKE 'タベ%'` (`is_kana = 1`)
- `*tabe` (romaji suffix) → convert to kana, reverse → `form_rev LIKE 'べた%' OR form_rev LIKE 'ベタ%'` (`is_kana = 1`)
- English → `words_fts MATCH 'query*'` (FTS5 prefix, matches "to eat" and "eatery" for "eat")

## Result ordering

Form searches share one ORDER BY, built by `rankOrder` in `internal/query/search.go`:
exact form match, then `is_common`, then `freq_rank` (ASC, NULLS last), then the
shortest matched form, then `jlpt_level`. English gloss search orders by `is_common`,
FTS5 `rank`, then `freq_rank`.

`freq_rank` is the JMdict news-frequency band (`nf01`–`nf48`), with 50/60 for entries
carrying only a primary/secondary priority marker and NULL for the unranked tail. It
comes from the original JMdict XML, not jmdict-simplified, which discards the
`ke_pri`/`re_pri` fields — see `internal/importer/jmdict_priority.go`.

## Database

SQLite via `modernc.org/sqlite` (pure Go, no CGo). The only SQL trigger is the FTS sync trigger (`words_ai`) — no business logic in SQL.

Key tables: `words`, `word_forms` (one row per kanji/kana form), `words_fts` (English gloss FTS), `names`, `name_forms`, `kanji`, `kanji_radicals`, `source_meta`.

Columns added after the initial schema go in `addedColumns` (`internal/db/migrations.go`)
as well as the CREATE TABLE, since `Migrate` also runs against existing databases.

DB path resolution order: `--db` flag → `$JISHO_DB` → `$XDG_DATA_HOME/jisho/jisho.db` → `~/.local/share/jisho/jisho.db`.

## Update command

`jisho update` downloads assets from the jmdict-simplified GitHub release, writes to `jisho.db.tmp`, then atomically renames to `jisho.db`. The rename is the commit point — an interrupted import never corrupts the live DB.

The same command also downloads and imports NECTEC's LEXiTRON English-to-Thai
data into the sibling `jisho-en-th.db`, even when Japanese data is already current.
The stores commit independently; failure in either is reported while the other
is still attempted (unless cancelled). `jisho update-th` is a Thai-only refresh,
with `--file` for an existing official ZIP archive. Thai schema is in
`internal/db/thai.go`; the archive's licenses and checksum are stored in `source_meta`.

## Interactive interfaces

Bare `jisho` and `jisho repl` run the full-screen Bubble Tea TUI. Its result
document keeps English glosses as structured selectable fields so wrapping never
joins separate definitions. `jisho repl-classic` retains the old Readline REPL
as a fallback. Keep fixes shared through the query/model layers where possible.

## Data sources

- [jmdict-simplified](https://github.com/scriptin/jmdict-simplified) — JMdict, JMnedict,
  Kanjidic2 and KRADFILE, as JSON. Everything except `words.freq_rank`.
- [JMdict](https://www.edrdg.org/pub/Nihongo/JMdict_e.gz) (EDRDG) — the original XML,
  read only for the `ke_pri`/`re_pri` priority markers behind `freq_rank`.

Both derive from the JMdict/EDICT and KANJIDIC projects of the Electronic Dictionary
Research and Development Group, used under CC BY-SA.

# Offline English → Thai lookup

Update Japanese dictionaries and ingest NECTEC's official LEXiTRON 2.0 archive:

```sh
jisho update
jisho th reluctant
jisho th "take care of"
```

`update` downloads and imports Thai data even when Japanese data is already
current. Each database is replaced independently after successful import; a
failure is reported without preventing the other dictionary from updating.
`update-th` remains available for a Thai-only refresh.

Inside the existing REPL, use `/th reluctant` or `/th take care of`.
To import an already downloaded official archive:

```sh
jisho update-th --file /path/to/lexitron_2.0_csv.zip
```

The English-to-Thai store sits beside the Japanese database: `jisho.db` uses
`jisho-en-th.db`. `--db`, `JISHO_DB`, and the existing XDG defaults determine
that location. Rebuilding the Japanese database does not overwrite this store.
Thai commands also work before the Japanese dictionary is installed.

Lookups match complete headwords or LEXiTRON search keys, ignoring case and
collapsing whitespace. They preserve all senses and parts of speech. Phrases
must exist in the dictionary; this is not machine translation. The current
archive has 83,232 rows; one without a Thai translation is skipped.

Imports build a temporary database and replace the installed one only after
success. The source URL, archive SHA-256, acknowledgement, and both original
licenses are retained in its `source_meta` table.

## Keyboard-first TUI

- `jisho` and `jisho repl` open the full-screen TUI. `jisho repl-classic`
  preserves the original scrolling Readline interface as a fallback.
- The prompt keeps focus after a search. Press Tab to browse the latest results.
- `/word` + Enter finds text in English results; `n`/`N` selects the
  next/previous match.
- Ctrl+E: look up the current word, or the current selection.
- `v`: start one continuous selection at the cursor character.
- `w`/`e`/`b`: Vim-style word motions; `h`/`l` and Left/Right move by
  character; `j`/`k` and Up/Down move between selectable lines.
- Keep selection inside one English gloss. Wrapped display lines are fine;
  crossing separate definitions should not combine unrelated meanings.
- Ctrl+E shows Thai meanings and parts of speech below the results. `d` toggles
  related terms, synonyms, and antonyms.
- Esc closes the pane, then leaves selection mode, then returns to the prompt.
- An unknown phrase offers word-by-word lookup on Enter; separate meanings stay
  explicitly labelled.

## Source and attribution

[NECTEC's official LEXiTRON 2.0 dataset](https://opend-portal.nectec.or.th/en/dataset/lexitron-2-0).
The download uses NECTEC's published ZIP, which includes the data and licenses.

This product is created by the adaptation of LEXiTRON developed by NECTEC (http://www.nectec.or.th/).

Copyright (c) 2003 National Electronics and Computer Technology Center (NECTEC).
All rights reserved. Original licenses: [English](licenses/LEXiTRON.txt),
[Thai](licenses/LEXiTRON-th.txt).

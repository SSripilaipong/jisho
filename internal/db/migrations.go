package db

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS source_meta (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS words (
		id         TEXT PRIMARY KEY,
		kanji_json TEXT NOT NULL DEFAULT '[]',
		kana_json  TEXT NOT NULL DEFAULT '[]',
		sense_json TEXT NOT NULL DEFAULT '[]',
		gloss_en   TEXT NOT NULL DEFAULT '',
		is_common  INTEGER NOT NULL DEFAULT 0,
		jlpt_level INTEGER
	)`,

	`CREATE TABLE IF NOT EXISTS word_forms (
		word_id  TEXT NOT NULL REFERENCES words(id),
		form     TEXT NOT NULL,
		form_rev TEXT NOT NULL,
		is_kana  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (word_id, form)
	)`,

	`CREATE INDEX IF NOT EXISTS idx_word_forms_form     ON word_forms(form)`,
	`CREATE INDEX IF NOT EXISTS idx_word_forms_form_rev ON word_forms(form_rev)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS words_fts USING fts5(
		id, gloss_en,
		content='words', content_rowid='rowid',
		tokenize='unicode61 remove_diacritics 1'
	)`,

	`CREATE TRIGGER IF NOT EXISTS words_ai AFTER INSERT ON words BEGIN
		INSERT INTO words_fts(rowid, id, gloss_en) VALUES (new.rowid, new.id, new.gloss_en);
	END`,

	`CREATE TABLE IF NOT EXISTS names (
		id               TEXT PRIMARY KEY,
		kanji_json       TEXT NOT NULL DEFAULT '[]',
		kana_json        TEXT NOT NULL DEFAULT '[]',
		translation_json TEXT NOT NULL DEFAULT '[]',
		translation_en   TEXT NOT NULL DEFAULT ''
	)`,

	`CREATE TABLE IF NOT EXISTS name_forms (
		name_id  TEXT NOT NULL REFERENCES names(id),
		form     TEXT NOT NULL,
		form_rev TEXT NOT NULL,
		is_kana  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (name_id, form)
	)`,

	`CREATE INDEX IF NOT EXISTS idx_name_forms_form     ON name_forms(form)`,
	`CREATE INDEX IF NOT EXISTS idx_name_forms_form_rev ON name_forms(form_rev)`,

	`CREATE TABLE IF NOT EXISTS kanji (
		literal           TEXT PRIMARY KEY,
		grade             INTEGER,
		stroke_count      INTEGER,
		frequency         INTEGER,
		jlpt_level        INTEGER,
		classical_radical INTEGER,
		nelson_radical    INTEGER,
		on_readings       TEXT NOT NULL DEFAULT '[]',
		kun_readings      TEXT NOT NULL DEFAULT '[]',
		nanori            TEXT NOT NULL DEFAULT '[]',
		meanings_en       TEXT NOT NULL DEFAULT '[]'
	)`,

	`CREATE TABLE IF NOT EXISTS kanji_radicals (
		literal TEXT NOT NULL REFERENCES kanji(literal),
		radical TEXT NOT NULL,
		PRIMARY KEY (literal, radical)
	)`,

	`CREATE INDEX IF NOT EXISTS idx_kanji_radicals_radical ON kanji_radicals(radical)`,
	`CREATE INDEX IF NOT EXISTS idx_words_jlpt             ON words(jlpt_level)`,
	`CREATE INDEX IF NOT EXISTS idx_words_common           ON words(is_common)`,
}

package model

// Word is a JMdict entry as returned by the query layer.
type Word struct {
	ID        string
	Kanji     []KanjiForm
	Kana      []KanaForm
	Senses    []Sense
	IsCommon  bool
	JLPTLevel int // 0 = unknown, 1–5
}

type KanjiForm struct {
	Text   string   `json:"text"`
	Common bool     `json:"common"`
	Tags   []string `json:"tags"`
}

type KanaForm struct {
	Text           string   `json:"text"`
	Common         bool     `json:"common"`
	Tags           []string `json:"tags"`
	AppliesToKanji []string `json:"appliesToKanji"`
}

type Sense struct {
	Gloss        []Gloss  `json:"gloss"`
	PartOfSpeech []string `json:"partOfSpeech"`
	Field        []string `json:"field"`
	Misc         []string `json:"misc"`
	Dialect      []string `json:"dialect"`
	Info         []string `json:"info"`
}

type Gloss struct {
	Lang string  `json:"lang"`
	Text string  `json:"text"`
	Type *string `json:"type,omitempty"`
}

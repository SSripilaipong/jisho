package model

// Name is a JMnedict entry as returned by the query layer.
type Name struct {
	ID          string
	Kanji       []NameKanjiForm
	Kana        []NameKanaForm
	Translation []NameTranslation
}

type NameKanjiForm struct {
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

type NameKanaForm struct {
	Text           string   `json:"text"`
	Tags           []string `json:"tags"`
	AppliesToKanji []string `json:"appliesToKanji"`
}

type NameTranslation struct {
	Types       []string `json:"type"`
	Translation []Gloss  `json:"translation"`
}

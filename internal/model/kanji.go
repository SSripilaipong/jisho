package model

// Kanji is a Kanjidic2 entry as returned by the query layer.
type Kanji struct {
	Literal          string
	Grade            int    // 0 = unknown
	StrokeCount      int
	Frequency        int    // 0 = not in top 2500
	JLPTLevel        int    // 0 = unknown, 1–4
	ClassicalRadical int
	OnReadings       []string
	KunReadings      []string
	Nanori           []string
	MeaningsEN       []string
	Radicals         []string // from kanji_radicals
}

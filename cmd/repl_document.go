package cmd

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
	"github.com/shsnail/jisho/internal/model"
)

type resultLine struct {
	prefix string
	text   string
	field  int // -1 means the line is not selectable
}

type resultDocument struct {
	lines  []resultLine
	fields []string
}

func newDocument() resultDocument { return resultDocument{} }

func (d *resultDocument) plain(text string) {
	d.lines = append(d.lines, resultLine{text: text, field: -1})
}
func (d *resultDocument) selectable(prefix, text string) {
	if text == "" {
		return
	}
	id := len(d.fields)
	d.fields = append(d.fields, text)
	d.lines = append(d.lines, resultLine{prefix: prefix, text: text, field: id})
}

func documentForWords(words []model.Word, names []model.Name) resultDocument {
	d := newDocument()
	for wi, word := range words {
		if wi > 0 {
			d.plain("")
		}
		kanji, kana := primaryText(word.Kanji), primaryKanaText(word.Kana)
		head := kanji
		if head == "" {
			head = kana
		}
		if kanji != "" && kana != "" && kana != kanji {
			head += "  【" + kana + "】"
		}
		var tags []string
		if word.IsCommon {
			tags = append(tags, "common")
		}
		if word.JLPTLevel > 0 {
			tags = append(tags, fmt.Sprintf("JLPT N%d", word.JLPTLevel))
		}
		if len(tags) > 0 {
			head += "  [" + strings.Join(tags, ", ") + "]"
		}
		d.plain(head)
		for i, sense := range word.Senses {
			if len(sense.PartOfSpeech) > 0 {
				d.plain("  " + strings.Join(sense.PartOfSpeech, ", "))
			}
			first := true
			for _, g := range sense.Gloss {
				if g.Lang != "eng" && g.Lang != "en" {
					continue
				}
				prefix := "     "
				if first {
					prefix = fmt.Sprintf("  %d. ", i+1)
					first = false
				}
				d.selectable(prefix, g.Text)
			}
			if len(sense.Info) > 0 {
				d.plain("     " + strings.Join(sense.Info, " "))
			}
		}
		if alts := buildAltForms(word.Kanji, word.Kana); len(alts) > 0 {
			d.plain("  also: " + strings.Join(alts, "、"))
		}
	}
	if len(names) > 0 {
		if len(words) > 0 {
			d.plain("")
		}
		d.plain("── Names ──")
		for _, n := range names {
			d.plain("")
			kanji, kana := "", ""
			if len(n.Kanji) > 0 {
				kanji = n.Kanji[0].Text
			}
			if len(n.Kana) > 0 {
				kana = n.Kana[0].Text
			}
			head := kanji
			if head == "" {
				head = kana
			}
			if kanji != "" && kana != "" {
				head += "  【" + kana + "】"
			}
			d.plain(head)
			for _, tr := range n.Translation {
				prefix := "  "
				if len(tr.Types) > 0 {
					prefix += "[" + strings.Join(tr.Types, ", ") + "] "
				}
				for _, g := range tr.Translation {
					if g.Lang == "eng" || g.Lang == "en" {
						d.selectable(prefix, g.Text)
						prefix = "  "
					}
				}
			}
		}
	}
	if len(d.lines) == 0 {
		d.plain("No results found.")
	}
	return d
}

func documentForKanji(k *model.Kanji) resultDocument {
	d := newDocument()
	if k == nil {
		d.plain("Kanji not found.")
		return d
	}
	d.plain(k.Literal)
	if len(k.OnReadings) > 0 {
		d.plain("  On:      " + strings.Join(k.OnReadings, "、"))
	}
	if len(k.KunReadings) > 0 {
		d.plain("  Kun:     " + strings.Join(k.KunReadings, "、"))
	}
	if len(k.Nanori) > 0 {
		d.plain("  Nanori:  " + strings.Join(k.Nanori, "、"))
	}
	for i, meaning := range k.MeaningsEN {
		prefix := "            "
		if i == 0 {
			prefix = "  Meaning: "
		}
		d.selectable(prefix, meaning)
	}
	var meta []string
	if k.JLPTLevel > 0 {
		meta = append(meta, fmt.Sprintf("JLPT N%d", k.JLPTLevel))
	}
	if k.Grade > 0 {
		meta = append(meta, fmt.Sprintf("grade %d", k.Grade))
	}
	if k.StrokeCount > 0 {
		meta = append(meta, fmt.Sprintf("%d strokes", k.StrokeCount))
	}
	if k.Frequency > 0 {
		meta = append(meta, fmt.Sprintf("freq #%d", k.Frequency))
	}
	if len(meta) > 0 {
		d.plain("  " + strings.Join(meta, "  "))
	}
	if len(k.Radicals) > 0 {
		d.plain("  Radicals: " + strings.Join(k.Radicals, " "))
	}
	return d
}

func documentForKanjiList(items []model.Kanji) resultDocument {
	d := newDocument()
	if len(items) == 0 {
		d.plain("No kanji found.")
		return d
	}
	for i := range items {
		kd := documentForKanji(&items[i])
		if i > 0 {
			d.plain("")
		}
		base := len(d.fields)
		d.fields = append(d.fields, kd.fields...)
		for _, line := range kd.lines {
			if line.field >= 0 {
				line.field += base
			}
			d.lines = append(d.lines, line)
		}
	}
	return d
}

func graphemes(s string) []string {
	var out []string
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		out = append(out, g.Str())
	}
	return out
}

func wordClass(g string) int {
	if strings.TrimSpace(g) == "" {
		return 0
	}
	r, _ := utf8FirstRune(g)
	if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '\'' || r == '-' {
		return 1
	}
	return 2
}

func utf8FirstRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 0
}

func primaryText(forms []model.KanjiForm) string {
	if len(forms) == 0 {
		return ""
	}
	return forms[0].Text
}

func primaryKanaText(forms []model.KanaForm) string {
	if len(forms) == 0 {
		return ""
	}
	return forms[0].Text
}

func buildAltForms(kanji []model.KanjiForm, kana []model.KanaForm) []string {
	if len(kanji) == 0 {
		var out []string
		for _, n := range kana[1:] {
			out = append(out, n.Text)
		}
		return out
	}
	var out []string
	for _, k := range kanji {
		for _, n := range kana {
			if k.Text == kanji[0].Text && len(kana) > 0 && n.Text == kana[0].Text {
				continue
			}
			ok := len(n.AppliesToKanji) == 0
			for _, a := range n.AppliesToKanji {
				if a == "*" || a == k.Text {
					ok = true
				}
			}
			if ok {
				out = append(out, k.Text+" 【"+n.Text+"】")
			}
		}
	}
	return out
}

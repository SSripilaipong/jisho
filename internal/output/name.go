package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/shsnail/jisho/internal/model"
)

// PrintNames writes a formatted name list to w.
func PrintNames(w io.Writer, names []model.Name) {
	if len(names) == 0 {
		fmt.Fprintln(w, "No results found.")
		return
	}
	for i, n := range names {
		if i > 0 {
			fmt.Fprintln(w)
		}
		printName(w, n)
	}
}

func printName(w io.Writer, n model.Name) {
	// Headword.
	kanji := ""
	if len(n.Kanji) > 0 {
		kanji = n.Kanji[0].Text
	}
	kana := ""
	if len(n.Kana) > 0 {
		kana = n.Kana[0].Text
	}

	head := kanji
	if head == "" {
		head = kana
	}
	fmt.Fprintf(w, "%s", bold(w, head))
	if kanji != "" && kana != "" {
		fmt.Fprintf(w, "  %s", dim(w, "【"+kana+"】"))
	}
	fmt.Fprintln(w)

	// Translations with type labels.
	for _, t := range n.Translation {
		var glosses []string
		for _, g := range t.Translation {
			if g.Lang == "eng" || g.Lang == "en" {
				glosses = append(glosses, g.Text)
			}
		}
		typeStr := ""
		if len(t.Types) > 0 {
			typeStr = dim(w, "["+strings.Join(t.Types, ", ")+"]") + " "
		}
		if len(glosses) > 0 {
			fmt.Fprintf(w, "  %s%s\n", typeStr, strings.Join(glosses, "; "))
		}
	}
}

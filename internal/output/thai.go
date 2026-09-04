package output

import (
	"fmt"
	"io"

	"github.com/shsnail/jisho/internal/model"
)

func PrintThai(w io.Writer, text string, entries []model.ThaiEntry) {
	if len(entries) == 0 {
		fmt.Fprintf(w, "No EN→TH entry for %q.\n", text)
		return
	}
	for _, e := range entries {
		fmt.Fprintf(w, "%s [%s] → %s\n", e.English, e.PartOfSpeech, e.Thai)
		if e.Related != "" {
			fmt.Fprintf(w, "  %s\n", e.Related)
		}
		if e.Synonyms != "" {
			fmt.Fprintf(w, "  synonyms: %s\n", e.Synonyms)
		}
		if e.Antonyms != "" {
			fmt.Fprintf(w, "  antonyms: %s\n", e.Antonyms)
		}
	}
	fmt.Fprintln(w, "Source: LEXiTRON 2.0 / NECTEC")
}

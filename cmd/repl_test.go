package cmd

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/shsnail/jisho/internal/model"
)

var update = flag.Bool("update", false, "update golden files")

func TestPaginateCombined(t *testing.T) {
	tests := []struct {
		name    string
		words   []model.Word
		names   []model.Name
		keyPresses []bool // false = continue ('n'), true = stop
	}{
		{
			name:  "words_only_fits",
			words: testWords("alpha", "beta", "gamma"),
		},
		{
			name:  "names_only_fits",
			names: testNames("Alice", "Bob", "Carol"),
		},
		{
			name:  "mixed_fits",
			words: testWords("alpha", "beta"),
			names: testNames("Alice", "Bob"),
		},
		{
			// 3 words + 4 names = 7; page 1: w0 w1 w2 n0 n1, page 2: n2 n3
			name:       "mixed_split",
			words:      testWords("alpha", "beta", "gamma"),
			names:      testNames("Alice", "Bob", "Carol", "Dave"),
			keyPresses: []bool{false}, // press 'n' to see page 2
		},
		{
			// 5 words + 2 names = 7; page 1: w0..w4, page 2: n0 n1
			name:       "names_on_second_page",
			words:      testWords("alpha", "beta", "gamma", "delta", "epsilon"),
			names:      testNames("Alice", "Bob"),
			keyPresses: []bool{false},
		},
		{
			// 7 total; user stops after page 1 — names section must not appear
			name:       "stop_early",
			words:      testWords("alpha", "beta", "gamma"),
			names:      testNames("Alice", "Bob", "Carol", "Dave"),
			keyPresses: []bool{true}, // stop at first prompt
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ki := 0
			nextKey := func() bool {
				if ki >= len(tc.keyPresses) {
					return true // stop if no more keys specified
				}
				k := tc.keyPresses[ki]
				ki++
				return k
			}

			var buf bytes.Buffer
			if err := paginateCombinedTo(&buf, tc.words, tc.names, nextKey); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := buf.Bytes()
			golden := filepath.Join("testdata", tc.name+".golden")

			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s: %v  (run with -update to create)", golden, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// testWords builds minimal model.Word values from display names.
func testWords(names ...string) []model.Word {
	out := make([]model.Word, len(names))
	for i, n := range names {
		out[i] = model.Word{
			ID:   n,
			Kana: []model.KanaForm{{Text: n}},
			Senses: []model.Sense{{
				Gloss: []model.Gloss{{Lang: "eng", Text: "gloss for " + n}},
			}},
		}
	}
	return out
}

// testNames builds minimal model.Name values from display names.
func testNames(names ...string) []model.Name {
	out := make([]model.Name, len(names))
	for i, n := range names {
		out[i] = model.Name{
			ID:   n,
			Kana: []model.NameKanaForm{{Text: n}},
			Translation: []model.NameTranslation{{
				Translation: []model.Gloss{{Lang: "eng", Text: "translation for " + n}},
			}},
		}
	}
	return out
}

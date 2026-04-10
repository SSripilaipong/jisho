package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shsnail/jisho/internal/output"
	"github.com/shsnail/jisho/internal/query"
)

var (
	flagJLPT   string
	flagCommon bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for words",
	Long: `Search for Japanese words by kanji, kana, romaji, or English.

Examples:
  jisho search 食べる
  jisho search taberu
  jisho search "to eat"
  jisho search "*食" --common
  jisho search eat --jlpt n5`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := strings.Join(args, " ")

		opts := query.SearchOpts{
			CommonOnly: flagCommon,
		}
		if flagJLPT != "" {
			n, err := parseJLPT(flagJLPT)
			if err != nil {
				return err
			}
			opts.JLPTLevel = n
		}

		querier := query.New(db)
		results, err := querier.SearchWords(cmd.Context(), q, opts)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}
		output.PrintWords(os.Stdout, results)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVar(&flagJLPT, "jlpt", "", "filter by JLPT level (n1-n5)")
	searchCmd.Flags().BoolVar(&flagCommon, "common", false, "show common words only")
}

// parseJLPT converts "n1"–"n5" (case-insensitive) to integer 1–5.
func parseJLPT(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "n1":
		return 1, nil
	case "n2":
		return 2, nil
	case "n3":
		return 3, nil
	case "n4":
		return 4, nil
	case "n5":
		return 5, nil
	}
	return 0, fmt.Errorf("invalid JLPT level %q, expected n1-n5", s)
}

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shsnail/jisho/internal/output"
	"github.com/shsnail/jisho/internal/query"
)

var radicalCmd = &cobra.Command{
	Use:   "radical <radical> [radical...]",
	Short: "Filter kanji by radicals",
	Long: `List kanji that contain all of the specified radicals.

Example:
  jisho radical 口 土`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Each arg may be a single radical or multiple characters; split into individual runes.
		var radicals []string
		for _, a := range args {
			for _, r := range []rune(a) {
				radicals = append(radicals, string(r))
			}
		}

		querier := query.New(db)
		results, err := querier.FilterKanjiByRadicals(cmd.Context(), radicals)
		if err != nil {
			return fmt.Errorf("radical filter: %w", err)
		}
		output.PrintKanjiList(os.Stdout, results)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(radicalCmd)
}

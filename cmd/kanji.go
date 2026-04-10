package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shsnail/jisho/internal/output"
	"github.com/shsnail/jisho/internal/query"
)

var kanjiCmd = &cobra.Command{
	Use:   "kanji <character>",
	Short: "Look up a kanji character",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		char := []rune(args[0])
		if len(char) != 1 {
			return fmt.Errorf("expected a single kanji character, got %q", args[0])
		}

		querier := query.New(db)
		k, err := querier.LookupKanji(cmd.Context(), string(char))
		if err != nil {
			return fmt.Errorf("kanji lookup: %w", err)
		}
		output.PrintKanji(os.Stdout, k)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(kanjiCmd)
}

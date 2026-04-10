package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shsnail/jisho/internal/output"
	"github.com/shsnail/jisho/internal/query"
)

var nameCmd = &cobra.Command{
	Use:   "name <query>",
	Short: "Search for Japanese names",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := strings.Join(args, " ")
		querier := query.New(db)
		results, err := querier.SearchNames(cmd.Context(), q)
		if err != nil {
			return fmt.Errorf("name search: %w", err)
		}
		output.PrintNames(os.Stdout, results)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(nameCmd)
}

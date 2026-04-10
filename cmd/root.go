package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shsnail/jisho/internal/query"
)

var dbPath string
var db *sql.DB

var rootCmd = &cobra.Command{
	Use:   "jisho",
	Short: "Offline Japanese dictionary",
	Long:  "jisho — offline Japanese dictionary CLI backed by JMdict/Kanjidic.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return runREPLWithQuerier(cmd.Context(), query.New(db))
		}
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// update command opens its own DB, skip here.
		if cmd.Name() == "update" {
			return nil
		}
		return openDB()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if db != nil {
			db.Close()
		}
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "path to jisho.db (overrides $JISHO_DB and XDG default)")
}

// resolveDBPath returns the path to the database file.
// Priority: --db flag > $JISHO_DB > $XDG_DATA_HOME/jisho/jisho.db > $HOME/.local/share/jisho/jisho.db
func resolveDBPath() string {
	if dbPath != "" {
		return dbPath
	}
	if v := os.Getenv("JISHO_DB"); v != "" {
		return v
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "jisho", "jisho.db")
}

// openDB opens the database and assigns the global db variable.
func openDB() error {
	path := resolveDBPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("database not found at %s\nRun `jisho update` to download and import dictionary data.", path)
	}

	var err error
	db, err = sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	return nil
}

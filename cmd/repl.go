package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shsnail/jisho/internal/model"
	"github.com/shsnail/jisho/internal/output"
	"github.com/shsnail/jisho/internal/query"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start an interactive REPL session",
	Long:  "Start an interactive prompt for searching words, kanji, and names.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runREPLWithQuerier(cmd.Context(), query.New(db))
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
}

const replPageSize = 5

var replCompleter = readline.NewPrefixCompleter(
	readline.PcItem("search",
		readline.PcItem("--common"),
		readline.PcItem("--jlpt",
			readline.PcItem("n1"),
			readline.PcItem("n2"),
			readline.PcItem("n3"),
			readline.PcItem("n4"),
			readline.PcItem("n5"),
		),
	),
	readline.PcItem("kanji"),
	readline.PcItem("name"),
	readline.PcItem("radical"),
	readline.PcItem("help"),
	readline.PcItem("exit"),
	readline.PcItem("quit"),
)

func replHistoryPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "jisho", "history")
}

func runREPLWithQuerier(ctx context.Context, q query.Querier) error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\x01\033[34m\x02jisho>\x01\033[0m\x02 ",
		AutoComplete:    replCompleter,
		HistoryFile:     replHistoryPath(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("readline init: %w", err)
	}
	defer rl.Close()

	fmt.Fprintln(os.Stdout, "jisho interactive mode  (type 'help' for commands, Ctrl-D to quit)")

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		}
		if err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if dispatchErr := replDispatch(ctx, rl, q, line); dispatchErr != nil {
			fmt.Fprintln(os.Stderr, "error:", dispatchErr)
		}
	}
	return nil
}

func replDispatch(ctx context.Context, rl *readline.Instance, q query.Querier, line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	cmd, args := parts[0], parts[1:]
	switch strings.ToLower(cmd) {
	case "search":
		return replSearch(ctx, q, args)
	case "kanji":
		return replKanji(ctx, q, args)
	case "name":
		return replName(ctx, q, args)
	case "radical":
		return replRadical(ctx, q, args)
	case "help":
		replPrintHelp()
	case "exit", "quit", "q":
		rl.Close()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q — type 'help' for usage\n", cmd)
	}
	return nil
}

func replSearch(ctx context.Context, q query.Querier, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: search [--jlpt n1-n5] [--common] <query>")
		return nil
	}

	var opts query.SearchOpts
	var queryParts []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--common":
			opts.CommonOnly = true
		case "--jlpt":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--jlpt requires a level: n1-n5")
				return nil
			}
			n, err := parseJLPT(args[i])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return nil
			}
			opts.JLPTLevel = n
		default:
			queryParts = append(queryParts, args[i])
		}
	}

	if len(queryParts) == 0 {
		fmt.Fprintln(os.Stderr, "usage: search [--jlpt n1-n5] [--common] <query>")
		return nil
	}

	results, err := q.SearchWords(ctx, strings.Join(queryParts, " "), opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stdout, "No results found.")
		return nil
	}
	return paginateWords(results)
}

func replKanji(ctx context.Context, q query.Querier, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kanji <character>")
		return nil
	}
	char := []rune(strings.Join(args, ""))
	if len(char) != 1 {
		fmt.Fprintln(os.Stderr, "kanji: expected a single character")
		return nil
	}
	k, err := q.LookupKanji(ctx, string(char))
	if err != nil {
		return fmt.Errorf("kanji: %w", err)
	}
	output.PrintKanji(os.Stdout, k)
	return nil
}

func replName(ctx context.Context, q query.Querier, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: name <query>")
		return nil
	}
	results, err := q.SearchNames(ctx, strings.Join(args, " "))
	if err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stdout, "No results found.")
		return nil
	}
	return paginateNames(results)
}

func replRadical(ctx context.Context, q query.Querier, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: radical <radical> [radical...]")
		return nil
	}
	var radicals []string
	for _, a := range args {
		for _, r := range []rune(a) {
			radicals = append(radicals, string(r))
		}
	}
	results, err := q.FilterKanjiByRadicals(ctx, radicals)
	if err != nil {
		return fmt.Errorf("radical: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stdout, "No kanji found.")
		return nil
	}
	return paginateKanji(results)
}

func replPrintHelp() {
	fmt.Print(`Commands:
  search [--jlpt n1-n5] [--common] <query>   search words
  kanji <character>                           look up a kanji
  name <query>                                search names
  radical <radical...>                        filter kanji by radicals
  help                                        show this message
  exit / quit                                 leave REPL
`)
}

// paginateWords prints words in pages of replPageSize, prompting to continue.
func paginateWords(items []model.Word) error {
	return paginate(len(items), func(start, end int) {
		output.PrintWords(os.Stdout, items[start:end])
	})
}

// paginateNames prints names in pages of replPageSize.
func paginateNames(items []model.Name) error {
	return paginate(len(items), func(start, end int) {
		output.PrintNames(os.Stdout, items[start:end])
	})
}

// paginateKanji prints kanji in pages of replPageSize.
func paginateKanji(items []model.Kanji) error {
	return paginate(len(items), func(start, end int) {
		output.PrintKanjiList(os.Stdout, items[start:end])
	})
}

// paginate drives page-by-page display, prompting the user after each page.
func paginate(total int, printPage func(start, end int)) error {
	for offset := 0; offset < total; offset += replPageSize {
		end := min(offset+replPageSize, total)
		if offset > 0 {
			fmt.Fprintln(os.Stdout)
		}
		printPage(offset, end)
		if end >= total {
			break
		}
		fmt.Fprintf(os.Stdout, "  ─── %d of %d ── any key for more, q to stop ─── ", end, total)
		if stop := readKey(); stop {
			fmt.Fprintln(os.Stdout)
			break
		}
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

// readKey reads a single keypress in raw mode. Returns true if the user wants to stop.
func readKey() bool {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		// Can't go raw (e.g. not a TTY); fall back to line read.
		var line string
		fmt.Scanln(&line)
		return strings.ToLower(strings.TrimSpace(line)) == "q"
	}
	defer term.Restore(fd, old)
	var buf [1]byte
	os.Stdin.Read(buf[:])
	b := buf[0]
	// q/Q, Ctrl-C, Ctrl-D, or Escape → stop
	return b == 'q' || b == 'Q' || b == 3 || b == 4 || b == 27
}

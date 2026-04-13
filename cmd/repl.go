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
	readline.PcItem("/search",
		readline.PcItem("--common"),
		readline.PcItem("--jlpt",
			readline.PcItem("n1"),
			readline.PcItem("n2"),
			readline.PcItem("n3"),
			readline.PcItem("n4"),
			readline.PcItem("n5"),
		),
	),
	readline.PcItem("/kanji"),
	readline.PcItem("/name"),
	readline.PcItem("/radical"),
	readline.PcItem("/help"),
	readline.PcItem("/exit"),
	readline.PcItem("/quit"),
	readline.PcItem("/q"),
	readline.PcItem("--common"),
	readline.PcItem("--jlpt",
		readline.PcItem("n1"),
		readline.PcItem("n2"),
		readline.PcItem("n3"),
		readline.PcItem("n4"),
		readline.PcItem("n5"),
	),
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

	fmt.Fprintln(os.Stdout, "jisho interactive mode  (type a word to search, '/help' for commands, Ctrl-D to quit)")

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
	if strings.HasPrefix(line, "/") {
		parts := strings.Fields(line[1:])
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
			fmt.Fprintf(os.Stderr, "unknown command %q — type '/help' for usage\n", "/"+cmd)
		}
		return nil
	}
	// Default: word search
	return replSearch(ctx, q, strings.Fields(line))
}

func replSearch(ctx context.Context, q query.Querier, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: /search [--jlpt n1-n5] [--common] <query>")
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
		fmt.Fprintln(os.Stderr, "usage: /search [--jlpt n1-n5] [--common] <query>")
		return nil
	}

	queryStr := strings.Join(queryParts, " ")
	results, err := q.SearchWords(ctx, queryStr, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	names, err := q.SearchNames(ctx, queryStr)
	if err != nil {
		return fmt.Errorf("search names: %w", err)
	}
	if len(results) == 0 && len(names) == 0 {
		fmt.Fprintln(os.Stdout, "No results found.")
		return nil
	}
	return paginateCombined(results, names)
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
  <query>                                      search words (default)
  <query> --common                             search common words only
  <query> --jlpt n1-n5                         search by JLPT level
  /search [--jlpt n1-n5] [--common] <query>   explicit search with flags
  /kanji <character>                           look up a kanji
  /name <query>                                search names
  /radical <radical...>                        filter kanji by radicals
  /help                                        show this message
  /exit  /quit  /q                             leave REPL
`)
}

// paginateCombined paginates words followed by names as a single stream, so the
// page size limit applies to the combined total rather than each section separately.
func paginateCombined(words []model.Word, names []model.Name) error {
	return paginateCombinedTo(os.Stdout, words, names, readKey)
}

func paginateCombinedTo(w io.Writer, words []model.Word, names []model.Name, nextKey func() bool) error {
	nw := len(words)
	total := nw + len(names)
	for offset := 0; offset < total; offset += replPageSize {
		end := min(offset+replPageSize, total)
		if offset > 0 {
			fmt.Fprintln(w)
		}
		wordEnd := min(end, nw)
		if offset < wordEnd {
			output.PrintWords(w, words[offset:wordEnd])
		}
		nameStart := max(0, offset-nw)
		nameEnd := max(0, end-nw)
		if nameEnd > nameStart {
			if nameStart == 0 {
				if offset < wordEnd {
					fmt.Fprintln(w)
				}
				fmt.Fprintln(w, "── Names ──")
			}
			output.PrintNames(w, names[nameStart:nameEnd])
		}
		if end >= total {
			break
		}
		fmt.Fprintf(w, "\033[34m  ─── %d of %d ── n for more, any other key to stop ─── \033[0m", end, total)
		if stop := nextKey(); stop {
			fmt.Fprintln(w)
			return nil
		}
		fmt.Fprintln(w)
	}
	return nil
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
		fmt.Fprintf(os.Stdout, "\033[34m  ─── %d of %d ── n for more, any other key to stop ─── \033[0m", end, total)
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
		return strings.ToLower(strings.TrimSpace(line)) != "n"
	}
	defer term.Restore(fd, old)
	var buf [1]byte
	os.Stdin.Read(buf[:])
	b := buf[0]
	// Only 'n'/'N' continues; everything else (including Ctrl-C, Ctrl-D, Escape) stops.
	return b != 'n' && b != 'N'
}

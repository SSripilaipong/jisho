package output

import (
	"io"
	"os"

	"golang.org/x/term"
)

// isTTY reports whether w is a terminal.
func isTTY(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// bold returns s wrapped in ANSI bold codes if w is a TTY, otherwise s unchanged.
func bold(w io.Writer, s string) string {
	if isTTY(w) {
		return "\033[1m" + s + "\033[0m"
	}
	return s
}

// dim returns s wrapped in ANSI dim codes if w is a TTY, otherwise s unchanged.
func dim(w io.Writer, s string) string {
	if isTTY(w) {
		return "\033[2m" + s + "\033[0m"
	}
	return s
}

package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func loadREPLHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" && (len(out) == 0 || out[len(out)-1] != line) {
			out = append(out, line)
		}
	}
	if len(out) > 1000 {
		out = out[len(out)-1000:]
	}
	return out
}

func appendREPLHistory(path, line string) {
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(strings.ReplaceAll(line, "\n", " ") + "\n")
}

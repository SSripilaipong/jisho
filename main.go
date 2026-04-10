package main

import (
	_ "modernc.org/sqlite"

	"github.com/shsnail/jisho/cmd"
)

func main() {
	cmd.Execute()
}

package main

import (
	"fmt"
	"os"

	"github.com/Kenttleton/orbiter/internal/commands"
)

func main() {
	root := commands.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

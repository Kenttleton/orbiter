package main

import (
	"fmt"
	"os"

	"github.com/Kenttleton/orbiter/internal/commands"
	_ "github.com/Kenttleton/orbiter/integrations"
	_ "github.com/Kenttleton/orbiter/internal/integrations/native"
)

func main() {
	root := commands.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

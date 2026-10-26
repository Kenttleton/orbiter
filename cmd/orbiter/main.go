package main

import (
	"os"

	"github.com/Kenttleton/orbiter/internal/commands"
)

func main() {
	os.Exit(commands.Execute())
}

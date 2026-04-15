package main

import (
	"os"

	"github.com/auro/devlane/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}

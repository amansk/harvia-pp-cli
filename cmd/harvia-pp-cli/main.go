package main

import (
	"os"

	"github.com/amansk/harvia-pp-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}

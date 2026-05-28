package main

import (
	"github.com/DreamCats/codegraph-cli/internal/cli"
	"os"
)

func main() {
	cli.Main(os.Args[1:])
}

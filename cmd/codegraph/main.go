package main

import (
	"codegraph-cli/internal/cli"
	"os"
)

func main() {
	cli.Main(os.Args[1:])
}

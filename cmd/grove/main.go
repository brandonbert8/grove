// Command grove is the Grove framework CLI: scaffolding and generation.
package main

import (
	"os"

	"github.com/brandonbert8/grove/packages/cli"
)

func main() {
	os.Exit(cli.New().Run(os.Args[1:]))
}

// Command grove is the Grove framework CLI: scaffolding and generation.
//
// Exit codes: 0 success, 2 usage error, 1 runtime failure.
package main

import (
	"os"

	"github.com/brandonbert8/grove/packages/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}

// Package cli implements the `grove` command-line tool: project
// scaffolding and code generation over a cobra command tree, with all
// terminal presentation isolated in the ui subpackage. Generators
// return FileChange events; commands render them. Main maps errors to
// exit codes: 0 success, 2 usage error, 1 runtime failure.
package cli

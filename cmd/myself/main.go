// Command myself is the agent-native graph memory CLI. See SPEC.md.
package main

import (
	"os"

	"github.com/mansus-org/myself/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}

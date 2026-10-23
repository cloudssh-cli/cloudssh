// Command cloudssh is a terminal UI for browsing cloud resources and SSHing into them.
package main

import (
	"fmt"
	"os"

	"github.com/SVMadhavaReddy/cloudssh/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

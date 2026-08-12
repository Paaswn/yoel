package main

import (
	"fmt"
	"os"

	"yoel/internal/cli"
)

// version is set to a release tag by the release workflow. Local builds keep
// the deliberately non-release value so they are never mistaken for one.
var version = "dev"

func main() {
	if err := cli.NewRootCommandWithVersion(version).Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

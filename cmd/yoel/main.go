package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/Paaswn/yoel/cli"
)

// version is set to a release tag by the release workflow. Local builds keep
// the deliberately non-release value so they are never mistaken for one.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
	)
	defer stop()
	cmd, err := cli.NewRootCommandWithVersion(version).ExecuteContextC(ctx)
	if err != nil {
		_ = cli.WriteCommandError(os.Stderr, cmd, err)
		os.Exit(1)
	}
}

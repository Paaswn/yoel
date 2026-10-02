package main

import (
	"context"
	"fmt"
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
	if err := cli.NewRootCommandWithVersion(version).ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

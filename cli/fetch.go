package cli

import (
	"context"

	"charm.land/huh/v2/spinner"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newFetchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch",
		Short: "Fetch the latest problems from the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := core.NewRegistry()
			if err != nil {
				return err
			}
			defer reg.Close()
			session, err := cliLoadSession(cmd)
			if err != nil {
				return err
			}
			if isTTY(cmd) {
				s := spinner.New().Title("Fetching problems...").Context(cmd.Context()).ActionWithErr(func(ctx context.Context) error {
					return reg.UpdateAllProblems(ctx, session)
				})
				return s.Run()
			} else {
    			return reg.UpdateAllProblems(cmd.Context(), session)
			
			}
			
		},
	}
}

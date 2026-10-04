package cli

import (
	"context"
	"fmt"

	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newSubmitCommand() *cobra.Command {
    var disableInteractive bool
    var maxRow int
	command := &cobra.Command{
		Use:   "submit",
		Short: "submit source file to grader",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
		    query := args[0]
			resultFunc := func(ctx context.Context, session core.SavedSession, problem core.ProblemLite, registry *core.Registry) error {
				submission, err := core.SubmitQuestion(ctx, session, problem, registry)
				if err != nil {
					return err
				}
				fmt.Println(submission)
				return nil
			}
			return queryWithFunc(cmd, query, resultFunc)
		},
	}
	command.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
	return command
}

package cli

import (
	"context"
	"fmt"

	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newMoveCommand() *cobra.Command {
	var trash bool
	var all bool
	var maxRow int
	cmd := &cobra.Command{
		Use:   "move <query>",
		Short: "Create a new problem",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			trash, err := cmd.Flags().GetBool("trash")
			if err != nil {
				return err
			}
			all, err := cmd.Flags().GetBool("all")
			if err != nil {
				return err
			}
			path := ""
			if len(args) > 1 {
				path = args[1]
			}
			singleFunc := func(ctx context.Context, ss core.SavedSession, pl *core.ProblemLite, r *core.Registry) error {
				return core.MoveProblem(ctx, pl, r, core.MoveOption{Trash: trash, Path: path})
			}
			funcs := resultFunc{
				single: singleFunc,
				multiple: func(c *cobra.Command, ss core.SavedSession, problems []core.ProblemLite, r *core.Registry, hsp handleSingleProblem) error {
					if all {
						for _, problem := range problems {
							err := singleFunc(cmd.Context(), ss, &problem, r)
							if err != nil {
							    fmt.Fprintln(cmd.ErrOrStderr(), err)
							}
						}
					} else if len(problems) == 1 {
						return singleFunc(cmd.Context(), ss, &problems[0], r)
					} else if isTTY(cmd) {
						problem, err := problemListInteractive(cmd, problems)
						if err != nil {
							return err
						}
						return singleFunc(cmd.Context(), ss, &problem, r)
					} else {
						return fmt.Errorf("query \"%s\" matched %v problems, but the output isn't interactive; submit using a problem ID or provide more specific problem's name.", query, len(problems))
					}
					return nil
				},
			}
			return queryWithResFunc(cmd, query, funcs)
		},
	}
	cmd.Flags().BoolVarP(&trash, "trash", "t", false, "move matched problem(s) to OS trash dir")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "move all matched problems without prompting")
	cmd.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
	return cmd
}

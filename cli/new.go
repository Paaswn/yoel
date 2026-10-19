package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	lg "charm.land/lipgloss/v2"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newNewCommand() *cobra.Command {
	var maxRow int
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "new <query>",
		Short: "Create a new problem",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			resFunc := resultFunc{
				single: func(ctx context.Context, session core.SavedSession, problem *core.ProblemLite, reg *core.Registry) error {
					if err := core.CreateProblem(ctx, session, problem, reg); err != nil {
						return err
					}
					_, err := lg.Fprintf(cmd.ErrOrStderr(), lg.NewStyle().Faint(true).Render("Problem successfully created at %s"), problem.DirectoryPath)
					return err
				},
				multiple: func(cmd *cobra.Command, session core.SavedSession, problems []core.ProblemLite, reg *core.Registry, singleFunc handleSingleProblem) error {
					if len(problems) == 1 {
						return singleFunc(cmd.Context(), session, &problems[0], reg)
					}
					if isTTY(cmd) {
						problem, err := problemListInteractive(cmd, problems)
						if err != nil {
							return err
						}
						return singleFunc(cmd.Context(), session, &problem, reg)
					} else {
						return fmt.Errorf("query \"%s\" matched %v problems, but the output isn't interactive; submit using a problem ID or provide more specific problem's name.", query, len(problems))
					}
				},
			}
			return queryWithResFunc(cmd, query, resFunc)
		},
	}
	command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
	command.Flags().BoolVarP(&jsonOutput, "json", "j", false, "format output as json")
	return command
}

type resultFunc struct {
	single   handleSingleProblem
	multiple handleMultipleProblems
}
type handleMultipleProblems func(*cobra.Command, core.SavedSession, []core.ProblemLite, *core.Registry, handleSingleProblem) error
type handleSingleProblem func(context.Context, core.SavedSession, *core.ProblemLite, *core.Registry) error

func queryWithResFunc(cmd *cobra.Command, query string, resFunc resultFunc) error {
	id, parseError := strconv.Atoi(query)
	if parseError != nil && !errors.Is(parseError, strconv.ErrSyntax) {
		return parseError
	}
	session, sessionError := cliLoadSession(cmd)
	if sessionError != nil {
		return sessionError
	}
	reg, regError := core.NewRegistry()
	if regError != nil {
		return regError
	}
	defer reg.Close()
	if parseError == nil {
		problem, err := reg.QueryByID(id)
		if err != nil {
			return err
		}
		return resFunc.single(cmd.Context(), session, &problem, reg)
	} else {
		problems, err := reg.QueryByName(query)
		if err != nil {
			return err
		}
		return resFunc.multiple(cmd, session, problems, reg, resFunc.single)
	}
}

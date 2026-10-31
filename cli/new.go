package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	lg "charm.land/lipgloss/v2"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newNewCommand() *cobra.Command {
	var maxRow int
	var jsonOutput bool
	var force bool
	var noPDF bool
	command := &cobra.Command{
		Use:   "new <query> [<path>]",
		Short: "Create a new problem",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			resFunc := resultFunc{
				single: func(ctx context.Context, session core.SavedSession, problem *core.ProblemLite, reg *core.Registry) error {
					path, err := os.Getwd()
					if err != nil {
						return err
					}
					if len(args) > 1 {
					    path = args[1]
					}
					return cliCreateQuestion(cmd, ctx, session, problem, reg, core.CreateProblemOption{Force: force, NoPDF: noPDF, Path: path})

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
	command.Flags().BoolVarP(&force, "force", "f", false, "format output as json")
	command.Flags().BoolVar(&noPDF, "no-pdf", false, "save the PDF without opening its viewer")
	return command
}

type resultFunc struct {
	single   handleSingleProblem
	multiple handleMultipleProblems
}
type handleMultipleProblems func(*cobra.Command, core.SavedSession, []core.ProblemLite, *core.Registry, handleSingleProblem) error
type handleSingleProblem func(context.Context, core.SavedSession, *core.ProblemLite, *core.Registry) error

func cliCreateQuestion(cmd *cobra.Command, ctx context.Context, session core.SavedSession, problem *core.ProblemLite, reg *core.Registry, option core.CreateProblemOption) error {
	err := core.CreateProblem(ctx, session, problem, reg, option)
	if errors.Is(err, core.ProblemExistErr) {
		if !isTTY(cmd) {
			return fmt.Errorf("cannot create \"%s\", path already exists, and force is set to \"%v\" while the terminal isn't interactive.", problem.CodeName, option.Force)
		}
		confirm, promptErr := yesNoPrompt(cmd, "Problem path already exists. Create anyway?")
		if promptErr != nil {
			return promptErr
		}
		if confirm {
			option.Force = true
			err = core.CreateProblem(ctx, session, problem, reg, option)
		}
	}
	if err != nil {
		return err
	}
	notice := lg.NewStyle().Faint(true).Render(fmt.Sprintf("Problem successfully created at %s", problem.DirectoryPath))
	_, err = lg.Fprintln(cmd.ErrOrStderr(), notice)
	return err
}
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

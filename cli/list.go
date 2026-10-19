package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	var disableInteractive bool
	var maxRow int
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "list",
		Short: "Show list of problems from grader",
		RunE: func(cmd *cobra.Command, args []string) error {
			single := func(ctx context.Context, session core.SavedSession, problem *core.ProblemLite, reg *core.Registry) error {
				jsonOutput, err := cmd.Flags().GetBool("json")
				if err != nil {
					return err
				}
				disableInteractive, err := cmd.Flags().GetBool("disable-interactive")
				if err != nil {
					return err
				}
				if jsonOutput {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(problem)
				} else if disableInteractive {
					return problemPrint(cmd.OutOrStdout(), *problem)
				} else {
					err = core.CreateProblem(ctx, session, problem, reg)
					if err != nil {
						return err
					}
					_, err := lg.Fprintf(cmd.ErrOrStderr(), lg.NewStyle().Faint(true).Render("Problem successfully created at %s"), problem.DirectoryPath)
					return err
				}
			}
			resFunc := resultFunc{
				single: single,
				multiple: func(cmd *cobra.Command, session core.SavedSession, problems []core.ProblemLite, reg *core.Registry, singleFunc handleSingleProblem) error {
					jsonOutput, err := cmd.Flags().GetBool("json")
					if err != nil {
						return err
					}
					disableInteractive, err := cmd.Flags().GetBool("disable-interactive")
					if err != nil {
						return err
					}
					if jsonOutput {
						return json.NewEncoder(cmd.OutOrStdout()).Encode(problems)
					} else if disableInteractive {
						for _, problem := range problems {
							if err := singleFunc(cmd.Context(), session, &problem, reg); err != nil {
								return err
							}
						}
					} else {
						problem, err := problemListInteractive(cmd, problems)
						if err != nil {
							return err
						}
						return singleFunc(cmd.Context(), session, &problem, reg)
					}
					return nil
				},
			}
			return queryWithResFunc(cmd, "", resFunc)
		},
	}
	command.Flags().BoolVarP(&jsonOutput, "json", "j", false, "print output as json")
	command.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
	command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
	return command
}

func problemListInteractive(cmd *cobra.Command, problems []core.ProblemLite) (core.ProblemLite, error) {
	var selected int
	maxRow, err := cmd.Flags().GetInt("max-row")
	if err != nil {
		return core.ProblemLite{}, err
	}
	keymap := huh.NewDefaultKeyMap()
	keymap.Select.Down.SetKeys("j", "down", "ctrl+j")
	keymap.Select.Up.SetKeys("k", "up", "ctrl+k")
	options := buildOptions(problems)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().TitleFunc(func() string {
				return problems[selected].PrettyName
			}, &selected).Options(
				options...,
			).Value(&selected).Height(maxRow),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).WithKeyMap(keymap)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return core.ProblemLite{}, errors.New("Aborted")
		}
		return core.ProblemLite{}, err
	}
	return problems[selected], nil
}

func problemPrint(writer io.Writer, problem core.ProblemLite) error {
	_, err := fmt.Fprintf(writer, "%s %d\n", problem.CodeName, problem.ID)
	return err
}

func problemListPrint(cmd *cobra.Command, problems []core.ProblemLite, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(problems)
	}

	writer := bufio.NewWriterSize(cmd.OutOrStdout(), 4096)
	for _, problem := range problems {
		if err := problemPrint(writer, problem); err != nil {
			return err
		}
	}
	return writer.Flush()
}

var (
	ColorGreen  = lipgloss.Color("#22C55E")
	ColorYellow = lipgloss.Color("#EAB308")
	ColorRed    = lipgloss.Color("#EF4444")
)

func buildOptions(problems []core.ProblemLite) []huh.Option[int] {
	options := make([]huh.Option[int], 0, len(problems))
	for i, q := range problems {
		prefixes := lipgloss.NewStyle()
		percentage := "-"
		if !q.IsOnLocal && q.BestScore == 0 {
			prefixes = prefixes.Faint(true)
		} else {
			bestScore := q.BestScore
			if bestScore <= 0 {
				prefixes = prefixes.Foreground(ColorRed)
			} else if bestScore < 100 {
				prefixes = prefixes.Foreground(ColorYellow)
			} else {
				prefixes = prefixes.Foreground(ColorGreen)
			}
			percentage = fmt.Sprintf("%.2f%%", bestScore)
		}
		num := prefixes
		prefixes = prefixes.Width(7).MarginRight(1).Align(lipgloss.Right)
		prettyName := prefixes.Render(percentage) + num.Render(q.CodeName)
		options = append(options, huh.NewOption(prettyName, i))
	}
	return options

}

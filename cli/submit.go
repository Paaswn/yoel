package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/huh/v2/spinner"
	lg "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newSubmitCommand() *cobra.Command {
	var maxRow int
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "submit",
		Short: "Submit source file to grader",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			single := func(ctx context.Context, session core.SavedSession, problem *core.ProblemLite, registry *core.Registry) error {
				var submission core.SubmissionResult
				jsonOutput, err := cmd.Flags().GetBool("json")
				if err != nil {
					return err
				}
				if isTTY(cmd) {
					relative, err := filepath.Rel(problem.DirectoryPath, problem.SourcePath)
					if err != nil {
						return err
					}
					_, err = lg.Fprintf(cmd.ErrOrStderr(), lg.NewStyle().Faint(true).Render("Submitting %s with file %s...\n"), problem.CodeName, relative)
					if err != nil {
						return err
					}
					s := spinner.New().Context(cmd.Context()).WithOutput(cmd.ErrOrStderr()).Title("Waiting for respone").ActionWithErr(func(c context.Context) error {
						sub, err := core.SubmitProblem(c, session, *problem, registry)
						submission = sub
						return err
					})
					if err := s.Run(); err != nil {
						return err
					}
				} else {
					sub, err := core.SubmitProblem(cmd.Context(), session, *problem, registry)
					submission = sub
					if err != nil {
						return err
					}
				}
				if jsonOutput {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(submission)
				} else {
					fm, err := formatSubmission(&submission)
					if err != nil {
						return err
					}
					_, err = lg.Fprintln(cmd.OutOrStdout(), fm)
					return err
				}
			}
			resFunc := resultFunc{
				single: single,
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

func formatSubmission(s *core.SubmissionResult) (string, error) {
	switch s.Status {
	case core.Done:
		return formatDoneSubmission(s)
	case core.CompilationError, core.GraderError:
		return formatFailedSubmission(s, s.Status), nil
	}
	return "", nil
}

const maxEvalWidth = 32

func formatDoneSubmission(s *core.SubmissionResult) (string, error) {
	header := lg.NewStyle().Bold(true)
	t := table.New().StyleFunc(func(row, col int) lg.Style {
		if col == 0 {
			return header
		}
		return lg.NewStyle()
	}).Border(lg.HiddenBorder()).Row("Problem:", s.ProblemName)

	fmtPoints := lg.NewStyle()
	points := *s.Points
	if points >= 70.0 {
		fmtPoints = fmtPoints.Foreground(ColorGreen)
	} else if points <= 30.0 {
		fmtPoints = fmtPoints.Foreground(ColorRed)
	} else {
		fmtPoints = fmtPoints.Foreground(ColorYellow)
	}
	t.Row("Points:", fmtPoints.Render(fmt.Sprintf("%.2f", points)))

	styleP := lg.NewStyle().Foreground(ColorGreen)
	styleT := lg.NewStyle().Foreground(ColorYellow)
	styleFail := lg.NewStyle().Foreground(ColorRed)
	evals := &strings.Builder{}
	comment := []rune(*s.GraderComment)
	lineWidth := 0
	for i, c := range comment {
		if c == '\n' {
			evals.WriteRune('\n')
			lineWidth = 0
			continue
		}
		if c == '[' {
			for j := i + 1; j < len(comment) && comment[j] != '\n'; j++ {
				if comment[j] == ']' {
					groupWidth := lg.Width(string(comment[i : j+1]))
					if groupWidth <= maxEvalWidth && lineWidth+groupWidth > maxEvalWidth {
						evals.WriteRune('\n')
						lineWidth = 0
					}
					break
				}
			}
		}
		if lineWidth+1 > maxEvalWidth {
			evals.WriteRune('\n')
			lineWidth = 0
		}
		lineWidth += 1
		switch c {
		case '[', ']':
			if _, err := evals.WriteRune(c); err != nil {
				return "", err
			}
		case 'P':
			if _, err := evals.WriteString(styleP.Render("P")); err != nil {
				return "", err
			}
		case '-':
			if _, err := evals.WriteString(styleFail.Render("-")); err != nil {
				return "", err
			}
		case 'T':
			if _, err := evals.WriteString(styleT.Render("T")); err != nil {
				return "", err
			}
		default:
			if _, err := evals.WriteString(styleFail.Render(string(c))); err != nil {
				return "", err
			}
		}
	}
	t.Row("Evaluations:", evals.String())
	return t.Render(), nil
}

func formatFailedSubmission(s *core.SubmissionResult, t core.Status) string {
	style := lg.NewStyle().MaxHeight(128)
	switch t {
	case core.CompilationError:
		return style.Render("Compiler message:\n", *s.CompilerMessage)
	case core.GraderError:
		return style.Render("Grader message:\n", *s.GraderComment)
	}
	return ""
}

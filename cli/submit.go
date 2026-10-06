package cli

import (
	"context"
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
    var disableInteractive bool
    var maxRow int
	command := &cobra.Command{
		Use:   "submit",
		Short: "submit source file to grader",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
		    query := args[0]
			resultFunc := func(ctx context.Context, session core.SavedSession, problem core.ProblemLite, registry *core.Registry) error {
			    var submission core.SubmissionResult
			    if isTTY(cmd) {
					lg.Fprintf(cmd.ErrOrStderr(), "Submitting %s with file %s...\n", problem.CodeName, filepath.Base( problem.SourcePath ))
					s := spinner.New().Context(cmd.Context()).WithOutput(cmd.ErrOrStderr()).Title("Waiting for respone").ActionWithErr(func(c context.Context) error {
        				sub, err := core.SubmitQuestion(c, session, problem, registry)
                        submission = sub
                        return err
					})
					if err := s.Run(); err != nil {
    					return err
					}
				} else {
    				sub, err := core.SubmitQuestion(cmd.Context(), session, problem, registry)
                    submission = sub
                    if err != nil {
                        return err
                    }
				}
				_, err := lg.Fprintln(cmd.OutOrStdout(), formatSubmission(&submission))
				return err
			}
			return queryWithFunc(cmd, query, resultFunc)
		},
	}
	command.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
	return command
}

func formatSubmission(s *core.SubmissionResult) string {
    switch s.Status {
        case core.Done:
            return formatDoneSubmission(s)
        case core.CompilationError, core.GraderError:
            return formatFailedSubmission(s, s.Status)
    }
    return ""
}

const maxEvalWitdth = 32
func formatDoneSubmission(s *core.SubmissionResult) string {
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
        fmtPoints.Foreground(ColorGreen)
    } else if points <= 30.0 {
        fmtPoints.Foreground(ColorRed)
    } else {
        fmtPoints.Foreground(ColorYellow)
    }
    t.Row("Points:", fmtPoints.Render( fmt.Sprintf("%.2f", points) ))
    var evalResult strings.Builder
    var eval strings.Builder
    styleP := lg.NewStyle().Foreground(ColorGreen)
    styleFail := lg.NewStyle().Foreground(ColorRed)
    rowWidth := 0
    for _, e := range s.Evaluations {
        width := 2
        eval.WriteByte('[')
        for _, ch := range *e.Result {
            if ch == 'P' {
                eval.WriteString(styleP.Render("P"))
            } else {
                eval.WriteString(styleFail.Render("-"))
            }
            width++
        }
        eval.WriteByte(']')
        if rowWidth+width > maxEvalWitdth {
            evalResult.WriteByte('\n')
            rowWidth = 0
        }
        evalResult.WriteString(eval.String())
        rowWidth += width
        eval.Reset()
    }
    t.Row("Evaluations:", evalResult.String())
    return t.Render()
}

func formatFailedSubmission(s *core.SubmissionResult, t core.Status) string {
    style := lg.NewStyle().MaxWidth(30).MaxHeight(50)
    switch t {
        case core.CompilationError:
            return style.Render("Compiler message:\n", *s.CompilerMessage)
        case core.GraderError:
            return style.Render("Grader message:\n",*s.GraderComment)
    }
    return ""
}
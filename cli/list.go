package cli

import (
	"bufio"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	gapi "github.com/Paaswn/yoel/graderapi"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
    var disableInteractive bool
    command := &cobra.Command{
        Use:   "list",
        Short: "show list of questions from grader",
        RunE: func(cmd *cobra.Command, args []string) error {
            return renderQuestionLists(cmd, disableInteractive)
        },
    }
    command.Flags().BoolVarP(&disableInteractive, "disbale-interactive", "d", false, "disable interactive mode")
    return command
}

func renderQuestionLists(cmd *cobra.Command, disableInteractive bool) error {
    session, err := loadSession()
    if err != nil {
        return err
    }
    client, err := gapi.NewClient(defaultGraderURL, nil )
    questions, err := client.WithToken(session.Token).ListProblems(cmd.Context())
    if err != nil {
        return err
    }
    if term.IsTerminal(os.Stdin.Fd()) && !disableInteractive {
        var selected int
        var (
       	ColorGreen  = lipgloss.Color("#22C55E")
       	ColorYellow = lipgloss.Color("#EAB308")
        ColorRed = lipgloss.Color("#EF4444")
        )
        huh.NewForm(
            huh.NewGroup(
                huh.NewSelect[int]().Title("Question List").Options(
                    func() []huh.Option[int] {
                        tmp := make( []huh.Option[int], 0,  len(questions))
                        for _, q := range questions {
                            prefixes := lipgloss.NewStyle()
                            percentage := "-"
                            if q.BestScore == nil {
                                prefixes = prefixes.Faint(true)
                            } else {
                                bestScore := int(*q.BestScore)
                                if (bestScore <= 0) {
                                    prefixes = prefixes.Foreground(ColorRed)
                                } else if bestScore < 100 {
                                    prefixes = prefixes.Foreground(ColorYellow)
                                } else {
                                    prefixes = prefixes.Foreground(ColorGreen)
                                }
                                percentage = fmt.Sprintf("%v%%", bestScore)
                            }
                            num := prefixes
                            prefixes = prefixes.Width(4).MarginRight(1).Align(lipgloss.Right)
                            prettyName := prefixes.Render(percentage) + num.Render( q.CodeName )
                            tmp = append(tmp, huh.NewOption(prettyName, q.ID))
                        }
                        return tmp
                    }()...,
                ).Value(&selected).Height(5).Inline(true),
            ),
        ).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).RunWithContext(cmd.Context())
    } else {
        writer := bufio.NewWriterSize(os.Stdout, 4096)
        for _, q := range questions {
            writer.WriteString(q.CodeName)
            fmt.Fprint(writer, " " , q.ID)
            writer.WriteRune('\n')
        }
        if err := writer.Flush(); err != nil {
            return err
        }
    }
    return nil
}

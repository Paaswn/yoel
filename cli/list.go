package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
    var disableInteractive bool
    var maxRow int
    var filter string
    command := &cobra.Command{
        Use:   "list",
        Short: "Show list of problems from grader",
        RunE: func(cmd *cobra.Command, args []string) error {
            return queryWithResFunc(cmd, "", core.CreateProblem)
        },
    }
    command.Flags().StringVarP(&filter, "filter", "f", "", "filter problem name")
    command.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
    return command
}

// func renderProblemLists(cmd *cobra.Command) error {
//     session, err := cliLoadSession(cmd)
//     if err != nil {
//         return err
//     }
//     reg, err := core.NewRegistry()
//     if err != nil {
//         return err
//     }
//     defer reg.Close()
//     var problems []core.ProblemLite
//     if cmd.Flags().Changed("filter") {
//         filter, err := cmd.Flags().GetString("filter")
//         if err != nil {
//             return err
//         }
//         problems, err = reg.QueryByName(filter)
//         if err != nil {
//             return err
//         }
//     } else {
//         problems, err = reg.GetAllProblems()
//         if err != nil {
//             return err
//         }
//     }
// 
//     if err != nil {
//         return err
//     }
//     disableInteractive, err := cmd.Flags().GetBool("disable-interactive")
//     if err != nil {
//         return err
//     }
//     if isTTY(cmd) && !disableInteractive {
//         problem, err := problemListInteractive(cmd, problems)
//         if err != nil {
//             return err
//         }
//         return core.CreateProblem(cmd.Context(), session, problem, reg)
//     } else {
//         if err := problemListPrint(cmd.OutOrStderr(), problems); err != nil {
//             return err
//         }
//     }
//     return nil
// }

func problemListInteractive(cmd *cobra.Command, problems []core.ProblemLite) ( core.ProblemLite, error ) {
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
                options...
            ).Value(&selected).Height(maxRow),
        ),
    ).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).WithKeyMap(keymap)
    if err := form.Run(); err != nil {
        if errors.Is(err, huh.ErrUserAborted) {
            return core.ProblemLite{}, errors.New("Aborted")
        }
        return core.ProblemLite{ }, err
    }
    return problems[selected], nil
}

func problemListPrint(w io.Writer, problems []core.ProblemLite) error {
    writer := bufio.NewWriterSize(w, 4096)
    for _, q := range problems {
        writer.WriteString(q.CodeName)
        fmt.Fprint(writer, " " , q.ID)
        writer.WriteRune('\n')
    }
    if err := writer.Flush(); err != nil {
        return err
    }
    return nil
}

var (
   	ColorGreen  = lipgloss.Color("#22C55E")
   	ColorYellow = lipgloss.Color("#EAB308")
    ColorRed = lipgloss.Color("#EF4444")
)
func buildOptions(problems []core.ProblemLite) []huh.Option[int] {
    options := make( []huh.Option[int], 0,  len(problems))
    for i, q := range problems {
        prefixes := lipgloss.NewStyle()
        percentage := "-"
        if !q.IsOnLocal && q.BestScore == 0 {
            prefixes = prefixes.Faint(true)
        } else {
            bestScore := q.BestScore
            if (bestScore <= 0) {
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
        prettyName := prefixes.Render(percentage) + num.Render( q.CodeName )
        options = append(options, huh.NewOption(prettyName, i))
    }
    return options

}

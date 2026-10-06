package cli

import (
	"context"
	"reflect"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newSeeCommand() *cobra.Command {
    var json bool
    var disableInteractive bool
    var maxRow int
    cmd := &cobra.Command{
        Use:   "see",
        Short: "Display problem informations",
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            query := args[0]
            resFunc := func(ctx context.Context, session core.SavedSession, problem core.ProblemLite, reg *core.Registry) error {
                rows := make([][]string, 0)
                row := 0
                v := reflect.ValueOf(problem)
                ty := reflect.TypeOf(problem)
                for i := 0; i < v.NumField(); i++ {
                    fType := ty.Field(i) 
                    fVal := v.Field(i)
                    if fType.Name == "SortOrder" {
                        continue
                    }
                    rows = append(rows, []string{})
                    rows[row] = append(rows[row], fType.Name)
                    rows[row] = append(rows[row], lipgloss.Sprintf( "%v", fVal.Interface() ))
                    row++
                }
                t := table.New().Rows(rows...).Border(lipgloss.HiddenBorder())
                _, err := lipgloss.Fprintln(cmd.OutOrStderr(), t.Render())
                return err
            }
            return queryWithResFunc(cmd, query, resFunc)
        },
    }
    cmd.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    cmd.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
    cmd.Flags().BoolVar(&json, "json", false, "format output as json")
    return cmd
}


package cli

import (
	"context"
	"fmt"
	"reflect"
	"encoding/json"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newSeeCommand() *cobra.Command {
    var jsonOutput bool
    var disableInteractive bool
    var maxRow int
    cmd := &cobra.Command{
        Use:   "see",
        Short: "Display problem informations",
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            query := args[0]
            single := func(ctx context.Context, session core.SavedSession, problem core.ProblemLite, reg *core.Registry) error {
                jsonOutput, err := cmd.Flags().GetBool("json")
                if err != nil {
                    return err
                }
                if jsonOutput {
                    return json.NewEncoder(cmd.OutOrStdout()).Encode(problem)
                } else {
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
                        rows[row] = append(rows[row], fmt.Sprintf( "%v", fVal.Interface() ))
                        row++
                    }
                    t := table.New().Rows(rows...).Border(lipgloss.HiddenBorder())
                    _, err := lipgloss.Fprintln(cmd.OutOrStdout(), t.Render())
                    return err
                }
            }
            resFunc :=  resultFunc {
                single: single,
                multiple: func (cmd *cobra.Command, session core.SavedSession, problems []core.ProblemLite, reg *core.Registry, resFunc handleSingleProblem) error {
                    jsonOutput, err := cmd.Flags().GetBool("json")
                    if err != nil {
                        return err
                    }
                    if len(problems) == 1 {
                        return resFunc(cmd.Context(), session, problems[0], reg)
                    } else if !disableInteractive && isTTY(cmd) {
                        problem, err := problemListInteractive(cmd, problems)
                        if err != nil {
                            return err
                        }
                        return resFunc(cmd.Context(), session, problem, reg)
                    } else {
                        if jsonOutput {
                            return json.NewEncoder(cmd.OutOrStdout()).Encode(problems)
                        } else {
                            for _, problem := range problems {
                                resFunc(cmd.Context(), session, problem, reg)
                            }    
                        }
                    }
                    return nil
                },
            }
            return queryWithResFunc(cmd, query, resFunc)
        },
    }
    cmd.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    cmd.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
    cmd.Flags().BoolVarP(&jsonOutput, "json", "j" ,false, "format output as json")
    return cmd
}

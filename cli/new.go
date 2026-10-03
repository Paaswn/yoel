package cli

import (
	"errors"
	"strconv"

	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newNewCommand() *cobra.Command {
    var disableInteractive bool
    var maxRow int
    command :=  &cobra.Command{
        Use:   "new <query>",
        Short: "Create a new question",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return newFunc(cmd, args[0], core.CreateQuestion)
        },
    }
    command.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
    return command
}

func newFunc(cmd *cobra.Command, query string, resFunc resultFunc) error {
    id, parseError := strconv.Atoi(query)
    if parseError != nil && !errors.Is(parseError, strconv.ErrSyntax) {
        return parseError
    }
    session, sessionError := core.LoadSession()
    if errors.Is(sessionError, core.SessionExpiredError) {
        if !yesNoPrompt(cmd, "Session expired. Login again ?") {
            return nil
        }
        return runLoginForm(cmd)
    }
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
        return resFunc(cmd.Context(), session, problem, reg)
    } else {
        problems, err := reg.QueryByName(query)
        if err != nil {
            return err
        }
        if len(problems) == 1 {
            return resFunc(cmd.Context(), session, problems[0], reg)
        }
        disableInteractive, err := cmd.Flags().GetBool("disable-interactive")
        if err != nil {
            return err
        }
        if isTTY(cmd) && !disableInteractive {
            problem, err := questionListInteractive(cmd, problems)
            if err != nil {
                return err
            }
            return resFunc(cmd.Context(), session, problem, reg)
        } else {
            if err := questionListPrint(cmd.OutOrStdout(), problems); err != nil {
                return err
            }
        }
    }
    return nil
}

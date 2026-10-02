package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/Paaswn/yoel/core"
	"github.com/charmbracelet/x/term"
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
            return newFunc(cmd, args[0])
        },
    }
    command.Flags().BoolVarP(&disableInteractive, "disable-interactive", "d", false, "disable interactive mode")
    command.Flags().IntVarP(&maxRow, "max-row", "m", 10, "maximum number of rows to display")
    return command
}

func newFunc(cmd *cobra.Command, query string) error {
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
            if errors.Is(err, core.ProblemNotFound) {
                return core.ProblemNotFoundNotice()
            }
            return err
        }
        ctx, cancel := context.WithTimeout(cmd.Context(), core.TimeOut)
        defer cancel()
        return core.CreateQuestion(ctx, session, problem)
    } else {
        problems, err := reg.QueryByName(query)
        if err != nil {
            if errors.Is(err, core.ProblemNotFound) {
                return core.ProblemNotFoundNotice()
            }
            return err
        }
        disableInteractive, err := cmd.Flags().GetBool("disable-interactive")
        if err != nil {
            return err
        }
        if term.IsTerminal(os.Stdin.Fd()) && !disableInteractive {
            problem, err := questionListInteractive(cmd, problems)
            if err != nil {
                return err
            }
            ctx, cancel := context.WithTimeout(cmd.Context(), core.TimeOut)
            defer cancel()
            return core.CreateQuestion(ctx, session, problem)
        } else {
            writer := bufio.NewWriterSize(os.Stdout, 4096)
            for _, q := range problems {
                writer.WriteString(q.CodeName)
                fmt.Fprint(writer, " " , q.ID)
                writer.WriteRune('\n')
            }
            if err := writer.Flush(); err != nil {
                return err
            }
        }
    }
    return nil           
}



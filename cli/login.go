package cli

import (
	"context"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func yesNoPrompt(cmd *cobra.Command, title string) bool {
    var confirm bool 
    huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().Title(title).Value(&confirm),
        ),
    ).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).RunWithContext(cmd.Context())
    return confirm
}

func newLoginCommand() *cobra.Command {
    var graderURL string
    command :=  &cobra.Command{
            Use: "login",
            Short: "login to your account",
            Long:  "login to your account, session is saved inside your os keyring",
            RunE: func(command *cobra.Command, _ []string) error {
                return runLoginForm(command)
		},
    }
    command.Flags().StringVar(&graderURL, "base-url", core.DefaultGraderURL, "grader API base URL")
    return command
}

func getUserPass(command *cobra.Command) (string, string, error) {
    var username string
	var password string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().EchoMode(huh.EchoModeNormal).Value(&username).Title("Username"),
			huh.NewInput().EchoMode(huh.EchoModePassword).Value(&password).Title("Password"),
		),
	).
	WithInput(command.InOrStdin()).
	WithOutput(command.ErrOrStderr())
	if err := form.Run(); err != nil {
		return "", "" ,err
	}
	return username, password, nil
}
func runLoginForm(command *cobra.Command) error {
    username, password, err := getUserPass(command)
    if err != nil {
        return err
    }
	ctx, cancel := context.WithTimeout(context.Background(), core.TimeOut)
    defer cancel()
    if err := core.LoginAndSaveSession(core.DefaultGraderURL, username, password, ctx); err != nil {
        return err
    }
    if _, err := fmt.Fprintln(os.Stderr, "Login Successfully"); err != nil {
        return err
    }
	return nil
}
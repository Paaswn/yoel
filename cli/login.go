package cli

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func yesNoPrompt(cmd *cobra.Command, title string) (bool, error) {
	var confirm bool
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Title(title).Value(&confirm),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).RunWithContext(cmd.Context())
	if err != nil {
		return false, err
	}
	return confirm, nil
}

func newLoginCommand() *cobra.Command {
	var graderURL string
	command := &cobra.Command{
		Use:   "login",
		Short: "Login to your account",
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
	if !isTTY(command) {
		return "", "", errors.New("Login requires an interactive terminal")
	}
	if err := form.Run(); err != nil {
		return "", "", err
	}
	return username, password, nil
}

func runLoginForm(command *cobra.Command) error {
	username, password, err := getUserPass(command)
	if err != nil {
		return err
	}
	if err := core.LoginAndSaveSession(core.DefaultGraderURL, username, password, command.Context()); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(command.ErrOrStderr(), "Login successfully"); err != nil {
		return err
	}
	return nil
}

package cli

import (
	"fmt"

	"yoel/internal/graderapi"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

const defaultGraderURL = "https://grader.nattee.net"

type loginForm func(command *cobra.Command) (username string, password string, err error)

func newLoginCommand(runForm loginForm) *cobra.Command {
	var baseURL string

	command := &cobra.Command{
		Use:   "login",
		Short: "Login to the grader",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := loginAndSave(command, runForm, baseURL)
			return err
		},
	}
	command.Flags().StringVar(&baseURL, "base-url", defaultGraderURL, "grader API base URL")
	return command
}

func loginAndSave(command *cobra.Command, runForm loginForm, baseURL string) (storedSession, error) {
	username, password, err := runForm(command)
	if err != nil {
		return storedSession{}, fmt.Errorf("login form: %w", err)
	}

	client, err := graderapi.NewClient(baseURL, nil)
	if err != nil {
		return storedSession{}, fmt.Errorf("login command: %w", err)
	}
	apiSession, err := client.Login(command.Context(), username, password)
	if err != nil {
		return storedSession{}, fmt.Errorf("login failed: %w", err)
	}
	session := storedSession{
		BaseURL:   baseURL,
		Token:     apiSession.Token,
		ExpiresAt: apiSession.ExpiresAt,
		User:      apiSession.User,
	}
	if err := saveSession(session); err != nil {
		return storedSession{}, fmt.Errorf("save login session: %w", err)
	}
	if _, err := fmt.Fprintf(command.OutOrStdout(), "✓ Login successful as %s\n", session.User.Login); err != nil {
		return storedSession{}, err
	}
	return session, nil
}

func runLoginForm(command *cobra.Command) (string, string, error) {
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
	if err := form.RunWithContext(command.Context()); err != nil {
		return "", "", err
	}
	return username, password, nil
}

func runReloginPrompt(command *cobra.Command) (bool, error) {
	accepted := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Session expired. Log in again?").
				Affirmative("Yes").
				Negative("No").
				Value(&accepted),
		),
	).
		WithInput(command.InOrStdin()).
		WithOutput(command.ErrOrStderr())
	if err := form.RunWithContext(command.Context()); err != nil {
		return false, err
	}
	return accepted, nil
}

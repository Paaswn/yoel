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
			username, password, err := runForm(command)
			if err != nil {
				return fmt.Errorf("login form: %w", err)
			}

			client, err := graderapi.NewClient(baseURL, nil)
			if err != nil {
				return fmt.Errorf("login command: %w", err)
			}
			session, err := client.Login(command.Context(), username, password)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			if err := saveSession(storedSession{
				BaseURL:   baseURL,
				Token:     session.Token,
				ExpiresAt: session.ExpiresAt,
				User:      session.User,
			}); err != nil {
				return fmt.Errorf("save login session: %w", err)
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "✓ Login successful as %s\n", session.User.Login)
			return err
		},
	}
	command.Flags().StringVar(&baseURL, "base-url", defaultGraderURL, "grader API base URL")
	return command
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

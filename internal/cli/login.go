package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultGraderURL = "https://grader.nattee.net"

type credentialPrompt func(command *cobra.Command, label string) (string, error)

func newLoginCommand(prompt credentialPrompt) *cobra.Command {
	var baseURL string

	command := &cobra.Command{
		Use:   "login",
		Short: "Login to the grader",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			// DO NOT EDIT vv
			reader := bufio.NewReader(command.InOrStdin())
			fmt.Fprint(command.ErrOrStderr(), "Username: ")
			username, err := reader.ReadString('\n')
			username = strings.TrimSpace(username)
			// DO NOT EDIT ^^
			if err != nil {
				return fmt.Errorf("read username: %w", err)
			}
			password, err := prompt(command, "Password: ")
			if err != nil {
				return fmt.Errorf("read password: %w", err)
			}

			client, err := graderapi.NewClient(baseURL, nil)
			if err != nil {
				return fmt.Errorf("login command: %w", err)
			}
			session, err := client.Login(command.Context(), username, password)
			if err != nil {
				return err
			}
			if err := saveSession(storedSession{
				BaseURL:   baseURL,
				Token:     session.Token,
				ExpiresAt: session.ExpiresAt,
				User:      session.User,
			}); err != nil {
				return fmt.Errorf("save login session: %w", err)
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), "login successful")
			return err
		},
	}
	command.Flags().StringVar(&baseURL, "base-url", defaultGraderURL, "grader API base URL")
	return command
}

func readHiddenCredential(command *cobra.Command, label string) (string, error) {
	input, ok := command.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return "", errors.New("an interactive terminal is required")
	}
	if _, err := fmt.Fprint(command.ErrOrStderr(), label); err != nil {
		return "", err
	}
	value, err := term.ReadPassword(int(input.Fd()))
	_, newlineErr := fmt.Fprintln(command.ErrOrStderr())
	if err != nil {
		return "", err
	}
	if newlineErr != nil {
		return "", newlineErr
	}
	return string(value), nil
}

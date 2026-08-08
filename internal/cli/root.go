package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"yoel/internal/graderapi"
)

const defaultGraderURL = "https://grader.nattee.net"

type credentialPrompt func(command *cobra.Command, label string) (string, error)

// NewRootCommand creates the top-level command.
func NewRootCommand() *cobra.Command {
	return newRootCommand(readHiddenCredential)
}

func newRootCommand(prompt credentialPrompt) *cobra.Command {
	root := &cobra.Command{
		Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}

	initSubCommand(root, prompt)

	return root
}

func initSubCommand(root *cobra.Command, prompt credentialPrompt) {
	var baseURL string

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to the grader",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			username, err := prompt(command, "Username: ")
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
			if _, err := client.Login(command.Context(), username, password); err != nil {
				return err
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), "login successful")
			return err
		},
	}
	loginCmd.Flags().StringVar(&baseURL, "base-url", defaultGraderURL, "grader API base URL")
	root.AddCommand(loginCmd)
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

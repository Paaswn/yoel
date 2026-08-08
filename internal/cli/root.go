package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"yoel/internal/graderapi"
)

const defaultGraderURL = "https://grader.nattee.net"

// NewRootCommand creates the top-level command. Login behavior, credentials,
// token storage, and output remain intentionally unimplemented for the owner
// to define.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}

	initSubCommand(root)

	return root
}

func initSubCommand(root *cobra.Command) {
	var baseURL string
	var login string
	var password string

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to the grader",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := graderapi.NewClient(baseURL, nil)
			if err != nil {
				return fmt.Errorf("login command: %w", err)
			}
			if _, err := client.Login(command.Context(), login, password); err != nil {
				return err
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), "login successful")
			return err
		},
	}
	loginCmd.Flags().StringVar(&baseURL, "base-url", defaultGraderURL, "grader API base URL")
	loginCmd.Flags().StringVar(&login, "login", "", "grader login")
	loginCmd.Flags().StringVar(&password, "password", "", "grader password")
	_ = loginCmd.MarkFlagRequired("login")
	_ = loginCmd.MarkFlagRequired("password")
	root.AddCommand(loginCmd)
}

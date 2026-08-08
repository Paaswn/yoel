package cli

import "github.com/spf13/cobra"

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
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to the grader",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.AddCommand(loginCmd)
}

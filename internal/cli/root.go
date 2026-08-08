package cli

import "github.com/spf13/cobra"

// NewRootCommand creates the top-level command. Endpoint commands, flags,
// prompts, and output remain intentionally unimplemented for the owner to
// define.
func NewRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}

	return command
}

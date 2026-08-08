package cli

import "github.com/spf13/cobra"

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

	root.AddCommand(newLoginCommand(prompt))
	root.AddCommand(newQuestionCommand())
	return root
}

package cli

import "github.com/spf13/cobra"

// NewRootCommand creates the top-level command.
func NewRootCommand() *cobra.Command {
	return newRootCommandWithOpener(readHiddenCredential, openWithDefaultViewer)
}

func newRootCommand(prompt credentialPrompt) *cobra.Command {
	return newRootCommandWithOpener(prompt, openWithDefaultViewer)
}

func newRootCommandWithOpener(prompt credentialPrompt, opener fileOpener) *cobra.Command {
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
	root.AddCommand(newQuestionCommand(opener))
	root.AddCommand(newUserCommand())
	return root
}

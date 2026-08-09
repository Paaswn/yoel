package cli

import "github.com/spf13/cobra"

// NewRootCommand creates the top-level command.
func NewRootCommand() *cobra.Command {
	return newRootCommandWithOpener(runLoginForm, openWithDefaultViewer)
}

func newRootCommand(form loginForm) *cobra.Command {
	return newRootCommandWithOpener(form, openWithDefaultViewer)
}

func newRootCommandWithOpener(form loginForm, opener fileOpener) *cobra.Command {
	root := &cobra.Command{
		Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}

	root.AddCommand(newLoginCommand(form))
	root.AddCommand(newQuestionCommand(opener))
	root.AddCommand(newUserCommand())
	return root
}

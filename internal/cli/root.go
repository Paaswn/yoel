package cli

import "github.com/spf13/cobra"

// NewRootCommand creates the top-level command.
func NewRootCommand() *cobra.Command {
	return newRootCommandWithDependencies(runLoginForm, runReloginPrompt, openWithDefaultViewer)
}

func newRootCommand(form loginForm) *cobra.Command {
	return newRootCommandWithDependencies(form, declineRelogin, openWithDefaultViewer)
}

func newRootCommandWithOpener(form loginForm, opener fileOpener) *cobra.Command {
	return newRootCommandWithDependencies(form, declineRelogin, opener)
}

func newRootCommandWithDependencies(form loginForm, confirm reloginPrompt, opener fileOpener) *cobra.Command {
	root := &cobra.Command{
		Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}

	sessions := newSessionProvider(form, confirm)
	root.AddCommand(newLoginCommand(form))
	root.AddCommand(newQuestionCommand(opener, sessions))
	root.AddCommand(newUserCommand(sessions))
	root.AddCommand(newSubmitCommand(sessions))
	return root
}

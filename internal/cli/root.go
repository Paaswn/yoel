package cli

import "github.com/spf13/cobra"

// NewRootCommand creates the top-level command.
func NewRootCommand() *cobra.Command {
	return NewRootCommandWithVersion("dev")
}

// NewRootCommandWithVersion constructs Yoel with its build-time release tag.
// The command package accepts it explicitly to keep tests and local builds
// independent from global version state.
func NewRootCommandWithVersion(version string) *cobra.Command {
	return newRootCommandWithDependenciesAndVersion(runLoginForm, runReloginPrompt, openWithDefaultViewer, version, defaultUpdateDependencies(version))
}

func newRootCommand(form loginForm) *cobra.Command {
	return newRootCommandWithDependencies(form, declineRelogin, openWithDefaultViewer)
}

func newRootCommandWithOpener(form loginForm, opener fileOpener) *cobra.Command {
	return newRootCommandWithDependencies(form, declineRelogin, opener)
}

func newRootCommandWithDependencies(form loginForm, confirm reloginPrompt, opener fileOpener) *cobra.Command {
	return newRootCommandWithDependenciesAndVersion(form, confirm, opener, "dev", defaultUpdateDependencies("dev"))
}

func newRootCommandWithDependenciesAndVersion(form loginForm, confirm reloginPrompt, opener fileOpener, version string, updates updateDependencies) *cobra.Command {
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
	root.AddCommand(newUpdateCommand(updates))
	root.AddCommand(newSubmitCommandWithUpdateNotice(sessions, func(command *cobra.Command) {
		maybeShowUpdateNotice(command, updates)
	}))
	return root
}

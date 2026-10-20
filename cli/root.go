package cli

import (
	"os"

	"github.com/Paaswn/yoel/core"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

func NewRootCommandWithVersion(version string) *cobra.Command {

	root := &cobra.Command{
		Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.AddGroup(&cobra.Group{
		ID:    "problem",
		Title: "Problem related commands",
	})
	root.AddGroup(&cobra.Group{
		ID:    "user",
		Title: "User related commands",
	})
	// user related command
	login := newLoginCommand()
	login.GroupID = "user"

	// problem related command
	list := newListCommand()
	fetch := newFetchCommand()
	new := newNewCommand()
	submit := newSubmitCommand()
	see := newSeeCommand()
	list.GroupID = "problem"
	fetch.GroupID = "problem"
	new.GroupID = "problem"
	submit.GroupID = "problem"
	see.GroupID = "problem"

	update := newUpdateCommand(defaultUpdateDependencies(version))
	root.AddCommand(login, list, fetch, new, submit, see, update)
	return root
}

func isTTY(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	fout, fok := cmd.OutOrStdout().(*os.File)
	return ok && term.IsTerminal(f.Fd()) && fok && term.IsTerminal(fout.Fd())
}

func cliLoadSession(cmd *cobra.Command) (core.SavedSession, error) {
	session, err := core.LoadSession()
	switch err {
	case nil:
	case core.SessionExpiredError:
		if err := runLoginForm(cmd); err != nil {
			return core.SavedSession{}, err
		}
		return core.LoadSession()
	default:
		return core.SavedSession{}, err
	}
	return session, nil

}

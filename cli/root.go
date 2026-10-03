package cli

import (
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)
func NewRootCommandWithVersion(version string) *cobra.Command {

    root := &cobra.Command {
        Use:           "yoel",
		Short:         "A command-line client for Cafe Grader",
		Version: version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
    }
    root.AddGroup(&cobra.Group{
        ID: "question",
        Title: "Question related commands",
    })
    root.AddGroup(&cobra.Group{
        ID: "user",
        Title: "User related commands",
    })
    login := newLoginCommand()
    login.GroupID = "user"
    list := newListCommand()
    fetch := newFetchCommand()
    new := newNewCommand()
    submit := newSubmitCommand()
    list.GroupID = "question"
    fetch.GroupID = "question"
    new.GroupID = "question"
    submit.GroupID = "question"
    root.AddCommand(login, list, fetch, new, submit)
    return root
}

func isTTY(cmd *cobra.Command) bool {
    f, ok := cmd.InOrStdin().(*os.File)
    fout, fok := cmd.OutOrStdout().(*os.File)
    return ok && term.IsTerminal(f.Fd()) && fok && term.IsTerminal(fout.Fd())
}
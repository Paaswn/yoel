package cli

import (
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
    list.GroupID = "question"
    fetch.GroupID = "question"
    new.GroupID = "question"
    root.AddCommand(login, list, fetch, new)
    return root
}


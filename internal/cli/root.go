package cli

import "github.com/spf13/cobra"
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
    return root
}

package cli

import (
	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func newFetchCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "fetch",
        Short: "Fetch the latest questions from the server",
        RunE: func(cmd *cobra.Command, args []string) error {
            reg, err := core.NewRegistry()
            if err != nil {
                return err
            }
            session, err := core.LoadSession()
            if err != nil {
                return err
            }
            return reg.UpdateAllQuestions(session)
        },
    }
}
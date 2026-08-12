package cli

import (
	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

func huhYesNo(command *cobra.Command, title string) (bool, error) {
	accepted := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(&accepted),
		),
	).
		WithInput(command.InOrStdin()).
		WithOutput(command.ErrOrStderr())
	if err := form.RunWithContext(command.Context()); err != nil {
		return false, err
	}
	return accepted, nil
}
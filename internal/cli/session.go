package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var errReloginDeclined = errors.New("saved session expired; run yoel login")

type reloginPrompt func(command *cobra.Command) (bool, error)
type sessionProvider func(command *cobra.Command) (storedSession, error)

func newSessionProvider(runForm loginForm, confirm reloginPrompt) sessionProvider {
	return func(command *cobra.Command) (storedSession, error) {
		session, err := loadStoredSession()
		if err != nil {
			return storedSession{}, err
		}
		if time.Now().Before(session.ExpiresAt) {
			return session, nil
		}

		accepted, err := confirm(command)
		if err != nil {
			return storedSession{}, fmt.Errorf("confirm re-login: %w", err)
		}
		if !accepted {
			return storedSession{}, errReloginDeclined
		}
		return loginAndSave(command, runForm, session.BaseURL)
	}
}

func declineRelogin(*cobra.Command) (bool, error) {
	return false, nil
}

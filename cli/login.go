package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"charm.land/huh/v2"
	gapi "github.com/Paaswn/yoel/graderapi"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

const defaultGraderURL = "https://grader.nattee.net"

func newLoginCommand() *cobra.Command {
    var graderURL string
    command :=  &cobra.Command{
            Use: "login",
            Short: "login to your account",
            Long:  "login to your account, session is saved inside your os keyring",
            RunE: func(command *cobra.Command, _ []string) error {
                return loginAndSaveSession(command, graderURL)
		},
    }
    command.Flags().StringVar(&graderURL, "base-url", defaultGraderURL, "grader API base URL")
    return command
}

func loginAndSaveSession(command *cobra.Command, url string) error {
    client, err := gapi.NewClient(url, nil)
    if err != nil {
        return err;
    }
    username, password, err := runLoginForm(command)
    session, err := client.Login(command.Context(), username, password)
    if err := saveSession(&session); err != nil {
        return err
    }
    _, err = fmt.Fprintln(os.Stderr, "Succesfully Login")
    return err;
}

func saveSession(session *gapi.Session) error {
    dataStruct := SavedSession{
        session.Token,
        session.ExpiresAt,
    }
    data, err := json.Marshal(dataStruct)
    if err != nil {
        return err
    }
    keyring.Set(keyringName, userCode, string(data))
    return nil
}

func runLoginForm(command *cobra.Command) (string, string, error) {
	var username string
	var password string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().EchoMode(huh.EchoModeNormal).Value(&username).Title("Username"),
			huh.NewInput().EchoMode(huh.EchoModePassword).Value(&password).Title("Password"),
		),
	).
	WithInput(command.InOrStdin()).
	WithOutput(command.ErrOrStderr())
	if err := form.RunWithContext(command.Context()); err != nil {
		return "", "", err
	}
	return username, password, nil
}
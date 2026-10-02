package cli

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/zalando/go-keyring"
)


type SavedSession struct {
    Token string `json:"token"`
    Expires time.Time `json:"expire"`
}

const keyringName string = "yoel"
const userCode string = "witcherFour"
func loadSession() ( SavedSession, error ) {
    rawData, err := keyring.Get(keyringName, userCode)
    if err != nil {
        return SavedSession{}, err
    }
    var session SavedSession
    err = json.Unmarshal([]byte(rawData), &session)
    if err != nil {
        return SavedSession{}, err
    }
    if time.Now().After(session.Expires) {
        return SavedSession{}, errors.New("session expired. Run yoel login")
    }
    return session, nil
}
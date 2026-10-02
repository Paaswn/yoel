package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
	"github.com/zalando/go-keyring"
)

const DefaultGraderURL = "https://grader.nattee.net"
const TimeOut = time.Second * 15
func LoginAndSaveSession(url, username, password string, ctx context.Context) error {
    client, err := gapi.NewClient(url, nil)
    if err != nil {
        return err;
    }
    session, err := client.Login(ctx, username, password)
    if err != nil {
        return err
    }
    if err := SaveSession(&session); err != nil {
        return err
    }
    return nil;
}

type SavedSession struct {
    Token string `json:"token"`
    Expires time.Time `json:"expire"`
}

const keyringName string = "yoel"
const userCode string = "witcherFour"

func SaveSession(session *gapi.Session) error {
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

var SessionExpiredError = errors.New("session expired. Run yoel login")
func LoadSession() ( SavedSession, error ) {
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
        return SavedSession{}, SessionExpiredError
    }
    return session, nil
}
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
    ctx, cancel := context.WithTimeout(context.Background(), TimeOut)
       defer cancel()
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

const (
    keyringService string = "yoel"
    keyringAccount string = "yoel-grader-session"
)

func saveData(data string) error {
    return keyring.Set(keyringService, keyringAccount, data)
}
func loadData() (string, error) {
    return keyring.Get(keyringService, keyringAccount)
}
func SaveSession(session *gapi.Session) error {
    dataStruct := SavedSession{
        session.Token,
        session.ExpiresAt,
    }
    data, err := json.Marshal(dataStruct)
    if err != nil {
        return err
    }
    return saveData(string(data))
}

var SessionExpiredError = errors.New("session expired. Run yoel login")
func LoadSession() (SavedSession, error) {
    rawData, err := loadData()
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
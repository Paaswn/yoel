package core

import (
	"context"

	gapi "github.com/Paaswn/yoel/graderapi"
)

func GetAttachment(ctx context.Context, session SavedSession, id int) (gapi.ProblemFile,  error ) {
    client, err := gapi.NewClient(DefaultGraderURL, nil)
    if err != nil {
        return gapi.ProblemFile{}, err
    }
    ctx, cancel := context.WithTimeout(ctx, TimeOut)
    defer cancel()
    return getAttachment(ctx, client.WithToken(session.Token), id)
}

func getAttachment(ctx context.Context, client *gapi.Client, id int) (gapi.ProblemFile,  error ) {
    attachment, err := client.DownloadProblemAttachment(ctx, id)
    if err != nil {
        return gapi.ProblemFile{}, err
    }
    return attachment, nil
}
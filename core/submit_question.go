package core

import (
	"context"
	"path/filepath"

	gapi "github.com/Paaswn/yoel/graderapi"
)
func SubmitQuestion(ctx context.Context, session SavedSession, problem ProblemLite, reg *Registry) ( gapi.Submission, error  ){
    client, err := gapi.NewClient(DefaultGraderURL, nil)
    if err != nil {
        return gapi.Submission{}, err
    }
    ctx, cancel := context.WithTimeout(ctx, TimeOut)
    defer cancel()
    return submitQuestion(ctx, client.WithToken(session.Token), problem)
}

func submitQuestion(ctx context.Context, client *gapi.Client, problem ProblemLite) ( gapi.Submission, error ) {
    name := filepath.Base(problem.SourcePath)
    submission, err := client.Submit(ctx, problem.ID, gapi.SubmissionRequest{
        Source: problem.SourcePath,
        Filename: name,
    })
    if err != nil {
        return gapi.Submission{}, err
    }
    return submission, nil
}
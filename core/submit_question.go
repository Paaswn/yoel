package core

import (
	"context"
	"path/filepath"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
)
func SubmitQuestion(ctx context.Context, session SavedSession, problem ProblemLite, reg *Registry) ( SubmissionResult, error  ){
    client, err := gapi.NewClient(DefaultGraderURL, nil)
    if err != nil {
        return SubmissionResult{}, err
    }
    ctx, cancel := context.WithTimeout(ctx, TimeOut)
    defer cancel()
    return submitQuestion(ctx, client.WithToken(session.Token), problem)
}

func submitQuestion(ctx context.Context, client *gapi.Client, problem ProblemLite) ( SubmissionResult, error ) {
    name := filepath.Base(problem.SourcePath)
    submission, err := client.Submit(ctx, problem.ID, gapi.SubmissionRequest{
        Source: problem.SourcePath,
        Filename: name,
    })
    if err != nil {
        return SubmissionResult{}, err
    }
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for {
        sub, err := client.GetSubmission(ctx, submission.ID)
        if err != nil {
            return SubmissionResult{}, err
        }
        switch sub.Status {
            case "done", "compilation_error", "grader_error":
                return toSubmissionResult(&sub), nil
            default:
        }
        select {
            case <-ctx.Done():
                return SubmissionResult{}, ctx.Err()
            case <-ticker.C:
        }
    }
}

type SubmissionResult struct {
	ID              int
	ProblemID       int
	ProblemName     string
	Language        string
	SubmittedAt     time.Time
	Points          *float64
	Status          string
	GraderComment   *string
	CompilerMessage *string
	MaxRuntime      *float64
	PeakMemory      *int
	Number          int
	Evaluations     []gapi.Evaluation
}

func toSubmissionResult(sub *gapi.Submission) SubmissionResult {
    return SubmissionResult{
        ID:              sub.ID,
        ProblemID:       sub.ProblemID,
        ProblemName:     sub.ProblemName,
        Language:        sub.Language,
        SubmittedAt:     sub.SubmittedAt,
        Points:          sub.Points,
        Status:          sub.Status,
        GraderComment:   sub.GraderComment,
        CompilerMessage: sub.CompilerMessage,
        MaxRuntime:      sub.MaxRuntime,
        PeakMemory:      sub.PeakMemory,
        Number:          sub.Number,
        Evaluations:     sub.Evaluations,
    }
}

func (s *SubmissionResult) Format() string {
    return ""
}
package core

import (
	"context"
	"os"
	"path/filepath"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
)
func SubmitProblem(ctx context.Context, session SavedSession, problem ProblemLite, reg *Registry) ( SubmissionResult, error  ){
    client, err := gapi.NewClient(DefaultGraderURL, nil)
    if err != nil {
        return SubmissionResult{}, err
    }
    ctx, cancel := context.WithTimeout(ctx, TimeOut)
    defer cancel()
    return submitProblem(ctx, client.WithToken(session.Token), problem)
}

func readSourceContentToString(path string) ( string, error  ){
    bytes, err :=  os.ReadFile(path)
    if err != nil {
        return "", err
    }
    return string(bytes), nil
}
func submitProblem(ctx context.Context, client *gapi.Client, problem ProblemLite) ( SubmissionResult, error ) {
    name := filepath.Base(problem.SourcePath)
    content, err := readSourceContentToString(problem.SourcePath)
    if err != nil {
        return SubmissionResult{}, err
    }
    submission, err := client.Submit(ctx, problem.ID, gapi.SubmissionRequest{
        Source: content,
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
            case "done":
                return toSubmissionResult(&sub, Done), nil
            case "compilation_error":
                return toSubmissionResult(&sub, CompilationError), nil
            case"grader_error":
                return toSubmissionResult(&sub, GraderError), nil
            default:
        }
        select {
            case <-ctx.Done():
                return SubmissionResult{}, ctx.Err()
            case <-ticker.C:
        }
    }
}

type Status int
const (
    Done Status = iota
    CompilationError
    GraderError
 )
type SubmissionResult struct {
	ID              int
	ProblemID       int
	ProblemName     string
	Language        string
	SubmittedAt     time.Time
	Points          *float64
	Status          Status
	GraderComment   *string
	CompilerMessage *string
	MaxRuntime      *float64
	PeakMemory      *int
	Number          int
	Evaluations     []gapi.Evaluation
}

func toSubmissionResult(sub *gapi.Submission, status Status) SubmissionResult {
    return SubmissionResult{
        ID:              sub.ID,
        ProblemID:       sub.ProblemID,
        ProblemName:     sub.ProblemName,
        Language:        sub.Language,
        SubmittedAt:     sub.SubmittedAt,
        Points:          sub.Points,
        Status:          status,
        GraderComment:   sub.GraderComment,
        CompilerMessage: sub.CompilerMessage,
        MaxRuntime:      sub.MaxRuntime,
        PeakMemory:      sub.PeakMemory,
        Number:          sub.Number,
        Evaluations:     sub.Evaluations,
    }
}

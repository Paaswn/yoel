package core

import (
	"context"
	"os"
	"path/filepath"
	"time"

	gapi "github.com/Paaswn/yoel/graderapi"
)

func SubmitProblem(ctx context.Context, session SavedSession, problem ProblemLite, reg *Registry) (SubmissionResult, error) {
	client, err := gapi.NewClient(DefaultGraderURL, nil)
	if err != nil {
		return SubmissionResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	return submitProblem(ctx, client.WithToken(session.Token), problem, reg)
}

func readSourceContentToString(path string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func submitProblem(ctx context.Context, client *gapi.Client, problem ProblemLite, reg *Registry) (SubmissionResult, error) {
	name := filepath.Base(problem.SourcePath)
	content, err := readSourceContentToString(problem.SourcePath)
	if err != nil {
		return SubmissionResult{}, err
	}
	submission, err := client.Submit(ctx, problem.ID, gapi.SubmissionRequest{
		Source:   content,
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
			tList, err := client.ListTestcases(ctx, problem.ID)
			// if any ListTestcases erorr occur just return submission result without testcaseList
			if err != nil {
				submissionResult := toSubmissionResult(&sub, Done, tList)
				setBestScore(reg, &problem, &submissionResult)
				return submissionResult, nil
			}
			submissionResult := toSubmissionResult(&sub, Done, tList)
			setBestScore(reg, &problem, &submissionResult)
			return submissionResult, nil
		case "compilation_error":
			return toSubmissionResult(&sub, CompilationError, []gapi.Testcase{}), nil
		case "grader_error":
			return toSubmissionResult(&sub, GraderError, []gapi.Testcase{}), nil
		default:
		}
		select {
		case <-ctx.Done():
			return SubmissionResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
func setBestScore(reg *Registry, problem *ProblemLite, submission *SubmissionResult) {
	if submission.Points != nil {
		// Keep returning the grader result even if the optional local cache
		// cannot be updated, as with the previous best-score-only cache.
		_ = reg.RecordProblemResult(problem.ID, *submission.Points, submission.GraderComment, submission.SubmittedAt)
	}
}

type Status int

const (
	Done Status = iota
	CompilationError
	GraderError
)

type SubmissionResult struct {
	ID              int             `json:"id"`
	ProblemID       int             `json:"problem_id"`
	ProblemName     string          `json:"problem_name"`
	Language        string          `json:"language"`
	SubmittedAt     time.Time       `json:"submitted_at"`
	Points          *float64        `json:"points"`
	Status          Status          `json:"status"`
	GraderComment   *string         `json:"grader_comment"`
	CompilerMessage *string         `json:"compiler_message"`
	MaxRuntime      *float64        `json:"max_runtime"`
	PeakMemory      *int            `json:"peak_memory"`
	Number          int             `json:"number"`
	TestCaseResults []gapi.Testcase `json:"test_case_results"`
}

func toSubmissionResult(sub *gapi.Submission, status Status, testResults []gapi.Testcase) SubmissionResult {
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
		TestCaseResults: testResults,
	}
}

type TestCaseResult struct {
	Group      int    `json:"group"`
	TestcaseID int    `json:"testcase_id"`
	Result     string `json:"result"`
}

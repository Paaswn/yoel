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
				testcaseList, groupAmount := toTestCaseResult(sub.Evaluations, []gapi.Testcase{})
				submissionResult := toSubmissionResult(&sub, Done, testcaseList, groupAmount)
				setBestScore(reg, &problem, &submissionResult)
				return submissionResult, nil
			}
			testcaseList, groupAmount := toTestCaseResult(sub.Evaluations, tList)
			submissionResult := toSubmissionResult(&sub, Done, testcaseList, groupAmount)
			setBestScore(reg, &problem, &submissionResult)
			return submissionResult, nil
		case "compilation_error":
			return toSubmissionResult(&sub, CompilationError, []TestCaseResult{}, 0), nil
		case "grader_error":
			return toSubmissionResult(&sub, GraderError, []TestCaseResult{}, 0), nil
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
	if *submission.Points > problem.BestScore {
		reg.UpdateProblemScore(problem.ID, *submission.Points)
	}
}

type Status int

const (
	Done Status = iota
	CompilationError
	GraderError
)

type SubmissionResult struct {
	ID              int              `json:"id"`
	ProblemID       int              `json:"problem_id"`
	ProblemName     string           `json:"problem_name"`
	Language        string           `json:"language"`
	SubmittedAt     time.Time        `json:"submitted_at"`
	Points          *float64         `json:"points"`
	Status          Status           `json:"status"`
	GraderComment   *string          `json:"grader_comment"`
	CompilerMessage *string          `json:"compiler_message"`
	MaxRuntime      *float64         `json:"max_runtime"`
	PeakMemory      *int             `json:"peak_memory"`
	Number          int              `json:"number"`
	TestCaseResults []TestCaseResult `json:"test_case_results"`
	TestGroupAmount int              `json:"test_group_amount"`
}



func toSubmissionResult(sub *gapi.Submission, status Status, testResults []TestCaseResult, groupAmount int) SubmissionResult {
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
		TestGroupAmount: groupAmount,
	}
}

type TestCaseResult struct {
	Group      int    `json:"group"`
	TestcaseID int    `json:"testcase_id"`
	Result     string `json:"result"`
}

func toTestCaseResult(evals []gapi.Evaluation, testCases []gapi.Testcase) ([]TestCaseResult, int) {
	cases := make(map[int]int, len(testCases))
	for i, t := range testCases {
		if t.Group == nil {
			cases[t.ID] = i + 1
		} else {
			cases[t.ID] = *t.Group
		}
	}
	results := make([]TestCaseResult, len(evals))
	groupAmount := -1
	for i, e := range evals {
		results[i].TestcaseID = e.TestcaseID
		if e.Result == nil {
			results[i].Result = ""
		} else {
			results[i].Result = *e.Result
		}
		if len(testCases) == 0 {
			results[i].Group = i + 1
		} else {
			results[i].Group = cases[e.TestcaseID]
		}
		groupAmount = max(results[i].Group, groupAmount)
	}
	return results, groupAmount
}

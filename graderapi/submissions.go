package graderapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SubmissionRequest contains source code to submit for a problem. A zero
// LanguageID is omitted so the grader can detect the language from Filename.
type SubmissionRequest struct {
	Source     string `json:"source"`
	Filename   string `json:"filename"`
	LanguageID int    `json:"language_id,omitempty"`
}

// Submission contains an acknowledgement or detailed grading result. Fields
// that are unavailable while judging is in progress remain nil or empty.
type Submission struct {
	ID              int          `json:"id"`
	ProblemID       int          `json:"problem_id"`
	ProblemName     string       `json:"problem_name"`
	Language        string       `json:"language"`
	SubmittedAt     time.Time    `json:"submitted_at"`
	Points          *float64     `json:"points"`
	Status          string       `json:"status"`
	GraderComment   *string      `json:"grader_comment"`
	CompilerMessage *string      `json:"compiler_message"`
	MaxRuntime      *float64     `json:"max_runtime"`
	PeakMemory      *int         `json:"peak_memory"`
	Number          int          `json:"number"`
	Evaluations     []Evaluation `json:"evaluations"`
}

// Evaluation contains one testcase result from a detailed submission.
type Evaluation struct {
	TestcaseID int      `json:"testcase_id"`
	Result     *string  `json:"result"`
	Score      *float64 `json:"score"`
	Time       *int     `json:"time"`
	Memory     *int     `json:"memory"`
}

// Submit creates one submission and returns the grader acknowledgement. It
// does not retry or poll for a final result.
func (c *Client) Submit(ctx context.Context, problemID int, req SubmissionRequest) (Submission, error) {
	if problemID <= 0 {
		return Submission{}, fmt.Errorf("submit: %w: problem ID must be positive", ErrInvalidInput)
	}
	if req.Source == "" {
		return Submission{}, fmt.Errorf("submit: %w: source is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.Filename) == "" {
		return Submission{}, fmt.Errorf("submit: %w: filename is required", ErrInvalidInput)
	}
	if req.LanguageID < 0 {
		return Submission{}, fmt.Errorf("submit: %w: language ID cannot be negative", ErrInvalidInput)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Submission{}, fmt.Errorf("submit: %w: could not encode request", ErrInvalidInput)
	}
	responseBody, err := c.do(
		ctx,
		"submit",
		http.MethodPost,
		fmt.Sprintf("/api/v1/problems/%d/submissions", problemID),
		bytes.NewReader(body),
		true,
	)
	if err != nil {
		return Submission{}, err
	}

	var response struct {
		ID     *int    `json:"id"`
		Number *int    `json:"number"`
		Status *string `json:"status"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return Submission{}, fmt.Errorf("submit: %w: malformed response", ErrInvalidResponse)
	}
	if response.ID == nil || *response.ID <= 0 || response.Number == nil || *response.Number <= 0 || response.Status == nil || strings.TrimSpace(*response.Status) == "" {
		return Submission{}, fmt.Errorf("submit: %w: response is missing required fields", ErrInvalidResponse)
	}

	return Submission{
		ID:     *response.ID,
		Number: *response.Number,
		Status: *response.Status,
	}, nil
}

// GetSubmission retrieves the current state of one submission. It performs one
// request and does not poll.
func (c *Client) GetSubmission(ctx context.Context, submissionID int) (Submission, error) {
	if submissionID <= 0 {
		return Submission{}, fmt.Errorf("get submission: %w: submission ID must be positive", ErrInvalidInput)
	}

	responseBody, err := c.do(
		ctx,
		"get submission",
		http.MethodGet,
		fmt.Sprintf("/api/v1/submissions/%d", submissionID),
		nil,
		true,
	)
	if err != nil {
		return Submission{}, err
	}

	var submission Submission
	if err := json.Unmarshal(responseBody, &submission); err != nil {
		return Submission{}, fmt.Errorf("get submission: %w: malformed response", ErrInvalidResponse)
	}
	if submission.ID <= 0 || submission.ProblemID <= 0 || strings.TrimSpace(submission.Language) == "" || submission.SubmittedAt.IsZero() {
		return Submission{}, fmt.Errorf("get submission: %w: response is missing required fields", ErrInvalidResponse)
	}
	return submission, nil
}

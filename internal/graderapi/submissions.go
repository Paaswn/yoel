package graderapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SubmissionRequest contains source code to submit for a problem. A zero
// LanguageID is omitted so the grader can detect the language from Filename.
type SubmissionRequest struct {
	Source     string `json:"source"`
	Filename   string `json:"filename"`
	LanguageID int    `json:"language_id,omitempty"`
}

// Submission is the acknowledgement returned when the grader queues a source
// submission. It is not a final grading result.
type Submission struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	Status string `json:"status"`
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

package graderapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Problem contains the fields returned by the problem-list endpoint.
type Problem struct {
	ID                 int               `json:"id"`
	Difficulty         *int              `json:"difficulty"`
	Tags               []string          `json:"tags"`
	BestScore          *float64          `json:"best_score"`
	LastScore          *float64          `json:"last_score"`
	SubmissionCount    int               `json:"submission_count"`
	LastResult         *string           `json:"last_result"`
	LastSubmissionTime *time.Time        `json:"last_submission_time"`
	LastSubmissionID   *int              `json:"last_submission_id"`
	HasTestcase        bool              `json:"has_testcase"`
	HasAttachment      bool              `json:"has_attachment"`
	PermittedLanguages []ProblemLanguage `json:"permitted_languages"`
	Name               string            `json:"name"`
	FullName           string            `json:"full_name"`
}

// ProblemLanguage identifies a language permitted for a problem.
type ProblemLanguage struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Ext  string `json:"ext"`
}

func (c *Client) ListProblems(ctx context.Context) ([]Problem, error) {
	body, err := c.do(ctx, "list problems", http.MethodGet, "/api/v1/problems", nil, true)
	if err != nil {
		return nil, err
	}
	var problems []Problem
	if err := json.Unmarshal(body, &problems); err != nil {
		return nil, fmt.Errorf("list problems: %w: malformed response", ErrInvalidResponse)
	}
	return problems, nil
}

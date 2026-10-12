package graderapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

const maxTestcaseFileSize int64 = 4 << 20

// Testcase contains the metadata returned for a problem's testcase.
// ID identifies the testcase globally; Num is its display number in the problem.
type Testcase struct {
	ID        int     `json:"id"`
	Num       int     `json:"num"`
	Group     *int    `json:"group"`
	GroupName *string `json:"group_name"`
	Weight    *int    `json:"weight"`
}

// ListTestcases returns testcase metadata in the order provided by the grader.
func (c *Client) ListTestcases(ctx context.Context, problemID int) ([]Testcase, error) {
	const operation = "list testcases"
	if problemID <= 0 {
		return nil, fmt.Errorf("%s: %w: problem ID must be positive", operation, ErrInvalidInput)
	}
	apiPath := "/api/v1/problems/" + strconv.Itoa(problemID) + "/testcases"
	body, err := c.do(ctx, operation, http.MethodGet, apiPath, nil, true)
	if err != nil {
		return nil, err
	}
	var testcases []Testcase
	if err := json.Unmarshal(body, &testcases); err != nil {
		return nil, fmt.Errorf("%s: %w: malformed response", operation, ErrInvalidResponse)
	}
	return testcases, nil
}

// DownloadTestcaseInput returns the raw input exposed for a testcase.
func (c *Client) DownloadTestcaseInput(ctx context.Context, testcaseID int) ([]byte, error) {
	return c.downloadTestcaseFile(ctx, testcaseID, "input", "download testcase input")
}

// DownloadTestcaseSolution returns the raw expected output exposed for a testcase.
func (c *Client) DownloadTestcaseSolution(ctx context.Context, testcaseID int) ([]byte, error) {
	return c.downloadTestcaseFile(ctx, testcaseID, "sol", "download testcase solution")
}

func (c *Client) downloadTestcaseFile(ctx context.Context, testcaseID int, suffix, operation string) ([]byte, error) {
	if testcaseID <= 0 {
		return nil, fmt.Errorf("%s: %w: testcase ID must be positive", operation, ErrInvalidInput)
	}

	apiPath := "/api/v1/testcases/" + strconv.Itoa(testcaseID) + "/" + suffix
	request, err := c.newRequest(ctx, http.MethodGet, apiPath, nil, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	request.Header.Set("Accept", "text/plain")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s: %w", operation, &HTTPError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
		})
	}

	data, err := readBoundedLimit(response.Body, maxTestcaseFileSize)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return data, nil
}

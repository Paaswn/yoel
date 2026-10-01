package graderapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
)

const maxStatementPDFSize = 32 << 20

// GetProblemStatementPDF downloads the PDF statement from the grader's web
// statement route. The returned bytes are bounded and validated as a PDF.
func (c *Client) GetProblemStatementPDF(ctx context.Context, problemID int) ([]byte, error) {
	if problemID <= 0 {
		return nil, fmt.Errorf("get problem statement PDF: %w: problem ID must be positive", ErrInvalidInput)
	}
	path := "/problems/" + strconv.Itoa(problemID) + "/download/statement"
	request, err := c.newRequest(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return nil, fmt.Errorf("get problem statement PDF: %w", err)
	}
	request.Header.Set("Accept", "application/pdf")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("get problem statement PDF: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("get problem statement PDF: %w", &HTTPError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
		})
	}

	pdf, err := readBoundedLimit(response.Body, maxStatementPDFSize)
	if err != nil {
		return nil, fmt.Errorf("get problem statement PDF: %w", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return nil, fmt.Errorf("get problem statement PDF: %w: response is not a PDF", ErrInvalidResponse)
	}
	return pdf, nil
}

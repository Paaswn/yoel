package graderapi

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxProblemPDFSize        int64 = 32 << 20
	maxProblemAttachmentSize int64 = 64 << 20
)

// ProblemFile is a file returned by a grader problem-file endpoint.
type ProblemFile struct {
	Data        []byte
	Filename    string
	ContentType string
}

// DownloadProblemPDF downloads a problem's PDF description.
func (c *Client) DownloadProblemPDF(ctx context.Context, problemID int) (ProblemFile, error) {
	const operation = "download problem PDF"

	if problemID <= 0 {
		return ProblemFile{}, fmt.Errorf("%s: problem ID must be positive: %w", operation, ErrInvalidInput)
	}

	endpoint := "/api/v1/problems/" + strconv.Itoa(problemID) + "/files/pdf"
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil, true)
	if err != nil {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, err)
	}
	req.Header.Set("Accept", "application/pdf")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		})
	}

	data, err := readBoundedLimit(resp.Body, maxProblemPDFSize)
	if err != nil {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return ProblemFile{}, fmt.Errorf("%s: response is not a PDF: %w", operation, ErrInvalidResponse)
	}

	return ProblemFile{
		Data:        data,
		Filename:    problemPDFFilename(resp.Header.Get("Content-Disposition"), problemID),
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

// DownloadProblemAttachment downloads a problem's attachment archive.
func (c *Client) DownloadProblemAttachment(ctx context.Context, problemID int) (ProblemFile, error) {
	const operation = "download problem attachment"

	if problemID <= 0 {
		return ProblemFile{}, fmt.Errorf("%s: problem ID must be positive: %w", operation, ErrInvalidInput)
	}

	endpoint := "/api/v1/problems/" + strconv.Itoa(problemID) + "/files/attachment"
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil, true)
	if err != nil {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, err)
	}
	req.Header.Set("Accept", "application/zip, application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		})
	}

	data, err := readBoundedLimit(resp.Body, maxProblemAttachmentSize)
	if err != nil {
		return ProblemFile{}, fmt.Errorf("%s: %w", operation, err)
	}
	if len(data) == 0 {
		return ProblemFile{}, fmt.Errorf("%s: response is empty: %w", operation, ErrInvalidResponse)
	}

	return ProblemFile{
		Data:        data,
		Filename:    problemAttachmentFilename(resp.Header.Get("Content-Disposition"), problemID),
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

func problemPDFFilename(contentDisposition string, problemID int) string {
	return problemFileName(contentDisposition, "problem-"+strconv.Itoa(problemID)+".pdf")
}

func problemAttachmentFilename(contentDisposition string, problemID int) string {
	return problemFileName(contentDisposition, "problem-"+strconv.Itoa(problemID)+"-attachment.zip")
}

func problemFileName(contentDisposition, fallback string) string {
	if contentDisposition == "" {
		return fallback
	}

	_, params, err := mime.ParseMediaType(contentDisposition)
	if err != nil {
		return fallback
	}

	filename := strings.ReplaceAll(params["filename"], `\`, "/")
	filename = path.Base(filename)
	filename = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, filename)
	filename = strings.TrimSpace(filename)
	if filename == "" || filename == "." || filename == ".." {
		return fallback
	}

	return filename
}

package graderapi

import (
	"errors"
	"fmt"
)

var (
	ErrAuthentication  = errors.New("grader authentication failed")
	ErrInvalidInput    = errors.New("invalid grader API input")
	ErrInvalidResponse = errors.New("invalid grader API response")
)

// HTTPError describes a non-successful HTTP response without retaining its
// response body. The body may contain credentials, source code, or other
// sensitive data and is therefore never included in this error.
type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "grader API returned an HTTP error"
	}
	return fmt.Sprintf("grader API returned %s", e.Status)
}

func (e *HTTPError) Unwrap() error {
	if e != nil && (e.StatusCode == 401 || e.StatusCode == 403) {
		return ErrAuthentication
	}
	return nil
}

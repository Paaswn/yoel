package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

type commandError struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Details *commandErrorDetails `json:"details,omitempty"`
}

type commandErrorDetails struct {
	DirectoryPath string `json:"directory_path"`
}

// WriteCommandError preserves human-readable errors unless the failed command
// opted into JSON. JSON failures use stderr, leaving stdout for success values.
func WriteCommandError(w io.Writer, cmd *cobra.Command, err error) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if !jsonOutput {
		_, writeErr := fmt.Fprintln(w, err)
		return writeErr
	}
	code := "command_failed"
	switch {
	case errors.Is(err, core.ProblemExistErr):
		code = "problem_exists"
	case errors.Is(err, core.SessionExpiredError):
		code = "session_expired"
	case errors.Is(err, core.ErrProblemNotFound):
		code = "problem_not_found"
	case errors.Is(err, context.DeadlineExceeded):
		code = "timeout"
	case errors.Is(err, context.Canceled):
		code = "cancelled"
	}
	var details *commandErrorDetails
	var exists *core.ProblemExistsError
	if errors.As(err, &exists) {
		details = &commandErrorDetails{DirectoryPath: exists.DirectoryPath}
	}
	return json.NewEncoder(w).Encode(struct {
		Error commandError `json:"error"`
	}{Error: commandError{Code: code, Message: err.Error(), Details: details}})
}

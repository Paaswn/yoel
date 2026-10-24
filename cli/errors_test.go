package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Paaswn/yoel/core"
	"github.com/spf13/cobra"
)

func TestWriteCommandError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"exists", core.ProblemExistErr, "problem_exists"},
		{"expired", core.SessionExpiredError, "session_expired"},
		{"not found", core.ErrProblemNotFound, "problem_not_found"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"cancelled", context.Canceled, "cancelled"},
		{"other", errors.New("unexpected failure"), "command_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("json", true, "")
			err := fmt.Errorf("context: %w", tc.err)
			var buf bytes.Buffer
			if writeErr := WriteCommandError(&buf, cmd, err); writeErr != nil {
				t.Fatal(writeErr)
			}
			var response struct {
				Error commandError `json:"error"`
			}
			if decodeErr := json.Unmarshal(buf.Bytes(), &response); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if response.Error.Code != tc.code || response.Error.Message != err.Error() {
				t.Fatalf("unexpected response: %+v", response)
			}
		})
	}
}

func TestWriteCommandErrorHumanOutput(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	if err := WriteCommandError(&buf, cmd, core.ProblemExistErr); err != nil {
		t.Fatal(err)
	}
	if buf.String() != core.ProblemExistErr.Error()+"\n" {
		t.Fatalf("human output changed: %q", buf.String())
	}
}

func TestProblemExistsErrorDetails(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", true, "")
	err := fmt.Errorf("cannot create problem: %w", &core.ProblemExistsError{DirectoryPath: "/workspace/sample"})
	var buf bytes.Buffer
	if writeErr := WriteCommandError(&buf, cmd, err); writeErr != nil {
		t.Fatal(writeErr)
	}
	var response struct {
		Error commandError `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "problem_exists" || response.Error.Details == nil ||
		response.Error.Details.DirectoryPath != "/workspace/sample" {
		t.Fatalf("lost destination details: %+v", response)
	}
}

func TestJSONArgumentError(t *testing.T) {
	root := NewRootCommandWithVersion("test")
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"new", "--json"})
	cmd, err := root.ExecuteC()
	if err == nil {
		t.Fatal("expected missing argument error")
	}
	if writeErr := WriteCommandError(&stderr, cmd, err); writeErr != nil {
		t.Fatal(writeErr)
	}
	if stdout.Len() != 0 || !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), `"code":"command_failed"`) {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

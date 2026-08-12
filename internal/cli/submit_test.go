package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
)

func TestProblemIDFromFilename(t *testing.T) {
	for name, test := range map[string]struct {
		filename string
		want     int
	}{
		"C++":              {"673.cpp", 673},
		"Python in folder": {filepath.Join("solutions", "1142.py"), 1142},
		"Rust":             {"900.rs", 900},
		"leading zeroes":   {"0007.go", 7},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := problemIDFromFilename(test.filename)
			if err != nil {
				t.Fatalf("problemIDFromFilename(%q): %v", test.filename, err)
			}
			if got != test.want {
				t.Fatalf("problemIDFromFilename(%q) = %d, want %d", test.filename, got, test.want)
			}
		})
	}
}

func TestProblemIDFromFilenameRejectsInvalidNames(t *testing.T) {
	for _, filename := range []string{
		"solution.cpp",
		"abc.cpp",
		"673",
		"673.extra.cpp",
		"0.cpp",
		"-1.cpp",
		"673.",
		".cpp",
		"673..cpp",
	} {
		t.Run(filename, func(t *testing.T) {
			if _, err := problemIDFromFilename(filename); !errors.Is(err, errInvalidSubmissionFilename) {
				t.Fatalf("problemIDFromFilename(%q) error = %v, want errInvalidSubmissionFilename", filename, err)
			}
		})
	}
}

func TestQuestionSubmitReadsFileAndShowsResult(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	const source = "package main\n\nfunc main() { println(\"สวัสดี\") }\n"
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "673.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	var resultRequests atomic.Int32
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/problems/673/submissions":
			if r.Method != http.MethodPost {
				t.Errorf("submit method = %s", r.Method)
			}
			var request map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			var gotSource, filename string
			if err := json.Unmarshal(request["source"], &gotSource); err != nil {
				t.Errorf("decode source: %v", err)
			}
			if err := json.Unmarshal(request["filename"], &filename); err != nil {
				t.Errorf("decode filename: %v", err)
			}
			if gotSource != source || filename != "673.go" {
				t.Errorf("source = %q, filename = %q", gotSource, filename)
			}
			if _, exists := request["language_id"]; exists {
				t.Errorf("request includes language_id: %s", request["language_id"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintln(w, `{"id":924618,"number":3,"status":"submitted"}`)
		case "/api/v1/submissions/924618":
			resultRequests.Add(1)
			if r.Method != http.MethodGet {
				t.Errorf("result method = %s", r.Method)
			}
			_, _ = fmt.Fprintln(w, `{
				"id":924618,"problem_id":673,"problem_name":"hello","language":"go",
				"submitted_at":"2030-01-02T03:04:05Z","points":75.5,"status":"done",
				"grader_comment":"partial score","compiler_message":null,
				"max_runtime":0.125,"peak_memory":2048,"number":3,
				"evaluations":[{"testcase_id":11,"result":"correct","score":25.5,"time":12,"memory":256}]
			}`)
		default:
			t.Errorf("unexpected request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := saveSession(storedSession{
		BaseURL:   server.URL,
		Token:     "fake-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      graderapi.User{ID: 7, Login: "fake-login", FullName: "Fake Student"},
	}); err != nil {
		t.Fatal(err)
	}

	command := newRootCommand(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("submit unexpectedly prompted for credentials")
		return "", "", nil
	})
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"submit", sourcePath, "--long"})
	if err := command.Execute(); err != nil {
		t.Fatalf("submit: %v", err)
	}
	for _, expected := range []string{
		"Judging complete",
		"ID       924618",
		"Attempt  3",
		"Status   done",
		"Language go",
		"Score    75.5",
		"Grader comment",
		"partial score",
		"Testcase 11 · correct · score 25.5 · time 12 · memory 256",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output = %q, want %q", output.String(), expected)
		}
	}
	if resultRequests.Load() != 1 {
		t.Fatalf("result requests = %d, want 1", resultRequests.Load())
	}
	for _, secret := range []string{"fake-token", source} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("output exposes %q", secret)
		}
	}
}

func TestWaitForSubmissionPollsUntilTerminalStatus(t *testing.T) {
	var requests atomic.Int32
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)
		status := "evaluating"
		compilerMessage := "null"
		if requestNumber == 3 {
			status = "compilation_error"
			compilerMessage = `"main.cpp:1: error: expected ';'"`
		}
		_, _ = fmt.Fprintf(w, `{
			"id":924618,"problem_id":673,"language":"cpp",
			"submitted_at":"2030-01-02T03:04:05Z","points":null,
			"status":%q,"compiler_message":%s,"number":3,"evaluations":[]
		}`, status, compilerMessage)
	}))
	defer server.Close()

	client, err := graderapi.NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := waitForSubmission(context.Background(), client.WithToken("fake-token"), 924618, time.Millisecond, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Status != "compilation_error" || submission.CompilerMessage == nil {
		t.Fatalf("submission = %#v", submission)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
}

func TestWaitForSubmissionTimesOut(t *testing.T) {
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, `{
			"id":924618,"problem_id":673,"language":"cpp",
			"submitted_at":"2030-01-02T03:04:05Z","status":"evaluating"
		}`)
	}))
	defer server.Close()

	client, err := graderapi.NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = waitForSubmission(context.Background(), client.WithToken("fake-token"), 924618, time.Millisecond, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestWaitForSubmissionHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		_, _ = fmt.Fprintln(w, `{
			"id":924618,"problem_id":673,"language":"cpp",
			"submitted_at":"2030-01-02T03:04:05Z","status":"evaluating"
		}`)
	}))
	defer server.Close()

	client, err := graderapi.NewClient(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, waitErr := waitForSubmission(ctx, client.WithToken("fake-token"), 924618, time.Hour, time.Minute)
		result <- waitErr
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waitForSubmission did not observe cancellation")
	}
}

func TestRenderSubmissionResultShowsCompilerError(t *testing.T) {
	message := "main.cpp:1: error: expected ';'\n    return 0"
	output := renderSubmissionResult(true,
	 graderapi.Submission{
		ID:              924618,
		Number:          3,
		Language:        "cpp",
		Status:          "compilation_error",
		CompilerMessage: &message,
	})
	for _, expected := range []string{"✗ Compilation failed", "Status   compilation_error", "Score    -", "Compiler message", "expected ';'"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output = %q, want %q", output, expected)
		}
	}
}

func TestSubmitCommandRejectsInvalidFilenameBeforeLoadingSession(t *testing.T) {
	var sessionCalls int
	command := newSubmitCommand(func(*cobra.Command) (storedSession, error) {
		sessionCalls++
		return storedSession{}, nil
	})
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs([]string{"solution.cpp"})
	if err := command.Execute(); !errors.Is(err, errInvalidSubmissionFilename) {
		t.Fatalf("error = %v, want errInvalidSubmissionFilename", err)
	}
	if sessionCalls != 0 {
		t.Fatalf("session provider called %d times, want 0", sessionCalls)
	}
}

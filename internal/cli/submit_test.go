package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func TestQuestionSubmitReadsFileAndShowsAcknowledgement(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	const source = "package main\n\nfunc main() { println(\"สวัสดี\") }\n"
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "673.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/problems/673/submissions" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-token" {
			t.Errorf("Authorization = %q", got)
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
	command.SetArgs([]string{"question", "submit", sourcePath})
	if err := command.Execute(); err != nil {
		t.Fatalf("submit: %v", err)
	}
	for _, expected := range []string{"✓ Submission created", "ID      924618", "Attempt 3", "Status  submitted"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output = %q, want %q", output.String(), expected)
		}
	}
	for _, secret := range []string{"fake-token", source} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("output exposes %q", secret)
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

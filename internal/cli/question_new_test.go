package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
)

func TestQuestionNewCreatesQuestionByListOrder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	server := newCLITestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/problems":
			_, _ = fmt.Fprintln(w, `[
				{"id":673,"name":"CPP-Basics-1","full_name":"CPP Basics 1","has_attachment":false},
				{"id":674,"name":"CPP-Basics-2","full_name":"CPP Basics 2","has_attachment":false}
			]`)
		case "/api/v1/problems/674/files/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = fmt.Fprintln(w, "%PDF-1.7")
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

	command := newRootCommandWithOpener(func(_ *cobra.Command) (string, string, error) {
		t.Fatal("question new unexpectedly prompted for credentials")
		return "", "", nil
	}, func(string) error { return nil })
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs([]string{"question", "new", "2", "--language", "go"})
	if err := command.Execute(); err != nil {
		t.Fatalf("question new by order: %v", err)
	}

	problemDir := filepath.Join(workingDirectory, "CPP-Basics-2")
	if _, err := os.Stat(filepath.Join(problemDir, "674.go")); err != nil {
		t.Fatalf("stat second question source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(problemDir, "Read.pdf")); err != nil {
		t.Fatalf("stat second question statement: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, "CPP-Basics-1")); !os.IsNotExist(err) {
		t.Fatalf("first question was unexpectedly created: %v", err)
	}
}

func TestQuestionNewRequiresEitherOrderOrID(t *testing.T) {
	for name, args := range map[string][]string{
		"missing selection": nil,
		"order and ID":      {"2", "--id", "674"},
		"zero order":        {"0"},
		"negative order":    {"--", "-1"},
		"non-integer order": {"second"},
		"too many orders":   {"1", "2"},
		"zero ID":           {"--id", "0"},
		"negative ID":       {"--id", "-1"},
	} {
		t.Run(name, func(t *testing.T) {
			sessionCalls := 0
			command := newQuestionNewCommand(func(string) error { return nil }, func(*cobra.Command) (storedSession, error) {
				sessionCalls++
				return storedSession{}, nil
			})
			command.SilenceErrors = true
			command.SilenceUsage = true
			command.SetOut(new(bytes.Buffer))
			command.SetErr(new(bytes.Buffer))
			command.SetArgs(args)
			err := command.Execute()
			if err == nil {
				t.Fatal("question new succeeded, want validation error")
			}

			if sessionCalls != 0 {
				t.Fatalf("session provider called %d times, want 0", sessionCalls)
			}
		})
	}
}

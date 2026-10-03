package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gapi "github.com/Paaswn/yoel/graderapi"
)

func TestCreateQuestionNoAttachment(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 42, CodeName: "sample", HasAttachment: false}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	client := newPDFTestClient(t, func(id int) []byte {
		if id != problem.ID {
			t.Errorf("requested problem ID = %d, want %d", id, problem.ID)
		}
		return []byte("%PDF-1.7 test content")
	})

	if err := createQuestion(context.Background(), cwd, problem, client,  r); err != nil {
		t.Fatalf("createQuestion() error = %v", err)
	}

	questionDir := filepath.Join(cwd, problem.CodeName)
	assertFileContents(t, filepath.Join(questionDir, "main.cpp"), []byte(yoelSourceFile), 0o755)
	assertFileContents(t, filepath.Join(questionDir, "42.pdf"), []byte("%PDF-1.7 test content"), 0o444)
	info, err := os.Stat(filepath.Join(questionDir, yoelHiddenDir))
	if err != nil {
		t.Fatalf("stat hidden directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", yoelHiddenDir)
	}

	got, err := r.QueryByID(problem.ID)
	if err != nil {
		t.Fatalf("QueryByID() error = %v", err)
	}
	if got.DirectoryPath != questionDir {
		t.Errorf("DirectoryPath = %q, want %q", got.DirectoryPath, questionDir)
	}
}

func TestCreateQuestionDoesNotReplaceExistingQuestion(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 7, CodeName: "existing"}
	r := newRegistryForTest(t)
	if err := r.Upsert(problem); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	destination := filepath.Join(cwd, problem.CodeName)
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := createQuestion(t.Context(), cwd, problem, newPDFTestClient(t, func(int) []byte {
		return []byte("%PDF-1.7 test content")
	}) ,  r)
	if err == nil {
		t.Fatal("createQuestion() error = nil, want error when destination exists")
	}
	assertFileContents(t, marker, []byte("keep"), 0o644)

	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatalf("read working directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != problem.CodeName {
		t.Fatalf("working directory entries = %v, want only %q", entries, problem.CodeName)
	}
}

func newPDFTestClient(t *testing.T, pdf func(int) []byte) *gapi.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("request method = %q, want GET", r.Method)
		}
		const prefix = "/api/v1/problems/"
		if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/files/pdf") {
			t.Errorf("request path = %q, want problem PDF endpoint", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		idText := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/files/pdf")
		id, err := strconv.Atoi(idText)
		if err != nil {
			t.Errorf("problem ID in request path = %q: %v", idText, err)
			http.Error(w, "invalid problem ID", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%d.pdf"`, id))
		_, _ = w.Write(pdf(id))
	}))
	t.Cleanup(server.Close)

	client, err := gapi.NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func assertFileContents(t *testing.T, path string, want []byte, wantMode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("contents of %s = %q, want %q", path, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if gotMode := info.Mode().Perm(); gotMode != wantMode {
		t.Errorf("permissions of %s = %#o, want %#o", path, gotMode, wantMode)
	}
}

func assertFileMode(t *testing.T, path string, wantMode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if gotMode := info.Mode().Perm(); gotMode != wantMode {
		t.Errorf("permissions of %s = %#o, want %#o", path, gotMode, wantMode)
	}
}

package core

import (
	"os"
	"path/filepath"
	"testing"

	gapi "github.com/Paaswn/yoel/graderapi"
)

func TestCreateQuestionWithPDF(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 42, CodeName: "sample"}
	pdf := gapi.ProblemFile{Data: []byte("%PDF-1.7 test content"), Filename: "problem.pdf"}

	if err := createQuestionWithPDF(cwd, problem, pdf); err != nil {
		t.Fatalf("createQuestionWithPDF() error = %v", err)
	}

	questionDir := filepath.Join(cwd, problem.CodeName)
	assertFileMode(t, filepath.Join(questionDir, "42.id"), 0o000)
	assertFileContents(t, filepath.Join(questionDir, pdf.Filename), pdf.Data, 0o444)
	info, err := os.Stat(filepath.Join(questionDir, yoelHiddenDir))
	if err != nil {
		t.Fatalf("stat hidden directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", yoelHiddenDir)
	}
}

func TestCreateQuestionWithPDFDoesNotReplaceExistingQuestion(t *testing.T) {
	cwd := t.TempDir()
	problem := ProblemLite{ID: 7, CodeName: "existing"}
	destination := filepath.Join(cwd, problem.CodeName)
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := createQuestionWithPDF(cwd, problem, gapi.ProblemFile{
		Data: []byte("pdf"), Filename: "problem.pdf",
	})
	if err == nil {
		t.Fatal("createQuestionWithPDF() error = nil, want error when destination exists")
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

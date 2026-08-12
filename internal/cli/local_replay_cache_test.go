package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"yoel/internal/runner"
)

func TestLocalReplayCacheWritesAndReadsSeparateDataFiles(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "673.cpp")
	if err := os.WriteFile(sourcePath, []byte("int main() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	score := 0.0
	remoteTime := 1
	remoteMemory := 1536
	testcase := runner.TestCase{ID: 11464, Index: 2, Input: []byte("5\n1 2 3 4 5\n"), Expected: []byte("15\n")}
	result := runner.RunResult{
		TestcaseID: 11464,
		Index:      2,
		Stdout:     []byte("14\n"),
		Stderr:     []byte("debug\n"),
		ExitCode:   0,
		Duration:   3 * time.Millisecond,
		Comparison: runner.ComparisonResult{Match: false},
	}
	if err := writeLocalReplayCache(sourcePath, 924618, remoteEvaluationSnapshot{
		Status: "wrong", Score: &score, Time: &remoteTime, Memory: &remoteMemory,
	}, testcase, result, ""); err != nil {
		t.Fatal(err)
	}

	data, err := readLocalReplayCache(sourcePath, 924618, 11464)
	if err != nil {
		t.Fatal(err)
	}
	if data.Metadata.TestcaseID != 11464 || data.Metadata.RemoteStatus != "wrong" || data.Metadata.LocalStatus != "wrong_approximate" || data.Metadata.DurationMS != 3 {
		t.Fatalf("metadata = %#v", data.Metadata)
	}
	if string(data.Input) != string(testcase.Input) || string(data.Expected) != string(testcase.Expected) || string(data.Stdout) != "14\n" || string(data.Stderr) != "debug\n" {
		t.Fatalf("cache data = %#v", data)
	}

	// Rewriting the same testcase directory is an idempotent cache update.
	result.Stdout = []byte("15\n")
	result.Comparison.Match = true
	if err := writeLocalReplayCache(sourcePath, 924618, remoteEvaluationSnapshot{Status: "wrong"}, testcase, result, ""); err != nil {
		t.Fatal(err)
	}
	data, err = readLocalReplayCache(sourcePath, 924618, 11464)
	if err != nil {
		t.Fatal(err)
	}
	if data.Metadata.LocalStatus != "correct_approximate" || string(data.Stdout) != "15\n" {
		t.Fatalf("updated cache = %#v", data)
	}
}

func TestLocalReplayCacheRejectsCorruptMetadata(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "673.cpp")
	directory, err := localReplayCacheDirectory(sourcePath, 7, 11)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "meta.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLocalReplayCache(sourcePath, 7, 11); err == nil {
		t.Fatal("corrupt cache was accepted")
	}
}

func TestLocalReplayCachePathsStayInsideQuestionDirectory(t *testing.T) {
	questionDirectory := filepath.Join(t.TempDir(), "Question With Spaces")
	sourcePath := filepath.Join(questionDirectory, "673.cpp")
	cachePath, err := localReplayCacheDirectory(sourcePath, 9, 12)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(questionDirectory, ".yoel", "testcases", "9", "12")
	if cachePath != want {
		t.Fatalf("cache path = %q, want %q", cachePath, want)
	}
	if _, err := localReplayCacheDirectory(sourcePath, 0, 12); err == nil {
		t.Fatal("invalid submission ID was accepted")
	}
}

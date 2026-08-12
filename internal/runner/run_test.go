package runner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestRunFeedsStdinAndCapturesOutputsAndExitStatus(t *testing.T) {
	binary := compileRunnerFixture(t)

	success := Run(context.Background(), binary, TestCase{
		ID:       11,
		Index:    2,
		Input:    []byte("hello\n"),
		Expected: []byte("got:hello\n"),
	}, time.Second)
	if success.Err != nil || success.ExitCode != 0 || string(success.Stdout) != "got:hello\n" || string(success.Stderr) != "diagnostic\n" || !success.Comparison.Match {
		t.Fatalf("success result = %#v", success)
	}
	if success.TestcaseID != 11 || success.Index != 2 {
		t.Fatalf("identity = %d/%d", success.TestcaseID, success.Index)
	}

	failure := Run(context.Background(), binary, TestCase{ID: 12, Input: []byte("exit\n")}, time.Second)
	if failure.Err == nil || failure.ExitCode != 7 || failure.TimedOut {
		t.Fatalf("failure result = %#v", failure)
	}
}

func TestRunTimesOutHungProgram(t *testing.T) {
	binary := compileRunnerFixture(t)
	result := Run(context.Background(), binary, TestCase{ID: 13, Input: []byte("hang\n")}, 50*time.Millisecond)
	if !result.TimedOut || result.Err == nil {
		t.Fatalf("result = %#v, want timeout", result)
	}
}

func TestRunCapsCapturedOutput(t *testing.T) {
	binary := compileRunnerFixture(t)
	result := Run(context.Background(), binary, TestCase{ID: 14, Input: []byte("output\n")}, time.Second)
	if !result.Truncated || len(result.Stdout) != outputLimit {
		t.Fatalf("truncated = %t, stdout bytes = %d, want %d", result.Truncated, len(result.Stdout), outputLimit)
	}
}

func TestRunAllKeepsResultsAssociatedWithTestcases(t *testing.T) {
	binary := compileRunnerFixture(t)
	testcases := []TestCase{
		{ID: 31, Index: 0, Input: []byte("one\n"), Expected: []byte("got:one\n")},
		{ID: 32, Index: 1, Input: []byte("two\n"), Expected: []byte("got:two\n")},
		{ID: 33, Index: 2, Input: []byte("three\n"), Expected: []byte("got:three\n")},
	}
	var results []RunResult
	for result := range RunAll(context.Background(), binary, testcases, 2, time.Second) {
		results = append(results, result)
	}
	if len(results) != len(testcases) {
		t.Fatalf("results = %d, want %d", len(results), len(testcases))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Index < results[j].Index })
	for i, result := range results {
		if result.TestcaseID != testcases[i].ID || !result.Comparison.Match {
			t.Fatalf("result[%d] = %#v, testcase = %#v", i, result, testcases[i])
		}
	}
}

func compileRunnerFixture(t *testing.T) string {
	t.Helper()
	requireGPP(t)
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "runner.cpp")
	source := `
#include <iostream>
#include <string>
int main() {
    std::string input;
    std::getline(std::cin, input);
    if (input == "hang") { while (true) {} }
    if (input == "exit") { std::cerr << "diagnostic\n"; return 7; }
    if (input == "output") { for (int i = 0; i < 300000; ++i) std::cout << 'x'; return 0; }
    std::cout << "got:" << input << "\n";
    std::cerr << "diagnostic\n";
    return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileCPP(context.Background(), sourcePath, filepath.Join(directory, ".yoel", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	return compiled.BinaryPath
}

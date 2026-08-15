package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yoel/internal/graderapi"
	"yoel/internal/runner"
)

func TestMeasureText(t *testing.T) {
	stats := measureText([]byte("one\nไทย\n"))
	if stats.Bytes != len("one\nไทย\n") || stats.Lines != 2 || stats.MaxLineWidth != 3 {
		t.Fatalf("stats = %#v", stats)
	}
}

func runnerSubmission(submissionID, testcaseID int) graderapi.Submission {
	wrong := "wrong"
	return graderapi.Submission{ID: submissionID, Evaluations: []graderapi.Evaluation{{TestcaseID: testcaseID, Result: &wrong}}}
}

func TestRenderTestcaseInspectionKeepsHugeRowsAndColumnsBounded(t *testing.T) {
	veryWide := strings.Repeat("x", 100_000)
	manyLines := strings.Repeat("row\n", 20_000)
	result := runner.RunResult{Stdout: []byte(manyLines)}
	state := localReplayState{
		Status:            localReplayFinished,
		InputAvailable:    true,
		ExpectedAvailable: true,
		Testcase:          runner.TestCase{ID: 1, Input: []byte(veryWide), Expected: []byte("expected\n")},
		Result:            &result,
	}
	state.Inspection = buildTestcaseInspection(state)
	view := renderLocalReplayStateAtWidth(state, 80)
	if len(view) > 8_000 {
		t.Fatalf("rendered view has %d bytes; large data leaked into the terminal", len(view))
	}
	for _, want := range []string{"100000 chars", "20000 lines", "use e to inspect"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestTestcaseInspectionDefersPreviewsAndMismatchContext(t *testing.T) {
	result := runner.RunResult{Stdout: []byte("actual\n")}
	state := localReplayState{
		InputAvailable:    true,
		ExpectedAvailable: true,
		Testcase:          runner.TestCase{ID: 1, Input: []byte(strings.Repeat("x", 100_000)), Expected: []byte("expected\n")},
		Result:            &result,
	}
	metadata := buildTestcaseInspection(state)
	if metadata.DetailsPrepared || len(metadata.Input.Preview) != 0 || metadata.Mismatch != nil {
		t.Fatalf("background inspection prepared details: %#v", metadata)
	}
	details := buildTestcaseInspectionDetails(state)
	if !details.DetailsPrepared || len(details.Input.Preview) == 0 || details.Mismatch == nil {
		t.Fatalf("selected inspection did not prepare details: %#v", details)
	}
}

func TestRenderTestcaseInspectionShowsFirstMismatchContext(t *testing.T) {
	result := runner.RunResult{Stdout: []byte("one\nactual\nthree\n")}
	state := localReplayState{
		Status:            localReplayFinished,
		InputAvailable:    true,
		ExpectedAvailable: true,
		Testcase:          runner.TestCase{ID: 1, Input: []byte("input\n"), Expected: []byte("one\nexpected\nthree\n")},
		Result:            &result,
	}
	state.Inspection = buildTestcaseInspection(state)
	view := renderLocalReplayStateAtWidth(state, 80)
	for _, want := range []string{"First mismatch at line 2", "expected: expected", "got:      actual"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestWriteInspectionFileIncludesRawContentWithoutTerminalRendering(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "673.cpp")
	result := runner.RunResult{Stdout: []byte("actual\n"), Stderr: []byte("warning\n")}
	state := localReplayState{
		InputAvailable: true,
		Testcase:       runner.TestCase{ID: 11, Input: []byte("input\n")},
		Result:         &result,
	}
	path, err := writeInspectionFile(sourcePath, 7, state)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Input:\ninput", "Expected:\n[unavailable", "Actual:\nactual", "Stderr:\nwarning"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("inspection file missing %q: %q", want, data)
		}
	}
}

func TestEditorCommandUsesVisualThenEditorWithSimpleFlags(t *testing.T) {
	command, err := editorCommand("/tmp/test case.txt", func(name string) string {
		if name == "VISUAL" {
			return "nano --linenumbers"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Path == "" || strings.Join(command.Args[1:], "|") != "--linenumbers|/tmp/test case.txt" {
		t.Fatalf("command = %#v", command)
	}
	if _, err := editorCommand("file", func(string) string { return "" }); err == nil {
		t.Fatal("missing editor was accepted")
	}
}

func TestPrepareInspectionFetchesExpectedLazily(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "673.cpp")
	model := newSubmissionResultModelForReplay(context.Background(), sourcePath, runnerSubmission(9, 11), nil, func(_ context.Context, testcaseID int) ([]byte, error) {
		if testcaseID != 11 {
			t.Errorf("testcase ID = %d", testcaseID)
		}
		return []byte("expected\n"), nil
	})
	model.states[0] = localReplayState{Status: localReplayInputReady, InputAvailable: true, Testcase: runner.TestCase{ID: 11, Index: 0, Input: []byte("input\n")}}
	message := model.prepareInspection()()
	prepared, ok := message.(inspectionPreparedMessage)
	if !ok || prepared.err != nil || !prepared.state.ExpectedAvailable || string(prepared.state.Testcase.Expected) != "expected\n" {
		t.Fatalf("prepared = %#v", message)
	}
}

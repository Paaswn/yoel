package cli

import (
	"strings"
	"testing"

	"yoel/internal/graderapi"
	"yoel/internal/runner"
)

func TestSubmissionResultModelKeepsSelectionWhileAsyncResultsArrive(t *testing.T) {
	wrong := "wrong"
	submission := graderapi.Submission{Evaluations: []graderapi.Evaluation{
		{TestcaseID: 11, Result: &wrong},
		{TestcaseID: 22, Result: &wrong},
	}}
	model := newSubmissionResultModel(submission, nil)
	model.selected = 1
	_, _ = model.Update(localReplayMessage{ok: true, event: localReplayEvent{
		Index: 0,
		State: localReplayState{Status: localReplayRunning, Testcase: runner.TestCase{ID: 11, Index: 0}},
	}})
	if model.selected != 1 {
		t.Fatalf("selection = %d, want 1", model.selected)
	}
	if detail := model.renderSelectedDetail(submission); !strings.Contains(detail, "Testcase 22") || strings.Contains(detail, "Local replay") {
		t.Fatalf("selected detail = %q", detail)
	}

	result := runner.RunResult{TestcaseID: 22, Index: 1, ExitCode: 0, Stdout: []byte("14\n")}
	_, _ = model.Update(localReplayMessage{ok: true, event: localReplayEvent{
		Index: 1,
		State: localReplayState{
			Status:   localReplayFinished,
			Testcase: runner.TestCase{ID: 22, Index: 1, Input: []byte("5\n"), Expected: []byte("15\n")},
			Result:   &result,
		},
	}})
	detail := model.renderSelectedDetail(submission)
	for _, want := range []string{"Testcase 22", "Local replay · wrong approximate", "Input", "5", "Expected", "15", "Got", "14"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail = %q, want %q", detail, want)
		}
	}
}

func TestRenderLocalReplayCompileFailureIncludesDiagnostic(t *testing.T) {
	got := renderLocalReplayState(localReplayState{
		Status:         localReplayCompileFailed,
		CompilerOutput: "main.cpp: error: missing semicolon",
	})
	if !strings.Contains(got, "Local replay · local compilation failed") || !strings.Contains(got, "missing semicolon") {
		t.Fatalf("output = %q", got)
	}
}

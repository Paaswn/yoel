package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"yoel/internal/graderapi"
	"yoel/internal/runner"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func TestSubmissionResultModelUsesEForTestcaseInspection(t *testing.T) {
	wrong := "wrong"
	submission := graderapi.Submission{ID: 7, Evaluations: []graderapi.Evaluation{{TestcaseID: 11, Result: &wrong}}}
	model := newSubmissionResultModelForReplay(context.Background(), filepath.Join(t.TempDir(), "673.cpp"), submission, nil, func(context.Context, int) ([]byte, error) {
		return []byte("expected\n"), nil
	})
	model.states[0] = localReplayState{Status: localReplayInputReady, InputAvailable: true, Testcase: runner.TestCase{ID: 11, Input: []byte("input\n")}}
	model.form.NextGroup()
	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	if command == nil {
		t.Fatal("e did not start inspection preparation")
	}
	message := command()
	prepared, ok := message.(inspectionPreparedMessage)
	if !ok || prepared.err != nil || !prepared.state.ExpectedAvailable {
		t.Fatalf("inspection message = %#v", message)
	}
	if model.form.State != huh.StateNormal {
		t.Fatalf("form state = %v, want normal", model.form.State)
	}
}

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
	if detail := model.renderSelectedDetail(submission); detail != "" {
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
	for _, want := range []string{"Local replay · wrong approximate", "Input", "5", "Expected", "15", "Got", "14"} {
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

func TestSubmissionResultModelIgnoresUnexpectedKeysInDetailView(t *testing.T) {
	wrong := "wrong"
	model := newSubmissionResultModel(graderapi.Submission{
		Evaluations: []graderapi.Evaluation{{TestcaseID: 11, Result: &wrong}},
	}, nil)
	model.form.NextGroup()

	if _, ok := model.form.GetFocusedField().(*huh.Note); !ok {
		t.Fatal("detail note is not focused")
	}
	_, command := model.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	if command != nil {
		t.Fatal("unexpected key produced a command")
	}
	if model.form.State != huh.StateNormal {
		t.Fatalf("form state = %v, want normal", model.form.State)
	}
}

func TestSubmissionResultModelKeepsNavigationAndCancellationKeys(t *testing.T) {
	wrong := "wrong"
	model := newSubmissionResultModel(graderapi.Submission{
		Evaluations: []graderapi.Evaluation{{TestcaseID: 11, Result: &wrong}},
	}, nil)
	model.form.NextGroup()

	for _, key := range []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}),
		tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c", Mod: tea.ModCtrl}),
	} {
		if ignoreReplayDetailKey(model.form, key) {
			t.Fatalf("key %q was ignored", key.String())
		}
	}
}

func TestNormalizeInteractiveSubmissionKeepsPatternAndResultsInTestcaseOrder(t *testing.T) {
	correct := "correct"
	wrong := "wrong"
	originalComment := "server order"
	submission := graderapi.Submission{
		GraderComment: &originalComment,
		Evaluations: []graderapi.Evaluation{
			{TestcaseID: 40, Result: &correct},
			{TestcaseID: 10, Result: &correct},
			{TestcaseID: 30, Result: &wrong},
			{TestcaseID: 20, Result: &correct},
		},
	}

	normalized := normalizeInteractiveSubmission(submission)
	if normalized.GraderComment == nil || *normalized.GraderComment != "PP-P" {
		t.Fatalf("result pattern = %v, want PP-P", normalized.GraderComment)
	}
	for index, testcaseID := range []int{10, 20, 30, 40} {
		if normalized.Evaluations[index].TestcaseID != testcaseID {
			t.Fatalf("evaluation %d testcase ID = %d, want %d", index, normalized.Evaluations[index].TestcaseID, testcaseID)
		}
	}
	if submission.Evaluations[0].TestcaseID != 40 || *submission.GraderComment != originalComment {
		t.Fatalf("original submission was mutated: %#v", submission)
	}
	if summary := renderSubmissionSummary(normalized); !strings.Contains(summary, "[PP-P]") {
		t.Fatalf("summary = %q, want [PP-P]", summary)
	}
}

func TestEvaluationResultPatternTreatsMissingAndNonCorrectResultsAsFailures(t *testing.T) {
	correct := " Correct "
	partial := "partial"
	got := evaluationResultPattern([]graderapi.Evaluation{
		{Result: &correct},
		{Result: nil},
		{Result: &partial},
	})
	if got != "P--" {
		t.Fatalf("pattern = %q, want P--", got)
	}
}

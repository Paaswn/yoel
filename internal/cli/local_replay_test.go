package cli

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"yoel/internal/graderapi"
	"yoel/internal/runner"
)

func TestLocallyRunnableEvaluation(t *testing.T) {
	for _, status := range []string{"wrong", "wrong answer", "runtime_error", "time-limit-exceeded", "memory_limit", "partial"} {
		status := status
		if !locallyRunnableEvaluation(graderapi.Evaluation{TestcaseID: 1, Result: &status}) {
			t.Errorf("status %q is not runnable", status)
		}
	}
	for _, status := range []string{"correct", "compilation_error", "grader_error", ""} {
		status := status
		if locallyRunnableEvaluation(graderapi.Evaluation{TestcaseID: 1, Result: &status}) {
			t.Errorf("status %q is runnable", status)
		}
	}
}

func TestCoordinateLocalReplaySkipsAllCorrectSubmission(t *testing.T) {
	correct := "correct"
	var calls atomic.Int32
	deps := localReplayDependencies{
		downloadInput: func(context.Context, int) ([]byte, error) { calls.Add(1); return nil, nil },
	}
	coordinateLocalReplay(context.Background(), "673.cpp", graderapi.Submission{
		Evaluations: []graderapi.Evaluation{{TestcaseID: 11, Result: &correct}},
	}, deps, func(localReplayEvent) { calls.Add(1) })
	if calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0", calls.Load())
	}
}

func TestCoordinateLocalReplaySkipsCompilationWithoutTestcaseData(t *testing.T) {
	wrong := "wrong"
	var compileCalls atomic.Int32
	deps := replayTestDependencies()
	deps.downloadSolution = func(context.Context, int) ([]byte, error) {
		return nil, errors.New("not exposed")
	}
	deps.compile = func(context.Context, string, string) (runner.CompileResult, error) {
		compileCalls.Add(1)
		return runner.CompileResult{}, nil
	}
	var last localReplayEvent
	coordinateLocalReplay(context.Background(), "673.cpp", graderapi.Submission{
		Evaluations: []graderapi.Evaluation{{TestcaseID: 11, Result: &wrong}},
	}, deps, func(event localReplayEvent) { last = event })
	if compileCalls.Load() != 0 || last.State.Status != localReplayUnavailable {
		t.Fatalf("compile calls = %d, last event = %#v", compileCalls.Load(), last)
	}
}

func TestCoordinateLocalReplayCompilationFailurePreservesRemoteResultAndDoesNotRun(t *testing.T) {
	wrong := "wrong"
	deps := replayTestDependencies()
	deps.compile = func(context.Context, string, string) (runner.CompileResult, error) {
		return runner.CompileResult{}, &runner.CompileError{Output: "fake compiler diagnostic"}
	}
	var runCalls atomic.Int32
	deps.runAll = func(context.Context, string, []runner.TestCase, int, time.Duration) <-chan runner.RunResult {
		runCalls.Add(1)
		return closedRunResults()
	}
	var cachedStatus string
	deps.writeCache = func(_ string, _ int, _ remoteEvaluationSnapshot, _ runner.TestCase, _ runner.RunResult, localStatus string) error {
		cachedStatus = localStatus
		return nil
	}
	var last localReplayEvent
	submission := graderapi.Submission{Evaluations: []graderapi.Evaluation{{TestcaseID: 11, Result: &wrong}}}
	coordinateLocalReplay(context.Background(), "673.cpp", submission, deps, func(event localReplayEvent) { last = event })
	if runCalls.Load() != 0 || cachedStatus != "compilation_failed" || last.State.Status != localReplayCompileFailed || last.State.CompilerOutput != "fake compiler diagnostic" {
		t.Fatalf("run calls = %d, last event = %#v", runCalls.Load(), last)
	}
	if *submission.Evaluations[0].Result != "wrong" {
		t.Fatal("local replay changed the remote result")
	}
}

func TestCoordinateLocalReplayPublishesResultForOriginalIndex(t *testing.T) {
	correct, wrong := "correct", "wrong"
	deps := replayTestDependencies()
	var compileCalls atomic.Int32
	deps.compile = func(context.Context, string, string) (runner.CompileResult, error) {
		compileCalls.Add(1)
		return runner.CompileResult{BinaryPath: "fake-binary"}, nil
	}
	deps.runAll = func(_ context.Context, binary string, testcases []runner.TestCase, limit int, timeout time.Duration) <-chan runner.RunResult {
		if binary != "fake-binary" || len(testcases) != 1 || testcases[0].Index != 1 || limit < 1 || timeout != runner.DefaultTestTimeout {
			t.Errorf("run args = %q, %#v, %d, %s", binary, testcases, limit, timeout)
		}
		results := make(chan runner.RunResult, 1)
		results <- runner.RunResult{TestcaseID: 22, Index: 1, ExitCode: 0, Stdout: []byte("14\n")}
		close(results)
		return results
	}
	var cacheCalls atomic.Int32
	deps.writeCache = func(_ string, submissionID int, _ remoteEvaluationSnapshot, testcase runner.TestCase, _ runner.RunResult, localStatus string) error {
		cacheCalls.Add(1)
		if submissionID != 99 || testcase.ID != 22 || testcase.Index != 1 {
			t.Errorf("cache identity = %d, %#v", submissionID, testcase)
		}
		return nil
	}
	var events []localReplayEvent
	coordinateLocalReplay(context.Background(), "673.cpp", graderapi.Submission{
		ID: 99,
		Evaluations: []graderapi.Evaluation{
			{TestcaseID: 11, Result: &correct},
			{TestcaseID: 22, Result: &wrong},
		},
	}, deps, func(event localReplayEvent) { events = append(events, event) })
	if compileCalls.Load() != 1 || cacheCalls.Load() != 1 {
		t.Fatalf("compile calls = %d, cache calls = %d", compileCalls.Load(), cacheCalls.Load())
	}
	last := events[len(events)-1]
	if last.Index != 1 || last.State.Result == nil || last.State.Result.TestcaseID != 22 {
		t.Fatalf("last event = %#v", last)
	}
}

func replayTestDependencies() localReplayDependencies {
	return localReplayDependencies{
		downloadInput: func(context.Context, int) ([]byte, error) { return []byte("input\n"), nil },
		downloadSolution: func(context.Context, int) ([]byte, error) {
			return []byte("expected\n"), nil
		},
		compile: func(context.Context, string, string) (runner.CompileResult, error) {
			return runner.CompileResult{BinaryPath: "fake-binary"}, nil
		},
		runAll: func(context.Context, string, []runner.TestCase, int, time.Duration) <-chan runner.RunResult {
			return closedRunResults()
		},
		writeCache: func(string, int, remoteEvaluationSnapshot, runner.TestCase, runner.RunResult, string) error {
			return nil
		},
	}
}

func closedRunResults() <-chan runner.RunResult {
	results := make(chan runner.RunResult)
	close(results)
	return results
}

package cli

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"yoel/internal/graderapi"
	"yoel/internal/runner"
)

type localReplayStatus string

const (
	localReplayFetching      localReplayStatus = "fetching testcase data…"
	localReplayCompiling     localReplayStatus = "compiling…"
	localReplayRunning       localReplayStatus = "running…"
	localReplayUnavailable   localReplayStatus = "unavailable"
	localReplayCompileFailed localReplayStatus = "local compilation failed"
	localReplayFinished      localReplayStatus = "finished"
)

type remoteEvaluationSnapshot struct {
	Status string
	Score  *float64
	Time   *int
	Memory *int
}

type localReplayState struct {
	Status         localReplayStatus
	Testcase       runner.TestCase
	Result         *runner.RunResult
	CompilerOutput string
}

type localReplayEvent struct {
	Index int
	State localReplayState
}

type localReplayDependencies struct {
	downloadInput    func(context.Context, int) ([]byte, error)
	downloadSolution func(context.Context, int) ([]byte, error)
	compile          func(context.Context, string, string) (runner.CompileResult, error)
	runAll           func(context.Context, string, []runner.TestCase, int, time.Duration) <-chan runner.RunResult
	writeCache       func(string, int, remoteEvaluationSnapshot, runner.TestCase, runner.RunResult, string) error
}

func defaultLocalReplayDependencies(client *graderapi.Client) localReplayDependencies {
	return localReplayDependencies{
		downloadInput:    client.DownloadTestcaseInput,
		downloadSolution: client.DownloadTestcaseSolution,
		compile:          runner.CompileCPP,
		runAll: func(ctx context.Context, binary string, testcases []runner.TestCase, limit int, timeout time.Duration) <-chan runner.RunResult {
			return runner.RunAll(ctx, binary, testcases, limit, timeout)
		},
		writeCache: writeLocalReplayCache,
	}
}

func coordinateLocalReplay(ctx context.Context, sourcePath string, submission graderapi.Submission, deps localReplayDependencies, emit func(localReplayEvent)) {
	if ctx == nil || emit == nil || strings.ToLower(filepath.Ext(sourcePath)) != ".cpp" {
		return
	}

	testcases := make([]runner.TestCase, 0)
	evaluations := make(map[int]remoteEvaluationSnapshot)
	for index, evaluation := range submission.Evaluations {
		if !locallyRunnableEvaluation(evaluation) {
			continue
		}
		state := localReplayState{Status: localReplayFetching, Testcase: runner.TestCase{ID: evaluation.TestcaseID, Index: index}}
		emit(localReplayEvent{Index: index, State: state})
		input, err := deps.downloadInput(ctx, evaluation.TestcaseID)
		if err != nil {
			state.Status = localReplayUnavailable
			emit(localReplayEvent{Index: index, State: state})
			continue
		}
		expected, err := deps.downloadSolution(ctx, evaluation.TestcaseID)
		if err != nil {
			state.Status = localReplayUnavailable
			emit(localReplayEvent{Index: index, State: state})
			continue
		}
		state.Testcase.Input = input
		state.Testcase.Expected = expected
		testcases = append(testcases, state.Testcase)
		evaluations[index] = snapshotEvaluation(evaluation)
	}
	if len(testcases) == 0 || ctx.Err() != nil {
		return
	}

	for _, testcase := range testcases {
		emit(localReplayEvent{Index: testcase.Index, State: localReplayState{Status: localReplayCompiling, Testcase: testcase}})
	}
	cacheDir, err := binaryCacheDirectory(sourcePath)
	if err != nil {
		return
	}
	compiled, err := deps.compile(ctx, sourcePath, cacheDir)
	if err != nil {
		var compileErr *runner.CompileError
		compilerOutput := ""
		if errors.As(err, &compileErr) {
			compilerOutput = compileErr.Output
		}
		for _, testcase := range testcases {
			emit(localReplayEvent{Index: testcase.Index, State: localReplayState{
				Status:         localReplayCompileFailed,
				Testcase:       testcase,
				CompilerOutput: compilerOutput,
			}})
			_ = deps.writeCache(sourcePath, submission.ID, evaluations[testcase.Index], testcase, runner.RunResult{
				TestcaseID: testcase.ID,
				Index:      testcase.Index,
				ExitCode:   -1,
			}, "compilation_failed")
		}
		return
	}

	for _, testcase := range testcases {
		emit(localReplayEvent{Index: testcase.Index, State: localReplayState{Status: localReplayRunning, Testcase: testcase}})
	}
	limit := min(runtime.NumCPU(), 4)
	for result := range deps.runAll(ctx, compiled.BinaryPath, testcases, limit, runner.DefaultTestTimeout) {
		resultCopy := result
		state := localReplayState{Status: localReplayFinished, Testcase: testcasesByIndex(testcases, result.Index), Result: &resultCopy}
		emit(localReplayEvent{Index: result.Index, State: state})
		if state.Testcase.ID > 0 {
			_ = deps.writeCache(sourcePath, submission.ID, evaluations[result.Index], state.Testcase, result, "")
		}
	}
}

func locallyRunnableEvaluation(evaluation graderapi.Evaluation) bool {
	if evaluation.TestcaseID <= 0 || evaluation.Result == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(*evaluation.Result))
	status = strings.NewReplacer(" ", "_", "-", "_").Replace(status)
	switch status {
	case "wrong", "wrong_answer", "partial", "runtime_error", "time_limit", "time_limit_exceeded", "memory_limit", "memory_limit_exceeded":
		return true
	default:
		return false
	}
}

func snapshotEvaluation(evaluation graderapi.Evaluation) remoteEvaluationSnapshot {
	status := ""
	if evaluation.Result != nil {
		status = *evaluation.Result
	}
	return remoteEvaluationSnapshot{Status: status, Score: evaluation.Score, Time: evaluation.Time, Memory: evaluation.Memory}
}

func testcasesByIndex(testcases []runner.TestCase, index int) runner.TestCase {
	for _, testcase := range testcases {
		if testcase.Index == index {
			return testcase
		}
	}
	return runner.TestCase{}
}

func localRunStatus(result runner.RunResult) string {
	switch {
	case result.TimedOut:
		return "timed_out"
	case result.Truncated:
		return "output_truncated"
	case result.Err != nil || result.ExitCode != 0:
		return "runtime_error"
	case result.Comparison.Match:
		return "correct_approximate"
	default:
		return "wrong_approximate"
	}
}

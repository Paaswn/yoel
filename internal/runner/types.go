package runner

import "time"

// TestCase is one remotely supplied testcase prepared for local execution.
type TestCase struct {
	ID       int
	Index    int
	Input    []byte
	Expected []byte
}

// ComparisonResult records Yoel's approximate local output comparison.
type ComparisonResult struct {
	Match bool
}

// RunResult records one local process execution. Remote grader results remain
// separate and authoritative.
type RunResult struct {
	TestcaseID int
	Index      int
	Stdout     []byte
	Stderr     []byte
	ExitCode   int
	Duration   time.Duration
	TimedOut   bool
	Truncated  bool
	Err        error
	Comparison ComparisonResult
}

// CompileResult identifies the source-specific binary selected for this run.
type CompileResult struct {
	BinaryPath string
	Cached     bool
}

// CompileError represents a failed compiler invocation. Output is kept out of
// Error so source excerpts cannot accidentally enter ordinary error logs.
type CompileError struct {
	Output string
}

func (e *CompileError) Error() string {
	return "local compilation failed"
}

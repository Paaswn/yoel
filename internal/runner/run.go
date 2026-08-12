package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

const DefaultTestTimeout = 5 * time.Second

// Run executes one testcase with bounded output and a per-process timeout.
func Run(ctx context.Context, binaryPath string, testcase TestCase, timeout time.Duration) RunResult {
	result := RunResult{TestcaseID: testcase.ID, Index: testcase.Index, ExitCode: -1}
	if ctx == nil || binaryPath == "" || timeout <= 0 {
		result.Err = errors.New("invalid local execution input")
		return result
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, binaryPath)
	command.WaitDelay = 500 * time.Millisecond
	command.Stdin = bytes.NewReader(testcase.Input)
	stdout := &limitedBuffer{limit: outputLimit}
	stderr := &limitedBuffer{limit: outputLimit}
	command.Stdout = stdout
	command.Stderr = stderr

	started := time.Now()
	err := command.Run()
	result.Duration = time.Since(started)
	result.Stdout = stdout.Bytes()
	result.Stderr = stderr.Bytes()
	result.Truncated = stdout.Truncated() || stderr.Truncated()
	if command.ProcessState != nil {
		result.ExitCode = command.ProcessState.ExitCode()
	}
	if errors.Is(runContext.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		result.TimedOut = true
		result.Err = context.DeadlineExceeded
		return result
	}
	if ctx.Err() != nil {
		result.Err = ctx.Err()
		return result
	}
	if err != nil {
		result.Err = err
		return result
	}
	result.Comparison = CompareOutput(result.Stdout, testcase.Expected)
	return result
}

// RunAll executes testcases with bounded concurrency. Results may arrive in
// completion order but retain their original testcase ID and display index.
func RunAll(ctx context.Context, binaryPath string, testcases []TestCase, limit int, timeout time.Duration) <-chan RunResult {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make(chan RunResult, len(testcases))
	if limit < 1 {
		limit = 1
	}
	if limit > len(testcases) && len(testcases) > 0 {
		limit = len(testcases)
	}
	jobs := make(chan TestCase)
	var workers sync.WaitGroup
	workers.Add(limit)
	for range limit {
		go func() {
			defer workers.Done()
			for testcase := range jobs {
				results <- Run(ctx, binaryPath, testcase, timeout)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, testcase := range testcases {
			select {
			case jobs <- testcase:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	return results
}

type limitedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		b.data = append(b.data, value[:remaining]...)
	}
	if remaining < len(value) {
		b.truncated = true
	}
	return len(value), nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

func (b *limitedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

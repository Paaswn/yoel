package runner

import "strings"

// OutputMismatch describes the first meaningful difference between two outputs.
// Line numbers start at one after Yoel's normal comparison normalization.
type OutputMismatch struct {
	Line          int
	Expected      string
	Actual        string
	ExpectedEnded bool
	ActualEnded   bool
}

// CompareOutput performs Yoel's approximate comparison: line endings are
// normalized and trailing horizontal whitespace and blank lines are ignored.
func CompareOutput(actual, expected []byte) ComparisonResult {
	return ComparisonResult{Match: FirstOutputMismatch(actual, expected) == nil}
}

// FirstOutputMismatch returns nil when the outputs compare equal under Yoel's
// approximate comparison rules. Otherwise it identifies the first different
// line without exposing an unbounded diff structure to callers.
func FirstOutputMismatch(actual, expected []byte) *OutputMismatch {
	actualLines := normalizedOutputLines(string(actual))
	expectedLines := normalizedOutputLines(string(expected))
	count := len(actualLines)
	if len(expectedLines) < count {
		count = len(expectedLines)
	}
	for index := 0; index < count; index++ {
		if actualLines[index] != expectedLines[index] {
			return &OutputMismatch{Line: index + 1, Expected: expectedLines[index], Actual: actualLines[index]}
		}
	}
	if len(actualLines) == len(expectedLines) {
		return nil
	}
	mismatch := &OutputMismatch{Line: count + 1}
	if count == len(expectedLines) {
		mismatch.ExpectedEnded = true
		mismatch.Actual = actualLines[count]
	} else {
		mismatch.ActualEnded = true
		mismatch.Expected = expectedLines[count]
	}
	return mismatch
}

func normalizedOutputLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

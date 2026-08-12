package runner

import "strings"

// CompareOutput performs Yoel's approximate comparison: line endings are
// normalized and trailing horizontal whitespace and blank lines are ignored.
func CompareOutput(actual, expected []byte) ComparisonResult {
	return ComparisonResult{Match: normalizeOutput(string(actual)) == normalizeOutput(string(expected))}
}

func normalizeOutput(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

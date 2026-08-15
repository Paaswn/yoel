package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"yoel/internal/runner"
)

const (
	inlineInspectionBytes  = 32 << 10
	inlineInspectionLines  = 100
	previewInspectionLines = 3
	inspectionContextLines = 3
	defaultInspectionWidth = 80
)

// TextStats describes content without forcing it into the terminal.
type TextStats struct {
	Bytes        int
	Lines        int
	MaxLineWidth int
}

type inspectionText struct {
	Available bool
	Error     string
	Stats     TextStats
	Inline    string
	Preview   []string
	Large     bool
}

type inspectionLine struct {
	Number   int
	Expected string
	Actual   string
}

type testcaseInspection struct {
	Prepared        bool
	DetailsPrepared bool
	Input           inspectionText
	Expected        inspectionText
	Actual          inspectionText
	Stderr          inspectionText
	Mismatch        *runner.OutputMismatch
	Context         []inspectionLine
}

func buildTestcaseInspection(state localReplayState) testcaseInspection {
	inputAvailable := state.InputAvailable || len(state.Testcase.Input) > 0
	expectedAvailable := state.ExpectedAvailable || len(state.Testcase.Expected) > 0
	inspection := testcaseInspection{
		Prepared: true,
		Input:    inspectionTextFor(state.Testcase.Input, inputAvailable, state.InputError),
		Expected: inspectionTextFor(state.Testcase.Expected, expectedAvailable, state.ExpectedError),
	}
	if state.Result != nil {
		inspection.Actual = inspectionTextFor(state.Result.Stdout, true, "")
		inspection.Stderr = inspectionTextFor(state.Result.Stderr, true, "")
	}
	return inspection
}

// buildTestcaseInspectionDetails performs the limited preview and mismatch work
// only after a user selects a testcase or explicitly opens its raw inspector.
func buildTestcaseInspectionDetails(state localReplayState) testcaseInspection {
	inspection := buildTestcaseInspection(state)
	inspection.DetailsPrepared = true
	inspection.Input = inspectionTextDetails(state.Testcase.Input, inspection.Input)
	inspection.Expected = inspectionTextDetails(state.Testcase.Expected, inspection.Expected)
	if state.Result != nil {
		inspection.Actual = inspectionTextDetails(state.Result.Stdout, inspection.Actual)
		inspection.Stderr = inspectionTextDetails(state.Result.Stderr, inspection.Stderr)
		if inspection.Expected.Available {
			inspection.Mismatch = runner.FirstOutputMismatch(state.Result.Stdout, state.Testcase.Expected)
			inspection.Context = mismatchContext(state.Result.Stdout, state.Testcase.Expected, inspection.Mismatch)
		}
	}
	return inspection
}

func inspectionTextFor(data []byte, available bool, message string) inspectionText {
	text := inspectionText{Available: available, Error: message}
	if !available {
		return text
	}
	text.Stats = measureText(data)
	text.Large = len(data) > inlineInspectionBytes || text.Stats.Lines > inlineInspectionLines || text.Stats.MaxLineWidth > defaultInspectionWidth
	return text
}

func inspectionTextDetails(data []byte, text inspectionText) inspectionText {
	if !text.Available {
		return text
	}
	if !text.Large {
		text.Inline = string(data)
		return text
	}
	text.Preview = firstInspectionLines(data, previewInspectionLines)
	return text
}

func firstInspectionLines(data []byte, limit int) []string {
	if limit <= 0 || len(data) == 0 {
		return nil
	}
	lines := make([]string, 0, limit)
	for len(data) > 0 && len(lines) < limit {
		lineEnd := bytes.IndexByte(data, '\n')
		if lineEnd < 0 {
			lines = append(lines, string(data))
			break
		}
		lines = append(lines, string(data[:lineEnd]))
		data = data[lineEnd+1:]
	}
	return lines
}

func measureText(data []byte) TextStats {
	stats := TextStats{Bytes: len(data)}
	if len(data) == 0 {
		return stats
	}
	width := 0
	endsWithNewline := false
	for len(data) > 0 {
		runeValue, size := utf8.DecodeRune(data)
		if runeValue == '\n' {
			if width > stats.MaxLineWidth {
				stats.MaxLineWidth = width
			}
			stats.Lines++
			width = 0
			endsWithNewline = true
		} else {
			width++
			endsWithNewline = false
		}
		data = data[size:]
	}
	if !endsWithNewline {
		stats.Lines++
	}
	if width > stats.MaxLineWidth {
		stats.MaxLineWidth = width
	}
	return stats
}

func mismatchContext(actual, expected []byte, mismatch *runner.OutputMismatch) []inspectionLine {
	if mismatch == nil {
		return nil
	}
	actualLines := normalizedInspectionLines(string(actual))
	expectedLines := normalizedInspectionLines(string(expected))
	start := max(0, mismatch.Line-1-inspectionContextLines)
	end := min(max(len(actualLines), len(expectedLines)), mismatch.Line+inspectionContextLines)
	context := make([]inspectionLine, 0, end-start)
	for index := start; index < end; index++ {
		line := inspectionLine{Number: index + 1}
		if index < len(expectedLines) {
			line.Expected = expectedLines[index]
		}
		if index < len(actualLines) {
			line.Actual = actualLines[index]
		}
		context = append(context, line)
	}
	return context
}

func normalizedInspectionLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func renderTestcaseInspection(state localReplayState, width int) string {
	if width <= 0 {
		width = defaultInspectionWidth
	}
	inspection := state.Inspection
	if state.Status == localReplayCompileFailed {
		sections := []string{"Local replay · " + readableReplayStatus(state)}
		if strings.TrimSpace(state.CompilerOutput) != "" {
			compilerOutput := inspectionTextFor([]byte(state.CompilerOutput), true, "")
			sections = append(sections, renderInspectionText("Compiler output", inspectionTextDetails([]byte(state.CompilerOutput), compilerOutput), width))
		}
		return strings.Join(sections, "\n\n")
	}
	if !inspection.Input.Available && !inspection.Expected.Available && state.Result == nil {
		if inspection.Input.Error != "" {
			return "Local replay · " + string(state.Status) + "\n\nInput unavailable: " + inspection.Input.Error
		}
		return "Local replay · " + string(state.Status)
	}
	sections := []string{
		"Local replay · " + readableReplayStatus(state),
		renderInspectionText("Input", inspection.Input, width),
	}
	if inspection.Expected.Available || inspection.Expected.Error != "" {
		sections = append(sections, renderInspectionText("Expected", inspection.Expected, width))
	}
	if state.Result != nil {
		sections = append(sections, renderInspectionText("Got", inspection.Actual, width))
		if inspection.Stderr.Stats.Bytes > 0 {
			sections = append(sections, renderInspectionText("Stderr", inspection.Stderr, width))
		}
		if inspection.Mismatch != nil {
			sections = append(sections, renderMismatch(inspection, width))
		}
	}
	if inspection.Input.Available {
		sections = append(sections, "Press e to inspect raw testcase data in your editor.")
	}
	return strings.Join(sections, "\n\n")
}

func readableReplayStatus(state localReplayState) string {
	if state.Status == localReplayFinished && state.Result != nil {
		return strings.ReplaceAll(localRunStatus(*state.Result), "_", " ")
	}
	return string(state.Status)
}

func renderInspectionText(title string, text inspectionText, width int) string {
	if !text.Available {
		if text.Error == "" {
			return title + " unavailable"
		}
		return title + " unavailable: " + text.Error
	}
	header := fmt.Sprintf("%s · %s", title, formatTextStats(text.Stats))
	if !text.Large {
		if text.Inline == "" {
			return header + "\n(empty)"
		}
		return header + "\n" + truncateRenderedLines(text.Inline, width)
	}
	preview := make([]string, 0, len(text.Preview)+1)
	for _, line := range text.Preview {
		preview = append(preview, truncateRenderedLine(line, width))
	}
	preview = append(preview, "… use e to inspect the complete data")
	return header + "\n" + strings.Join(preview, "\n")
}

func formatTextStats(stats TextStats) string {
	lineLabel := "lines"
	if stats.Lines == 1 {
		lineLabel = "line"
	}
	return fmt.Sprintf("%d %s · %s · max width %d chars", stats.Lines, lineLabel, formatBytes(stats.Bytes), stats.MaxLineWidth)
}

func formatBytes(value int) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.1f KiB", float64(value)/1024)
}

func truncateRenderedLines(value string, width int) string {
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = truncateRenderedLine(lines[index], width)
	}
	return strings.Join(lines, "\n")
}

func truncateRenderedLine(value string, width int) string {
	if width < 16 {
		width = 16
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	suffix := fmt.Sprintf("… [%d chars]", len(runes))
	limit := max(1, width-len([]rune(suffix)))
	return string(runes[:limit]) + suffix
}

func renderMismatch(inspection testcaseInspection, width int) string {
	if inspection.Mismatch == nil {
		return ""
	}
	message := fmt.Sprintf("First mismatch at line %d", inspection.Mismatch.Line)
	if inspection.Mismatch.ExpectedEnded {
		message += " (expected output ended)"
	}
	if inspection.Mismatch.ActualEnded {
		message += " (program output ended)"
	}
	lines := []string{message}
	for _, line := range inspection.Context {
		marker := " "
		if line.Number == inspection.Mismatch.Line {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %d expected: %s", marker, line.Number, truncateRenderedLine(line.Expected, width-18)))
		lines = append(lines, fmt.Sprintf("  %d got:      %s", line.Number, truncateRenderedLine(line.Actual, width-18)))
	}
	return strings.Join(lines, "\n")
}

func inspectionFilePath(sourcePath string, submissionID, testcaseID int) (string, error) {
	directory, err := localReplayCacheDirectory(sourcePath, submissionID, testcaseID)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "inspection.txt"), nil
}

func writeInspectionFile(sourcePath string, submissionID int, state localReplayState) (string, error) {
	path, err := inspectionFilePath(sourcePath, submissionID, state.Testcase.ID)
	if err != nil {
		return "", err
	}
	content := strings.Join([]string{
		"Input:\n" + inspectionRawSection(state.Testcase.Input, state.InputAvailable, state.InputError),
		"Expected:\n" + inspectionRawSection(state.Testcase.Expected, state.ExpectedAvailable, state.ExpectedError),
		"Actual:\n" + inspectionActualSection(state),
		"Stderr:\n" + inspectionStderrSection(state),
	}, "\n\n") + "\n"
	if err := writePrivateFile(path, []byte(content)); err != nil {
		return "", fmt.Errorf("write testcase inspection file: %w", err)
	}
	return path, nil
}

func inspectionRawSection(data []byte, available bool, message string) string {
	if available {
		return string(data)
	}
	if message == "" {
		message = "unavailable"
	}
	return "[unavailable: " + message + "]"
}

func inspectionActualSection(state localReplayState) string {
	if state.Result == nil {
		return "[unavailable: local program was not run]"
	}
	return string(state.Result.Stdout)
}

func inspectionStderrSection(state localReplayState) string {
	if state.Result == nil {
		return "[unavailable: local program was not run]"
	}
	return string(state.Result.Stderr)
}

func editorCommand(path string, lookup func(string) string) (*exec.Cmd, error) {
	value := strings.TrimSpace(lookup("VISUAL"))
	if value == "" {
		value = strings.TrimSpace(lookup("EDITOR"))
	}
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil, errors.New("set VISUAL or EDITOR to inspect testcase data")
	}
	return exec.Command(parts[0], append(parts[1:], path)...), nil
}

func defaultEditorCommand(path string) (*exec.Cmd, error) {
	return editorCommand(path, os.Getenv)
}

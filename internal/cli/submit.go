package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"yoel/internal/graderapi"
	"yoel/internal/registry"

	"charm.land/huh/v2/spinner"
	lg "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

const (
	submissionPollInterval = time.Second
	submissionPollTimeout  = 2 * time.Minute
)

var errInvalidSubmissionFilename = errors.New("source filename must match <positive-problem-id>.<extension>")

func newSubmitCommand(sessions sessionProvider) *cobra.Command {
	return newSubmitCommandWithUpdateNotice(sessions, func(*cobra.Command) {})
}

func newSubmitCommandWithUpdateNotice(sessions sessionProvider, showUpdateNotice func(*cobra.Command)) *cobra.Command {
	var long bool
	command := &cobra.Command{
		Use:   "submit [file-or-directory]",
		Short: "Submit a source file or question directory",
		Long:  "Submit a source file or recursively resolve it from a question directory. If a named file does not exist directly, the current directory is searched recursively for its exact basename.",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			questions, err := registry.LoadDefault()
			if err != nil {
				return fmt.Errorf("load question registry: %w", err)
			}
			resolved, err := resolveSubmissionSourceWithRegistry(args[0], questions)
			if err != nil {
				return err
			}
			if resolved.UsedLegacy {
				if _, err := fmt.Fprintf(command.ErrOrStderr(), "Using legacy source discovery for %q.\n", args[0]); err != nil {
					return err
				}
			}
			source, err := os.ReadFile(resolved.Path)
			if err != nil {
				return fmt.Errorf("read source file: %w", err)
			}
			session, err := sessions(command)
			if err != nil {
				return err
			}
			client, err := graderapi.NewClient(session.BaseURL, nil)
			if err != nil {
				return fmt.Errorf("submit command: %w", err)
			}
			authenticatedClient := client.WithToken(session.Token)
			submission, err := authenticatedClient.Submit(command.Context(), resolved.ProblemID, graderapi.SubmissionRequest{
				Source:   string(source),
				Filename: resolved.Filename,
			})
			if err != nil {
				return err
			}
			err = spinner.New().WithOutput(command.ErrOrStderr()).ActionWithErr(func(context context.Context) error {
				submission, err = waitForSubmission(
					context,
					authenticatedClient,
					submission.ID,
					submissionPollInterval,
					submissionPollTimeout,
				)
				return err
			}).Title(fmt.Sprintf("Waiting for submission %d", submission.ID)).Run()
			if err != nil {
				return err
			}
			if interactiveTerminal(command) {
				if err = renderSubmissionResultInteractive(command, resolved.Path, authenticatedClient, submission); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintln(command.OutOrStdout(), renderSubmissionResult(long, submission)); err != nil {
					return err
				}
			}
			showUpdateNotice(command)
			return nil
		},
	}
	command.Flags().BoolVar(&long, "long", false, "show additional details of submission response")
	return command
}

var submissionCardStyle = lg.NewStyle().Border(lg.RoundedBorder()).Padding(0, 1)
var submissionCompleteStyle = lg.NewStyle().Bold(true).Foreground(lg.Blue)
var submissionFailureStyle = lg.NewStyle().Bold(true).Foreground(lg.Red)

func waitForSubmission(ctx context.Context, client *graderapi.Client, submissionID int, interval, timeout time.Duration) (graderapi.Submission, error) {
	pollContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		submission, err := client.GetSubmission(pollContext, submissionID)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				return graderapi.Submission{}, fmt.Errorf("wait for submission %d: timed out after %s: %w", submissionID, timeout, context.DeadlineExceeded)
			}
			return graderapi.Submission{}, err
		}
		if submissionFinished(submission.Status) {
			return submission, nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-pollContext.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return graderapi.Submission{}, ctx.Err()
			}
			return graderapi.Submission{}, fmt.Errorf("wait for submission %d: timed out after %s: %w", submissionID, timeout, context.DeadlineExceeded)
		case <-timer.C:
		}
	}
}

func submissionFinished(status string) bool {
	switch strings.ToLower(status) {
	case "done", "compilation_error", "grader_error":
		return true
	default:
		return false
	}
}

func renderSubmissionResultShort(submission graderapi.Submission) string {
	tt := titleStyle.Width(10)
	lines := []string{
		renderSubmissionHeading(submission.Status),
		formatRow(tt, valueStyle, Row{"Attempt", submission.Number}),
	}
	if submission.GraderComment != nil && strings.TrimSpace(*submission.GraderComment) != "" {
		var score string
		if submission.Points != nil {
			score = strconv.FormatFloat(*submission.Points, 'f', 2, 64)
		} else {
			score = "-"
		}
		lines = append(lines, formatRow(tt, valueStyle, Row{"Score", fmt.Sprintf("[%v] / %v %% ", *submission.GraderComment, score)}))
	}
	if submission.CompilerMessage != nil && strings.TrimSpace(*submission.CompilerMessage) != "" {
		lines = append(lines, "Compiler message", indentSubmissionMessage(*submission.CompilerMessage))
	}
	lines = append(lines, headStyle.Render("Result"))
	if len(submission.Evaluations) > 0 {
		for i, evaluation := range submission.Evaluations {
			result := "-"
			if evaluation.Result != nil {
				result = resultAsSym(*evaluation.Result)
			}
			lines = append(lines, formatRow(tt, valueStyle, Row{fmt.Sprintf("Test %v", i+1), result}))
		}
	}
	return submissionCardStyle.Render(lg.JoinVertical(lg.Left, lines...))
}
func renderSubmissionSummary(submission graderapi.Submission) string {
	tt := titleStyle.Width(10)
	lines := []string{
		renderSubmissionHeading(submission.Status),
		formatRow(tt, valueStyle, Row{"Attempt", submission.Number}),
	}
	if submission.GraderComment != nil && strings.TrimSpace(*submission.GraderComment) != "" {
		var score string
		if submission.Points != nil {
			score = strconv.FormatFloat(*submission.Points, 'f', 2, 64)
		} else {
			score = "-"
		}
		lines = append(lines, formatRow(tt, valueStyle, Row{"Score", fmt.Sprintf("[%v] / %v %% ", *submission.GraderComment, score)}))
	}
	if submission.CompilerMessage != nil && strings.TrimSpace(*submission.CompilerMessage) != "" {
		lines = append(lines, "Compiler message", indentSubmissionMessage(*submission.CompilerMessage))
	}
	return submissionCardStyle.Render(lg.JoinVertical(lg.Left, lines...))
}

func renderSubmissionResult(long bool, submission graderapi.Submission) string {
	if !long {
		return renderSubmissionResultShort(submission)
	}
	lines := []string{
		renderSubmissionHeading(submission.Status),
		fmt.Sprintf("ID       %d", submission.ID),
		fmt.Sprintf("Attempt  %d", submission.Number),
		fmt.Sprintf("Status   %s", submission.Status),
		fmt.Sprintf("Language %s", valueOrDash(submission.Language)),
	}
	if submission.Points != nil {
		lines = append(lines, fmt.Sprintf("Score    %g", *submission.Points))
	} else {
		lines = append(lines, "Score    -")
	}
	if submission.MaxRuntime != nil {
		lines = append(lines, fmt.Sprintf("Runtime  %g", *submission.MaxRuntime))
	}
	if submission.PeakMemory != nil {
		lines = append(lines, fmt.Sprintf("Memory   %d", *submission.PeakMemory))
	}
	if submission.GraderComment != nil && strings.TrimSpace(*submission.GraderComment) != "" {
		lines = append(lines, "Grader comment", indentSubmissionMessage(*submission.GraderComment))
	}
	if submission.CompilerMessage != nil && strings.TrimSpace(*submission.CompilerMessage) != "" {
		lines = append(lines, "Compiler message", indentSubmissionMessage(*submission.CompilerMessage))
	}
	if len(submission.Evaluations) > 0 {
		lines = append(lines, "Testcases")
		for _, evaluation := range submission.Evaluations {
			lines = append(lines, "  "+renderEvaluation(evaluation))
		}
	}
	return submissionCardStyle.Render(lg.JoinVertical(lg.Left, lines...))
}

func resultAsSym(result string) string {
	if result == "correct" {
		return "✔"
	}
	return "✗"
}

func renderSubmissionHeading(status string) string {
	switch strings.ToLower(status) {
	case "done":
		return submissionCompleteStyle.Render("Judging complete")
	case "compilation_error":
		return submissionFailureStyle.Render("✗ Compilation failed")
	case "grader_error":
		return submissionFailureStyle.Render("✗ Grader error")
	default:
		return "Submission status"
	}
}

func renderEvaluation(evaluation graderapi.Evaluation) string {
	parts := []string{fmt.Sprintf("Testcase %d", evaluation.TestcaseID)}
	if evaluation.Result != nil {
		parts = append(parts, *evaluation.Result)
	}
	if evaluation.Score != nil {
		parts = append(parts, fmt.Sprintf("score %g", *evaluation.Score))
	}
	if evaluation.Time != nil {
		parts = append(parts, fmt.Sprintf("time %d", *evaluation.Time))
	}
	if evaluation.Memory != nil {
		parts = append(parts, fmt.Sprintf("memory %d", *evaluation.Memory))
	}
	return strings.Join(parts, " · ")
}

func indentSubmissionMessage(message string) string {
	return lg.NewStyle().PaddingLeft(2).Render(strings.TrimSpace(message))
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func problemIDFromFilename(name string) (int, error) {
	base := filepath.Base(name)
	if strings.Count(base, ".") != 1 {
		return 0, errInvalidSubmissionFilename
	}
	separator := strings.IndexByte(base, '.')
	if separator <= 0 || separator == len(base)-1 {
		return 0, errInvalidSubmissionFilename
	}

	problemIDText := base[:separator]
	for _, digit := range problemIDText {
		if digit < '0' || digit > '9' {
			return 0, errInvalidSubmissionFilename
		}
	}
	problemID, err := strconv.Atoi(problemIDText)
	if err != nil || problemID <= 0 {
		return 0, errInvalidSubmissionFilename
	}
	return problemID, nil
}

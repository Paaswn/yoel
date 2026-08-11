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

	lg "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

const (
	submissionPollInterval = time.Second
	submissionPollTimeout  = 2 * time.Minute
)

var errInvalidSubmissionFilename = errors.New("source filename must match <positive-problem-id>.<extension>")

func newSubmitCommand(sessions sessionProvider) *cobra.Command {
	var long bool;
	command :=  &cobra.Command{
		Use:   "submit [file]",
		Short: "Submit a source file to the grader",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			sourcePath := args[0]
			filename := filepath.Base(sourcePath)
			problemID, err := problemIDFromFilename(filename)
			if err != nil {
				return err
			}

			source, err := os.ReadFile(sourcePath)
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
			submission, err := authenticatedClient.Submit(command.Context(), problemID, graderapi.SubmissionRequest{
				Source:   string(source),
				Filename: filename,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "Waiting for submission %d...\n", submission.ID); err != nil {
				return err
			}
			submission, err = waitForSubmission(
				command.Context(),
				authenticatedClient,
				submission.ID,
				submissionPollInterval,
				submissionPollTimeout,
			)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), renderSubmissionResult(long, submission))
			return err
		},
	}
	command.Flags().BoolVar(&long, "long", false, "show additional details of submission response")
	return command;
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
		var score string;
		if submission.Points != nil {
			score = strconv.FormatFloat(*submission.Points, 'f', 2, 64)
		} else {
			score = "-"
		}
		lines = append(lines, formatRow(tt, valueStyle, Row{"Score", fmt.Sprintf("[%v] / %v %% ",  *submission.GraderComment,  score)}))
	}
	if submission.CompilerMessage != nil && strings.TrimSpace(*submission.CompilerMessage) != "" {
		lines = append(lines, "Compiler message", indentSubmissionMessage(*submission.CompilerMessage))
	}
	if len(submission.Evaluations) > 0 {
		for _, evaluation := range submission.Evaluations {
			lines = append(lines, resultAsSym(*evaluation.Result))
		}
	}
	return submissionCardStyle.Render(lg.JoinVertical(lg.Left, lines...))
}
func renderSubmissionResult(long bool ,submission graderapi.Submission) string {
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
	if result == "" || result == "wrong" {
		return "✗"
	} 
	return "✔"
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

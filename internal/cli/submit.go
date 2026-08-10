package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"yoel/internal/graderapi"

	lg "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

var errInvalidSubmissionFilename = errors.New("source filename must match <positive-problem-id>.<extension>")

func newSubmitCommand(sessions sessionProvider) *cobra.Command {
	return &cobra.Command{
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
			submission, err := client.WithToken(session.Token).Submit(command.Context(), problemID, graderapi.SubmissionRequest{
				Source:   string(source),
				Filename: filename,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), renderSubmissionAcknowledgement(submission))
			return err
		},
	}
}

var submissionCardStyle = lg.NewStyle().Border(lg.RoundedBorder()).Padding(0, 1)
var submissionHeadingStyle = lg.NewStyle().Bold(true).Foreground(lg.Green)

func renderSubmissionAcknowledgement(submission graderapi.Submission) string {
	return submissionCardStyle.Render(lg.JoinVertical(
		lg.Left,
		submissionHeadingStyle.Render("✓ Submission created"),
		fmt.Sprintf("ID      %d", submission.ID),
		fmt.Sprintf("Attempt %d", submission.Number),
		fmt.Sprintf("Status  %s", submission.Status),
	))
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

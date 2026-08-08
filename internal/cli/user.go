package cli

import (
	"errors"
	"fmt"
	"time"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
)

const userTimeFormat = "2006-01-02 15:04:05 MST"

type userStatistics struct {
	LastSubmission    *time.Time
	ProblemsAttempted int
	TotalSubmissions  int
}

func newUserCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "user",
		Short: "Show the saved grader user and activity",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			now := time.Now()
			session, err := loadStoredSession()
			if err != nil {
				return err
			}

			tokenStatus := "expired"
			var statistics *userStatistics
			if now.Before(session.ExpiresAt) {
				client, err := graderapi.NewClient(session.BaseURL, nil)
				if err != nil {
					return fmt.Errorf("user command: %w", err)
				}
				problems, err := client.WithToken(session.Token).ListProblems(command.Context())
				if errors.Is(err, graderapi.ErrAuthentication) {
					tokenStatus = "rejected"
				} else if err != nil {
					return err
				} else {
					tokenStatus = "unexpired"
					computed := calculateUserStatistics(problems)
					statistics = &computed
				}
			}

			return printUser(command, session, tokenStatus, statistics)
		},
	}
}

func calculateUserStatistics(problems []graderapi.Problem) userStatistics {
	var statistics userStatistics
	for _, problem := range problems {
		statistics.TotalSubmissions += problem.SubmissionCount
		if problem.SubmissionCount > 0 {
			statistics.ProblemsAttempted++
		}
		if problem.LastSubmissionTime != nil &&
			(statistics.LastSubmission == nil || problem.LastSubmissionTime.After(*statistics.LastSubmission)) {
			lastSubmission := *problem.LastSubmissionTime
			statistics.LastSubmission = &lastSubmission
		}
	}
	return statistics
}

func printUser(command *cobra.Command, session storedSession, tokenStatus string, statistics *userStatistics) error {
	lastSubmission := "unavailable"
	problemsAttempted := "unavailable"
	totalSubmissions := "unavailable"
	if statistics != nil {
		lastSubmission = "never"
		if statistics.LastSubmission != nil {
			lastSubmission = statistics.LastSubmission.Local().Format(userTimeFormat)
		}
		problemsAttempted = fmt.Sprint(statistics.ProblemsAttempted)
		totalSubmissions = fmt.Sprint(statistics.TotalSubmissions)
	}

	output := command.OutOrStdout()
	if _, err := fmt.Fprintf(output, "Username:            %s\n", session.User.Login); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Full name:           %s\n", session.User.FullName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Last submission:     %s\n", lastSubmission); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Problems attempted:  %s\n", problemsAttempted); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Total submissions:   %s\n", totalSubmissions); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Token expires:       %s\n", session.ExpiresAt.Local().Format(userTimeFormat)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Token status:        %s\n", tokenStatus); err != nil {
		return err
	}
	_, err := fmt.Fprintln(output, "Cookie status:       not used")
	return err
}

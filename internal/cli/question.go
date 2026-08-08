package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"yoel/internal/graderapi"
)

func newQuestionCommand() *cobra.Command {
	question := &cobra.Command{
		Use:   "question",
		Short: "Work with grader questions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	question.AddCommand(newQuestionListCommand())
	return question
}

func newQuestionListCommand() *cobra.Command {
	var refresh bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List accessible questions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			now := time.Now()
			session, err := loadSession(now)
			if err != nil {
				return err
			}

			cache, cacheErr := loadQuestionCache()
			cacheMatches := cacheErr == nil &&
				cache.BaseURL == session.BaseURL &&
				cache.UserID == session.User.ID
			cacheFresh := cacheMatches && now.Sub(cache.FetchedAt) >= 0 && now.Sub(cache.FetchedAt) < questionCacheTTL

			problems := cache.Problems
			if refresh || !cacheFresh {
				client, err := graderapi.NewClient(session.BaseURL, nil)
				if err != nil {
					return fmt.Errorf("question list: %w", err)
				}
				problems, err = client.WithToken(session.Token).ListProblems(command.Context())
				if err != nil {
					return err
				}
				if err := saveQuestionCache(questionCache{
					BaseURL:   session.BaseURL,
					UserID:    session.User.ID,
					FetchedAt: now,
					Problems:  problems,
				}); err != nil {
					return fmt.Errorf("save question cache: %w", err)
				}
			}

			return printQuestions(command, problems)
		},
	}
	command.Flags().BoolVar(&refresh, "refresh", false, "refresh questions from the grader")
	return command
}

func printQuestions(command *cobra.Command, problems []graderapi.Problem) error {
	output := command.OutOrStdout()
	if _, err := fmt.Fprintln(output, "Question_Name Id Difficulty"); err != nil {
		return err
	}
	for _, problem := range problems {
		difficulty := "-"
		if problem.Difficulty != nil {
			difficulty = strconv.Itoa(*problem.Difficulty)
		}
		if _, err := fmt.Fprintf(output, "%s %d %s\n", problem.Name, problem.ID, difficulty); err != nil {
			return err
		}
	}
	return nil
}

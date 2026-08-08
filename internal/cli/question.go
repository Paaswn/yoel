package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"yoel/internal/graderapi"

	"github.com/spf13/cobra"
)

type fileOpener func(path string) error

func newQuestionCommand(opener fileOpener) *cobra.Command {
	question := &cobra.Command{
		Use:   "question",
		Short: "Work with grader questions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	question.AddCommand(newQuestionListCommand())
	question.AddCommand(newQuestionShowCommand(opener))
	question.AddCommand(newQuestionNewCommand(opener))
	return question
}

func newQuestionListCommand() *cobra.Command {
	var refresh bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List accessible questions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := loadSession(time.Now())
			if err != nil {
				return err
			}
			problems, err := getQuestions(command.Context(), session, refresh, time.Now())
			if err != nil {
				return err
			}
			return printQuestions(command, problems)
		},
	}
	command.Flags().BoolVar(&refresh, "refresh", false, "refresh questions from the grader")
	return command
}

func newQuestionShowCommand(opener fileOpener) *cobra.Command {
	var name string
	var refresh bool
	command := &cobra.Command{
		Use:   "show [id]",
		Short: "Open a question statement PDF",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return showQuestionPDF(command.Context(), opener, args, name, refresh)
		},
	}
	command.Flags().StringVar(&name, "name", "", "question name")
	command.Flags().BoolVar(&refresh, "refresh", false, "download the statement again")
	return command
}

func newQuestionNewCommand(opener fileOpener) *cobra.Command {
	var lang string
	newCmd := &cobra.Command{
		Use:   "new [id]",
		Short: "New file based on flag --language, defualt to .cpp",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			err := showQuestionPDF(command.Context(), opener, args, "", false)
			if err != nil {
				return errors.New("question may not exist")
			}
			calleDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}

			if lang == "" {
				lang = "cpp"
			}

			name := strings.Join([]string{args[0], lang}, ".")
			filePath := filepath.Join(calleDir, name)
			file, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("create source file: %w", err)
			}
			if _, err := file.WriteString("// --- Automatically Created by yoel ---\n"); err != nil {
				file.Close()
				return fmt.Errorf("write source file: %w", err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close source file: %w", err)
			}

			return nil
		},
	}
	newCmd.Flags().StringVar(&lang, "language", "", "language of choice")
	return newCmd
}

func showQuestionPDF(ctx context.Context, opener fileOpener, args []string, name string, refresh bool) error {
	if (len(args) == 0) == (name == "") {
		return errors.New("provide either a question id or --name")
	}
	session, err := loadStoredSession()
	if err != nil {
		return err
	}

	problemID, err := resolveQuestionID(ctx, session, args, name, refresh)
	if err != nil {
		return err
	}
	path, err := statementPDFPath(session.BaseURL, session.User.ID, problemID)
	if err != nil {
		return err
	}
	if !refresh && validCachedPDF(path) {
		return opener(path)
	}
	if !time.Now().Before(session.ExpiresAt) {
		return errors.New("saved session expired; run yoel login")
	}

	client, err := graderapi.NewClient(session.BaseURL, nil)
	if err != nil {
		return fmt.Errorf("question show: %w", err)
	}
	problemFile, err := client.WithToken(session.Token).DownloadProblemPDF(ctx, problemID)
	if err != nil {
		return err
	}
	if err := writePrivateFile(path, problemFile.Data); err != nil {
		return fmt.Errorf("cache statement PDF: %w", err)
	}
	return opener(path)
}

func getQuestions(ctx context.Context, session storedSession, refresh bool, now time.Time) ([]graderapi.Problem, error) {
	cache, cacheErr := loadQuestionCache()
	cacheMatches := cacheErr == nil &&
		cache.BaseURL == session.BaseURL &&
		cache.UserID == session.User.ID
	cacheFresh := cacheMatches && now.Sub(cache.FetchedAt) >= 0 && now.Sub(cache.FetchedAt) < questionCacheTTL
	if !refresh && cacheFresh {
		return cache.Problems, nil
	}

	client, err := graderapi.NewClient(session.BaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("question list: %w", err)
	}
	problems, err := client.WithToken(session.Token).ListProblems(ctx)
	if err != nil {
		return nil, err
	}
	if err := saveQuestionCache(questionCache{
		BaseURL:   session.BaseURL,
		UserID:    session.User.ID,
		FetchedAt: now,
		Problems:  problems,
	}); err != nil {
		return nil, fmt.Errorf("save question cache: %w", err)
	}
	return problems, nil
}

func resolveQuestionID(ctx context.Context, session storedSession, args []string, name string, refresh bool) (int, error) {
	if len(args) == 1 {
		id, err := strconv.Atoi(args[0])
		if err != nil || id <= 0 {
			return 0, errors.New("question id must be a positive integer")
		}
		return id, nil
	}

	cache, cacheErr := loadQuestionCache()
	cacheMatches := cacheErr == nil && cache.BaseURL == session.BaseURL && cache.UserID == session.User.ID
	problems := cache.Problems
	if refresh || !cacheMatches {
		if !time.Now().Before(session.ExpiresAt) {
			return 0, errors.New("saved session expired; run yoel login")
		}
		var err error
		problems, err = getQuestions(ctx, session, true, time.Now())
		if err != nil {
			return 0, err
		}
	}

	matchID, err := findQuestionID(problems, name)
	if err != nil || matchID != 0 {
		return matchID, err
	}
	if !refresh && cacheMatches && time.Now().Before(session.ExpiresAt) {
		problems, err = getQuestions(ctx, session, true, time.Now())
		if err != nil {
			return 0, err
		}
		matchID, err = findQuestionID(problems, name)
		if err != nil || matchID != 0 {
			return matchID, err
		}
	}
	return 0, fmt.Errorf("question named %q was not found", name)
}

func findQuestionID(problems []graderapi.Problem, name string) (int, error) {
	var matchID int
	for _, problem := range problems {
		if strings.EqualFold(problem.Name, name) {
			if matchID != 0 {
				return 0, fmt.Errorf("question name %q is ambiguous", name)
			}
			matchID = problem.ID
		}
	}
	return matchID, nil
}

func validCachedPDF(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	header := make([]byte, 5)
	if _, err := file.Read(header); err != nil {
		return false
	}
	return string(header) == "%PDF-"
}

func openWithDefaultViewer(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", filepath.Clean(path))
	default:
		command = exec.Command("xdg-open", path)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open PDF with default viewer: %w", err)
	}
	return nil
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

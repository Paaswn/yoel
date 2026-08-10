package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"yoel/internal/graderapi"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

type fileOpener func(path string) error

func newQuestionCommand(opener fileOpener, sessions sessionProvider) *cobra.Command {
	question := &cobra.Command{
		Use:   "question",
		Short: "Work with grader questions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	question.AddCommand(newQuestionListCommand(opener, sessions))
	question.AddCommand(newQuestionShowCommand(opener, sessions))
	question.AddCommand(newQuestionNewCommand(opener, sessions))
	return question
}

func newQuestionListCommand(opener fileOpener, sessions sessionProvider) *cobra.Command {
	var lang string
	var refresh bool
	command := &cobra.Command{
		Use:   "list",
		Short: "Interactively create a new question file",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := sessions(command)
			if err != nil {
				return err
			}
			problems, err := getQuestions(command.Context(), session, refresh, time.Now())
			if err != nil {
				return err
			}
			problem, err := showQuestions(command, problems)
			if err != nil {
				return err
			}
			return createQuestion(command, opener, sessions, problem, lang)
		},
	}
	command.Flags().StringVar(&lang, "language", "", "language of choice")
	command.Flags().BoolVar(&refresh, "refresh", false, "refresh questions from the grader")
	return command
}

func newQuestionShowCommand(opener fileOpener, sessions sessionProvider) *cobra.Command {
	var name string
	var refresh bool
	command := &cobra.Command{
		Use:   "show [id]",
		Short: "Open a question statement PDF",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return showQuestionPDF(command, opener, sessions, args, name, refresh)
		},
	}
	command.Flags().StringVar(&name, "name", "", "question name")
	command.Flags().BoolVar(&refresh, "refresh", false, "download the statement again")
	return command
}

func newQuestionNewCommand(opener fileOpener, sessions sessionProvider) *cobra.Command {
	var lang string
	newCmd := &cobra.Command{
		Use:   "new [id]",
		Short: "Create a new question file",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			problemID, err := strconv.Atoi(args[0])
			if err != nil || problemID <= 0 {
				return errors.New("question id must be a positive integer")
			}
			session, err := sessions(command)
			if err != nil {
				return err
			}
			client, err := graderapi.NewClient(session.BaseURL, nil)
			if err != nil {
				return fmt.Errorf("question new: %w", err)
			}
			problem, err := client.WithToken(session.Token).GetProblem(command.Context(), problemID)
			if err != nil {
				return err
			}
			return createQuestion(command, opener, sessions, problem, lang)
		},
	}
	newCmd.Flags().StringVar(&lang, "language", "", "language of choice")
	return newCmd
}

func createQuestion(command *cobra.Command, opener fileOpener, sessions sessionProvider, problem graderapi.Problem, lang string) error {
	session, err := sessions(command)
	if err != nil {
		return err
	}
	questionID := strconv.Itoa(problem.ID)
	pdfPath, err := openQuestionPDFForSession(command, opener, session, problem.ID, false)
	if err != nil {
		return err
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find current directory: %w", err)
	}
	if problem.HasAttachment {
		return createQuestionFromAttachment(command.Context(), session, problem, currentDir, pdfPath)
	}
	if lang == "" {
		lang = "cpp"
	}

	filePath := filepath.Join(currentDir, strings.Join([]string{questionID, lang}, "."))
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
}

func showQuestionPDF(command *cobra.Command, opener fileOpener, sessions sessionProvider, args []string, name string, refresh bool) error {
	if (len(args) == 0) == (name == "") {
		return errors.New("provide either a question id or --name")
	}
	session, err := loadStoredSession()
	if err != nil {
		return err
	}

	problemID, err := resolveQuestionID(command, sessions, &session, args, name, refresh)
	if err != nil {
		return err
	}
	cachedPath, err := statementPDFPath(session.BaseURL, session.User.ID, problemID)
	if err != nil {
		return err
	}
	if !refresh && validCachedPDF(cachedPath) {
		return opener(cachedPath)
	}
	if !time.Now().Before(session.ExpiresAt) {
		session, err = sessions(command)
		if err != nil {
			return err
		}
	}
	_, err = openQuestionPDFForSession(command, opener, session, problemID, refresh)
	return err
}

func openQuestionPDFForSession(command *cobra.Command, opener fileOpener, session storedSession, problemID int, refresh bool) (string, error) {
	pdfPath, err := statementPDFPath(session.BaseURL, session.User.ID, problemID)
	if err != nil {
		return "", err
	}
	if !refresh && validCachedPDF(pdfPath) {
		return pdfPath, opener(pdfPath)
	}

	client, err := graderapi.NewClient(session.BaseURL, nil)
	if err != nil {
		return "", fmt.Errorf("question show: %w", err)
	}
	problemFile, err := client.WithToken(session.Token).DownloadProblemPDF(command.Context(), problemID)
	if err != nil {
		return "", err
	}
	if err := writePrivateFile(pdfPath, problemFile.Data); err != nil {
		return "", fmt.Errorf("cache statement PDF: %w", err)
	}
	return pdfPath, opener(pdfPath)
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

func resolveQuestionID(command *cobra.Command, sessions sessionProvider, session *storedSession, args []string, name string, refresh bool) (int, error) {
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
			var err error
			*session, err = sessions(command)
			if err != nil {
				return 0, err
			}
		}
		var err error
		problems, err = getQuestions(command.Context(), *session, true, time.Now())
		if err != nil {
			return 0, err
		}
	}

	matchID, err := findQuestionID(problems, name)
	if err != nil || matchID != 0 {
		return matchID, err
	}
	if !refresh && cacheMatches {
		if !time.Now().Before(session.ExpiresAt) {
			*session, err = sessions(command)
			if err != nil {
				return 0, err
			}
		}
		problems, err = getQuestions(command.Context(), *session, true, time.Now())
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

type Row struct {
	title string
	val   any
}

var headStyle = lg.NewStyle().Bold(true)

func showQuestions(command *cobra.Command, problems []graderapi.Problem) (graderapi.Problem, error) {
	if len(problems) == 0 {
		return graderapi.Problem{}, errors.New("no accessible questions")
	}

	var selected int
	options := make([]huh.Option[int], 0, len(problems))
	for i, problem := range problems {
		options = append(options, huh.NewOption(problem.Name, i))
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().Title("Questions List").Options(
				options...,
			).
				Value(&selected).Height(5),

			huh.NewNote().DescriptionFunc(func() string {
				formatRow := func(row Row) string {
					return formatRow(titleStyle, valueStyle, row)
				}
				prob := &problems[selected]
				card := lg.JoinVertical(lg.Left,
					headStyle.Foreground(lg.Magenta).Render(prob.FullName),
					lg.NewStyle().Padding(0, 2, 0, 2).Border(lg.ASCIIBorder(), true).Render(
						lipgloss.JoinVertical(lg.Left,
							lipgloss.JoinVertical(lg.Left,
								headStyle.Render("Question"),
								formatRow(Row{"ID", prob.ID}),
								formatRow(Row{"Difficulty", solveDifficulty(prob.Difficulty)}),
								formatRow(Row{"Tags", prob.Tags}),
							),
							lipgloss.JoinVertical(lg.Left,
								headStyle.Render("Score"),
								formatRow(Row{"Best", prob.BestScore}),
								formatRow(Row{"Last", prob.LastScore}),
								formatRow(Row{"Submissions", prob.SubmissionCount}),
							),
							lipgloss.JoinVertical(lg.Left,
								headStyle.Render("Latest Submission"),
								formatRow(Row{"Result", prob.LastResult}),
								formatRow(Row{"Score", prob.LastScore}),
							),
						),
					),
				)
				return card
			},
				&selected),
		),
	).WithInput(command.InOrStdin()).WithOutput(command.OutOrStdout())
	if err := form.RunWithContext(command.Context()); err != nil {
		return graderapi.Problem{}, err
	}
	return problems[selected], nil
}

func solveDifficulty(diff *int) string {
	var builder strings.Builder
	if diff == nil {
		return "-"
	}
	for i := 0; i < *diff; i += 1 {
		builder.WriteRune('★')
	}
	return builder.String()
}

var titleStyle = lg.NewStyle().Faint(true).Width(20).PaddingLeft(1)
var valueStyle = lg.NewStyle().Bold(true).Italic(false)

func formatRow(title lg.Style, value lg.Style, row Row) string {
	var builder strings.Builder
	builder.WriteString(title.Render(row.title))
	valOfcont := reflect.ValueOf(row.val)
	if valOfcont.Kind() == reflect.Pointer {
		valOfcont = valOfcont.Elem()
	}
	if !valOfcont.IsValid() {
		builder.WriteString(value.Render("-"))
	} else {
		builder.WriteString(value.Render(fmt.Sprintf("%v", valOfcont.Interface())))
	}
	return builder.String()
}

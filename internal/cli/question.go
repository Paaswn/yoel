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
	"charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
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
			var problems []graderapi.Problem
			if term.IsTerminal(os.Stderr.Fd()) {
				err = spinner.New().WithOutput(command.ErrOrStderr()).Title("Fetching questions...").ActionWithErr(
					func(context context.Context) error {
						problems, err = getQuestions(context, session, refresh, time.Now())
						if err != nil {
							return err
						}
						return nil
					},
				).Run()
				if err != nil {
					return err
				}
			} else {
				problems, err = getQuestions(command.Context(), session, refresh, time.Now())
				if err != nil {
					return err
				}
			}
			if err := refreshQuestionRegistry(problems); err != nil {
				return fmt.Errorf("refresh question registry: %w", err)
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
	var problemID int
	var refresh bool
	newCmd := &cobra.Command{
		Use:   "new <query>",
		Short: "Create a new question by ID, order, name, or tag",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			idProvided := command.Flags().Changed("id")
			if (len(args) == 1) == idProvided {
				return errors.New("provide either a question query or --id")
			}
			query := ""
			if idProvided {
				if problemID <= 0 {
					return errors.New("question id must be a positive integer")
				}
				query = strconv.Itoa(problemID)
			} else {
				query = args[0]
				if numericQuery, err := strconv.Atoi(query); err == nil && numericQuery <= 0 {
					return errors.New("question order must be a positive integer")
				}
			}
			session, err := sessions(command)
			if err != nil {
				return err
			}
			problem, err := resolveProblem(command, session, query, refresh)
			if err != nil {
				return err
			}
			return createQuestion(command, opener, sessions, problem, lang)
		},
	}
	newCmd.Flags().StringVar(&lang, "language", "", "language of choice")
	newCmd.Flags().IntVar(&problemID, "id", 0, "grader problem ID")
	newCmd.Flags().BoolVar(&refresh, "refresh", false, "refresh questions from the grader")
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

	problemDir, err := questionDirectory(currentDir, problem.Name)
	if err != nil {
		return err
	}

	temporaryDir, err := os.MkdirTemp(currentDir, ".yoel-question-*")
	if err != nil {
		return fmt.Errorf("create temporary question directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)

	if _, err := os.Lstat(problemDir); err == nil {
		return fmt.Errorf("create question directory: %s already exists", problemDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect question directory: %w", err)
	}

	sourcePath := ""
	if problem.HasAttachment {
		files, err := createQuestionFromAttachment(command.Context(), session, problem, temporaryDir)
		if err != nil {
			return err
		}
		if len(files) == 1 {
			relativePath, err := filepath.Rel(temporaryDir, files[0])
			if err != nil {
				return fmt.Errorf("resolve attachment source path: %w", err)
			}
			sourcePath = filepath.Join(problemDir, relativePath)
		}
	} else {
		if lang == "" {
			lang = "cpp"
		}
		if err := createQuestionNoAttachment(temporaryDir, questionID, lang); err != nil {
			return err
		}
		sourcePath = filepath.Join(problemDir, questionID+"."+lang)
	}
	if err := createQuestionPDFReference(temporaryDir, problemDir, pdfPath); err != nil {
		return err
	}

	if err := os.Rename(temporaryDir, problemDir); err != nil {
		return fmt.Errorf("create question directory: %w", err)
	}
	if err := recordCreatedQuestion(problem, problemDir, sourcePath); err != nil {
		return recordCreatedQuestionError(problem.ID, err)
	}
	fmt.Fprintln(command.ErrOrStderr(), lg.NewStyle().Faint(true).Render("✔ Created question directory at", problemDir))
	return nil
}
func createQuestionNoAttachment(temporaryDir string, questionID string, lang string) error {
	if lang == "" {
		lang = "cpp"
	}
	filePath := filepath.Join(temporaryDir, strings.Join([]string{questionID, lang}, "."))
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create source file: %w", err)
	}
	if _, err := file.WriteString("// --- Automatically Created by yoel ---\n#include <iostream>\nusing namespace std;\n\nint main(){\n\n}"); err != nil {
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
	query := name
	if len(args) == 1 {
		query = args[0]
		if id, err := strconv.Atoi(query); err == nil && id > 0 {
			return id, nil
		}
	}
	if !time.Now().Before(session.ExpiresAt) {
		var err error
		*session, err = sessions(command)
		if err != nil {
			return 0, err
		}
	}
	problem, err := resolveProblem(command, *session, query, refresh)
	if err != nil {
		return 0, err
	}
	return problem.ID, nil
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
		var problemName string
		if problem.BestScore == nil {
			problemName = lg.NewStyle().Render("○", problem.Name)
		} else if *problem.BestScore == 100 {
			problemName = lg.NewStyle().Foreground(lg.Green).Render("●", problem.Name)
		} else {
			problemName = lg.NewStyle().Foreground(lg.Yellow).Render("◐", problem.Name)
		}
		options = append(options, huh.NewOption(problemName, i))
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
					lg.NewStyle().Padding(0, 2, 0, 2).Border(lg.RoundedBorder(), true).Render(
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
							lipgloss.JoinVertical(lg.Left,
								headStyle.Render("Resources"),
								formatRow(Row{"Attachment", boolAsSym(prob.HasAttachment)}),
								formatRow(Row{"Test Cases", boolAsSym(prob.HasTestcase)}),
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
func boolAsSym(b bool) string {
	if !b {
		return "✗"
	}
	return "✔"
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

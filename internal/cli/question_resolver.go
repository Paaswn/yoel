package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"yoel/internal/graderapi"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

// resolveProblem resolves the way a user refers to a question against the
// cached problem metadata. A cache miss is refreshed once so newly available
// questions can be found without making normal lookups depend on the network.
func resolveProblem(command *cobra.Command, session storedSession, query string, refresh bool) (graderapi.Problem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return graderapi.Problem{}, fmt.Errorf("question query is empty")
	}

	cache, cacheErr := loadQuestionCache()
	cacheMatches := cacheErr == nil && cache.BaseURL == session.BaseURL && cache.UserID == session.User.ID
	problems := cache.Problems
	if refresh || !cacheMatches {
		var err error
		problems, err = getQuestions(command.Context(), session, true, time.Now())
		if err != nil {
			return graderapi.Problem{}, err
		}
		if err := refreshQuestionRegistry(problems); err != nil {
			return graderapi.Problem{}, fmt.Errorf("refresh question registry: %w", err)
		}
	}

	matches := searchProblems(problems, query)
	if len(matches) == 0 && !refresh {
		var err error
		problems, err = getQuestions(command.Context(), session, true, time.Now())
		if err != nil {
			return graderapi.Problem{}, err
		}
		if err := refreshQuestionRegistry(problems); err != nil {
			return graderapi.Problem{}, fmt.Errorf("refresh question registry: %w", err)
		}
		matches = searchProblems(problems, query)
	}
	if len(matches) == 0 {
		return graderapi.Problem{}, fmt.Errorf("no question matching %q was found", query)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return selectProblem(command, matches)
}

// searchProblems returns only the highest-priority group of matches. This
// keeps an exact ID or name from being obscured by broader text matches.
func searchProblems(problems []graderapi.Problem, query string) []graderapi.Problem {
	if id, err := strconv.Atoi(query); err == nil && id > 0 {
		if matches := filterProblems(problems, func(problem graderapi.Problem, _ int) bool { return problem.ID == id }); len(matches) > 0 {
			return matches
		}
		if matches := filterProblems(problems, func(_ graderapi.Problem, index int) bool { return index+1 == id }); len(matches) > 0 {
			return matches
		}
	}

	normalizedQuery := normalizeProblemQuery(query)
	if normalizedQuery == "" {
		return nil
	}
	fields := func(problem graderapi.Problem) []string {
		return []string{normalizeProblemQuery(problem.Name), normalizeProblemQuery(problem.FullName)}
	}
	if matches := filterProblems(problems, func(problem graderapi.Problem, _ int) bool {
		for _, field := range fields(problem) {
			if field == normalizedQuery {
				return true
			}
		}
		return false
	}); len(matches) > 0 {
		return matches
	}
	if matches := filterProblems(problems, func(problem graderapi.Problem, _ int) bool {
		for _, field := range fields(problem) {
			if strings.HasPrefix(field, normalizedQuery) {
				return true
			}
		}
		return false
	}); len(matches) > 0 {
		return matches
	}
	if matches := filterProblems(problems, func(problem graderapi.Problem, _ int) bool {
		for _, field := range fields(problem) {
			if strings.Contains(field, normalizedQuery) {
				return true
			}
		}
		return false
	}); len(matches) > 0 {
		return matches
	}
	return filterProblems(problems, func(problem graderapi.Problem, _ int) bool {
		for _, tag := range problem.Tags {
			if strings.Contains(normalizeProblemQuery(tag), normalizedQuery) {
				return true
			}
		}
		return false
	})
}

func filterProblems(problems []graderapi.Problem, match func(graderapi.Problem, int) bool) []graderapi.Problem {
	matches := make([]graderapi.Problem, 0)
	for index, problem := range problems {
		if match(problem, index) {
			matches = append(matches, problem)
		}
	}
	return matches
}

func normalizeProblemQuery(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

func selectProblem(command *cobra.Command, problems []graderapi.Problem) (graderapi.Problem, error) {
	var selected int
	options := make([]huh.Option[int], 0, len(problems))
	for index, problem := range problems {
		label := problem.FullName
		if label == "" {
			label = problem.Name
		}
		options = append(options, huh.NewOption(fmt.Sprintf("%d — %s", problem.ID, label), index))
	}
	form := huh.NewForm(huh.NewGroup(huh.NewSelect[int]().Title("Select question").Options(options...).Value(&selected).Height(5))).WithInput(command.InOrStdin()).WithOutput(command.OutOrStdout())
	if err := form.RunWithContext(command.Context()); err != nil {
		return graderapi.Problem{}, err
	}
	return problems[selected], nil
}

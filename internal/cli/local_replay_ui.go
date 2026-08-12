package cli

import (
	"context"
	"fmt"
	"strings"

	"yoel/internal/graderapi"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

const localReplayDetailLimit = 8 << 10

type localReplayMessage struct {
	event localReplayEvent
	ok    bool
}

type submissionResultModel struct {
	form     *huh.Form
	selected int
	revision int
	updates  <-chan localReplayEvent
	states   map[int]localReplayState
}

func newSubmissionResultModel(submission graderapi.Submission, updates <-chan localReplayEvent) *submissionResultModel {
	model := &submissionResultModel{
		updates: updates,
		states:  make(map[int]localReplayState),
	}
	options := make([]huh.Option[int], 0, len(submission.Evaluations))
	for index, evaluation := range submission.Evaluations {
		options = append(options, huh.NewOption(renderEvaluationOption(evaluation), index))
	}
	model.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Description(renderSubmissionSummary(submission)),
			huh.NewSelect[int]().Title("Test Cases Result").Options(options...).Value(&model.selected),
			huh.NewNote().DescriptionFunc(func() string {
				return model.renderSelectedDetail(submission)
			}, struct {
				Selected *int
				Revision *int
			}{&model.selected, &model.revision}),
		),
	)
	model.form.SubmitCmd = tea.Quit
	model.form.CancelCmd = tea.Quit
	return model
}

func (m *submissionResultModel) Init() tea.Cmd {
	return tea.Batch(m.form.Init(), waitForLocalReplay(m.updates))
}

func (m *submissionResultModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd
	if replay, ok := message.(localReplayMessage); ok && replay.ok {
		m.states[replay.event.Index] = replay.event.State
		m.revision++
		commands = append(commands, waitForLocalReplay(m.updates))
	}
	formModel, command := m.form.Update(message)
	if form, ok := formModel.(*huh.Form); ok {
		m.form = form
	}
	commands = append(commands, command)
	if m.form.State != huh.StateNormal {
		commands = append(commands, tea.Quit)
	}
	return m, tea.Batch(commands...)
}

func (m *submissionResultModel) View() tea.View {
	return tea.NewView(m.form.View())
}

func (m *submissionResultModel) renderSelectedDetail(submission graderapi.Submission) string {
	if m.selected < 0 || m.selected >= len(submission.Evaluations) {
		return ""
	}
	detail := renderEvaluation(submission.Evaluations[m.selected])
	state, exists := m.states[m.selected]
	if !exists {
		return detail
	}
	return detail + "\n\n" + renderLocalReplayState(state)
}

func waitForLocalReplay(updates <-chan localReplayEvent) tea.Cmd {
	if updates == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-updates
		return localReplayMessage{event: event, ok: ok}
	}
}

func newRenderSubmissionResult(command *cobra.Command, sourcePath string, client *graderapi.Client, submission graderapi.Submission) error {
	if len(submission.Evaluations) == 0 {
		_, err := fmt.Fprintln(command.OutOrStdout(), renderSubmissionSummary(submission))
		return err
	}

	replayContext, cancel := context.WithCancel(command.Context())
	defer cancel()
	updates := make(chan localReplayEvent, max(1, len(submission.Evaluations)*4))
	go func() {
		defer close(updates)
		coordinateLocalReplay(replayContext, sourcePath, submission, defaultLocalReplayDependencies(client), func(event localReplayEvent) {
			select {
			case updates <- event:
			case <-replayContext.Done():
			}
		})
	}()

	model := newSubmissionResultModel(submission, updates)
	finalModel, err := tea.NewProgram(
		model,
		tea.WithContext(replayContext),
		tea.WithInput(command.InOrStdin()),
		tea.WithOutput(command.OutOrStdout()),
	).Run()
	if err != nil {
		return err
	}
	if final, ok := finalModel.(*submissionResultModel); ok && final.form.State == huh.StateAborted {
		return huh.ErrUserAborted
	}
	return nil
}

func renderEvaluationOption(evaluation graderapi.Evaluation) string {
	if evaluation.Result == nil {
		return "-"
	}
	if strings.EqualFold(*evaluation.Result, "correct") {
		return lg.NewStyle().Foreground(lg.Green).Render("CORRECT")
	}
	return lg.NewStyle().Foreground(lg.Red).Render(strings.ToUpper(*evaluation.Result))
}

func renderLocalReplayState(state localReplayState) string {
	heading := "Local replay · " + string(state.Status)
	if state.Status == localReplayCompileFailed {
		if strings.TrimSpace(state.CompilerOutput) == "" {
			return heading
		}
		return heading + "\n\nCompiler output\n" + limitReplayDetail(state.CompilerOutput)
	}
	if state.Status != localReplayFinished || state.Result == nil {
		return heading
	}
	result := *state.Result
	heading = "Local replay · " + strings.ReplaceAll(localRunStatus(result), "_", " ")
	sections := []string{heading}
	sections = append(sections, "Input\n"+limitReplayDetail(string(state.Testcase.Input)))
	sections = append(sections, "Expected\n"+limitReplayDetail(string(state.Testcase.Expected)))
	sections = append(sections, "Got\n"+limitReplayDetail(string(result.Stdout)))
	if len(result.Stderr) > 0 {
		sections = append(sections, "Stderr\n"+limitReplayDetail(string(result.Stderr)))
	}
	return strings.Join(sections, "\n\n")
}

func limitReplayDetail(value string) string {
	if len(value) <= localReplayDetailLimit {
		return value
	}
	return value[:localReplayDetailLimit] + "\n… output truncated for display"
}

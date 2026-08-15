package cli

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"yoel/internal/graderapi"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	lg "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

type localReplayMessage struct {
	event localReplayEvent
	ok    bool
}

type inspectionPreparedMessage struct {
	index int
	state localReplayState
	path  string
	err   error
}

type inspectionEditorFinishedMessage struct{ err error }

type submissionResultModel struct {
	form             *huh.Form
	selected         int
	revision         int
	updates          <-chan localReplayEvent
	states           map[int]localReplayState
	width            int
	sourcePath       string
	submissionID     int
	context          context.Context
	downloadExpected func(context.Context, int) ([]byte, error)
	writeExpected    func(string, int, int, []byte) error
	buildEditor      func(string) (*exec.Cmd, error)
	notice           string
}

func newSubmissionResultModel(submission graderapi.Submission, updates <-chan localReplayEvent) *submissionResultModel {
	submission = normalizeInteractiveSubmission(submission)
	model := &submissionResultModel{
		updates:     updates,
		states:      make(map[int]localReplayState),
		width:       defaultInspectionWidth,
		context:     context.Background(),
		buildEditor: defaultEditorCommand,
	}
	options := make([]huh.Option[int], 0, len(submission.Evaluations))
	for index, evaluation := range submission.Evaluations {
		options = append(options, huh.NewOption(renderEvaluationOption(evaluation), index))
	}
	model.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Description(renderSubmissionSummary(submission)),
			huh.NewSelect[int]().Title("Test Cases Result").Options(options...).Value(&model.selected),
		),
		huh.NewGroup(
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

func newSubmissionResultModelForReplay(ctx context.Context, sourcePath string, submission graderapi.Submission, updates <-chan localReplayEvent, downloadExpected func(context.Context, int) ([]byte, error)) *submissionResultModel {
	model := newSubmissionResultModel(submission, updates)
	model.context = ctx
	model.sourcePath = sourcePath
	model.submissionID = submission.ID
	model.downloadExpected = downloadExpected
	model.writeExpected = writeLocalReplayExpectedCache
	return model
}

func (m *submissionResultModel) Init() tea.Cmd {
	return tea.Batch(m.form.Init(), waitForLocalReplay(m.updates))
}

func (m *submissionResultModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd
	if size, ok := message.(tea.WindowSizeMsg); ok && size.Width > 0 {
		m.width = size.Width
		m.revision++
	}
	if replay, ok := message.(localReplayMessage); ok && replay.ok {
		m.states[replay.event.Index] = replay.event.State
		m.revision++
		commands = append(commands, waitForLocalReplay(m.updates))
	}
	if prepared, ok := message.(inspectionPreparedMessage); ok {
		if prepared.err != nil {
			m.notice = "Open testcase data: " + prepared.err.Error()
			m.revision++
			return m, tea.Batch(commands...)
		}
		m.states[prepared.index] = prepared.state
		m.revision++
		command, err := m.buildEditor(prepared.path)
		if err != nil {
			m.notice = "Open testcase data: " + err.Error()
			return m, tea.Batch(commands...)
		}
		return m, tea.Batch(append(commands, tea.ExecProcess(command, func(err error) tea.Msg {
			return inspectionEditorFinishedMessage{err: err}
		}))...)
	}
	if editor, ok := message.(inspectionEditorFinishedMessage); ok && editor.err != nil {
		m.notice = "Editor failed: " + editor.err.Error()
		m.revision++
		return m, tea.Batch(commands...)
	}
	if m.openInspectionKey(message) {
		command := m.prepareInspection()
		if command == nil {
			m.revision++
			return m, tea.Batch(commands...)
		}
		return m, tea.Batch(append(commands, command)...)
	}
	if ignoreReplayDetailKey(m.form, message) {
		return m, tea.Batch(commands...)
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

func (m *submissionResultModel) openInspectionKey(message tea.Msg) bool {
	if _, ok := m.form.GetFocusedField().(*huh.Note); !ok {
		return false
	}
	key, ok := message.(tea.KeyPressMsg)
	return ok && key.Mod == 0 && key.Code == 'e'
}

func (m *submissionResultModel) prepareInspection() tea.Cmd {
	state, exists := m.states[m.selected]
	if !exists || !state.InputAvailable {
		m.notice = "Testcase input is still loading or unavailable."
		return nil
	}
	if m.sourcePath == "" || m.submissionID <= 0 {
		m.notice = "Testcase inspection is unavailable for this result."
		return nil
	}
	index := m.selected
	return func() tea.Msg {
		if !state.ExpectedAvailable && m.downloadExpected != nil {
			expected, err := m.downloadExpected(m.context, state.Testcase.ID)
			if err != nil {
				state.ExpectedError = "expected output is unavailable"
			} else {
				state.Testcase.Expected = expected
				state.ExpectedAvailable = true
				state.ExpectedError = ""
				if m.writeExpected != nil {
					_ = m.writeExpected(m.sourcePath, m.submissionID, state.Testcase.ID, expected)
				}
			}
		}
		state.Inspection = buildTestcaseInspectionDetails(state)
		path, err := writeInspectionFile(m.sourcePath, m.submissionID, state)
		return inspectionPreparedMessage{index: index, state: state, path: path, err: err}
	}
}

func ignoreReplayDetailKey(form *huh.Form, message tea.Msg) bool {
	if _, ok := form.GetFocusedField().(*huh.Note); !ok {
		return false
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return false
	}
	if key.Mod&tea.ModCtrl != 0 && key.Code == 'c' {
		return false
	}
	if key.Code == tea.KeyEnter || key.Code == tea.KeyReturn {
		return false
	}
	return key.Code != tea.KeyTab || key.Mod != tea.ModShift
}

func (m *submissionResultModel) View() tea.View {
	return tea.NewView(m.form.View())
}

func (m *submissionResultModel) renderSelectedDetail(submission graderapi.Submission) string {
	if m.selected < 0 || m.selected >= len(submission.Evaluations) {
		return ""
	}
	state, exists := m.states[m.selected]
	if !exists {
		return ""
	}
	if !state.Inspection.DetailsPrepared {
		state.Inspection = buildTestcaseInspectionDetails(state)
		m.states[m.selected] = state
	}
	detail := renderLocalReplayStateAtWidth(state, m.width)
	if m.notice != "" {
		detail += "\n\n" + m.notice
	}
	return detail
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

func renderSubmissionResultInteractive(command *cobra.Command, sourcePath string, client *graderapi.Client, submission graderapi.Submission) error {
	if len(submission.Evaluations) == 0 {
		_, err := fmt.Fprintln(command.OutOrStdout(), renderSubmissionSummary(submission))
		return err
	}
	submission = normalizeInteractiveSubmission(submission)

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

	model := newSubmissionResultModelForReplay(replayContext, sourcePath, submission, updates, client.DownloadTestcaseSolution)
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

func normalizeInteractiveSubmission(submission graderapi.Submission) graderapi.Submission {
	submission.Evaluations = append([]graderapi.Evaluation(nil), submission.Evaluations...)
	sort.SliceStable(submission.Evaluations, func(left, right int) bool {
		return submission.Evaluations[left].TestcaseID < submission.Evaluations[right].TestcaseID
	})
	if len(submission.Evaluations) > 0 {
		pattern := evaluationResultPattern(submission.Evaluations)
		submission.GraderComment = &pattern
	}
	return submission
}

func evaluationResultPattern(evaluations []graderapi.Evaluation) string {
	var pattern strings.Builder
	pattern.Grow(len(evaluations))
	for _, evaluation := range evaluations {
		if evaluation.Result != nil && strings.EqualFold(strings.TrimSpace(*evaluation.Result), "correct") {
			pattern.WriteByte('P')
		} else {
			pattern.WriteByte('-')
		}
	}
	return pattern.String()
}

func renderEvaluationOption(evaluation graderapi.Evaluation) string {
	if evaluation.Result == nil {
		return "-"
	}
	if strings.EqualFold(*evaluation.Result, "correct") {
		return lg.NewStyle().Foreground(lg.Green).Render("CORRECT")
	}
	return lg.NewStyle().Foreground(lg.Red).Render(strings.ToUpper(*evaluation.Result) , "⏎")
}

func renderLocalReplayState(state localReplayState) string {
	return renderLocalReplayStateAtWidth(state, defaultInspectionWidth)
}

func renderLocalReplayStateAtWidth(state localReplayState, width int) string {
	if !state.Inspection.DetailsPrepared {
		state.Inspection = buildTestcaseInspectionDetails(state)
	}
	return renderTestcaseInspection(state, width)
}

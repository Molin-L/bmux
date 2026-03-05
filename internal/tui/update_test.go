package tui

import (
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

func TestIssuesLoadedNoIssuesAvailable(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)

	next, _ := m.Update(issuesLoadedMsg{
		issues: []model.Issue{},
		state:  app.IssueSourceState{Available: true},
	})

	got := next.(Model)
	if got.status != "No ready issues. Press r to refresh." {
		t.Fatalf("status = %q", got.status)
	}
}

func TestIssuesLoadedNoIssuesUnavailable(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)

	next, _ := m.Update(issuesLoadedMsg{
		issues: []model.Issue{},
		state:  app.IssueSourceState{Available: false, Reason: "bd not found in PATH"},
	})

	got := next.(Model)
	want := "No ready issues. Issue source unavailable: bd not found in PATH."
	if got.status != want {
		t.Fatalf("status = %q, want %q", got.status, want)
	}
}

func TestActionWhileSourceUnavailableDoesNotDispatch(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.issues = []model.Issue{{ID: "bd-1"}}
	m.issueSourceAvailable = false
	m.issueSourceReason = "bd not found in PATH"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	got := next.(Model)
	if cmd != nil {
		t.Fatalf("expected no command dispatch")
	}
	want := "No ready issues. Issue source unavailable: bd not found in PATH."
	if got.status != want {
		t.Fatalf("status = %q, want %q", got.status, want)
	}
	if got.busy {
		t.Fatalf("expected not busy")
	}
}

func TestActionWithNoIssuesRemainsNoop(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.issueSourceAvailable = true

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if cmd != nil {
		t.Fatalf("expected no command dispatch")
	}
	if got.busy {
		t.Fatalf("expected not busy")
	}
}

func TestNOpensAgentSelector(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if got.mode != modeAgentSelect {
		t.Fatalf("mode = %v", got.mode)
	}
}

func TestNewModelIncludesApeModeOption(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	if len(m.taskModeOptions) != 3 {
		t.Fatalf("task mode options len = %d", len(m.taskModeOptions))
	}
	if got, want := m.taskModeOptions[2], model.RunModeApe; got != want {
		t.Fatalf("task mode option[2] = %q, want %q", got, want)
	}
}

func TestEnterDispatchesUsingCurrentMode(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.issueSourceAvailable = true
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = firstSelectableRow(m.rows)
	m.selectedTaskIssueIDs = map[string]struct{}{"bd-1": {}}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd == nil {
		t.Fatalf("expected command dispatch")
	}
	if !got.busy {
		t.Fatalf("expected busy")
	}
}

func TestEnterWithoutSelectedTaskDoesNotDispatch(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.issueSourceAvailable = true
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = firstSelectableRow(m.rows)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("expected no command dispatch")
	}
	if got.busy {
		t.Fatalf("expected not busy")
	}
	if got.status != "No tasks selected. Press Space to select task(s)." {
		t.Fatalf("status = %q", got.status)
	}
}

func TestShiftTabCyclesCurrentMode(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.taskModeSelected = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	got := next.(Model)
	if got.taskModeSelected != len(got.taskModeOptions)-1 {
		t.Fatalf("taskModeSelected = %d, want %d", got.taskModeSelected, len(got.taskModeOptions)-1)
	}
}

func TestSpaceTogglesMultipleSelectedTasks(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-2", Title: "Task 2", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = firstSelectableRow(m.rows)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got := next.(Model)
	if len(got.selectedTaskIssueIDs) != 1 {
		t.Fatalf("selected count = %d", len(got.selectedTaskIssueIDs))
	}
	if _, ok := got.selectedTaskIssueIDs["bd-1"]; !ok {
		t.Fatalf("expected bd-1 selected")
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = next.(Model)
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got = next.(Model)
	if len(got.selectedTaskIssueIDs) != 2 {
		t.Fatalf("selected count = %d", len(got.selectedTaskIssueIDs))
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got = next.(Model)
	if len(got.selectedTaskIssueIDs) != 1 {
		t.Fatalf("selected count = %d", len(got.selectedTaskIssueIDs))
	}
	if _, ok := got.selectedTaskIssueIDs["bd-1"]; !ok {
		t.Fatalf("expected bd-1 to remain selected")
	}
}

func TestSelectionPrunesMissingIssuesOnRefresh(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.selectedTaskIssueIDs = map[string]struct{}{
		"bd-1":       {},
		"bd-missing": {},
	}

	next, _ := m.Update(issuesLoadedMsg{
		issues: []model.Issue{
			{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		},
		state: app.IssueSourceState{Available: true},
	})
	got := next.(Model)
	if len(got.selectedTaskIssueIDs) != 1 {
		t.Fatalf("selected count = %d", len(got.selectedTaskIssueIDs))
	}
	if _, ok := got.selectedTaskIssueIDs["bd-1"]; !ok {
		t.Fatalf("expected bd-1 to remain selected")
	}
}

func TestAgentSelectorEscReturnsMain(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.mode = modeAgentSelect
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(Model)
	if got.mode != modeMain {
		t.Fatalf("mode = %v", got.mode)
	}
}

func TestAgentSelectorEnterDispatchesCommand(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.mode = modeAgentSelect
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if !got.busy {
		t.Fatalf("expected busy true")
	}
	if cmd == nil {
		t.Fatalf("expected command dispatch")
	}
}

func TestConfirmCreateOnlyWhenPlanMode(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("unexpected command in main mode")
	}
	if got.mode != modeMain {
		t.Fatalf("mode = %v", got.mode)
	}

	m.mode = modePlanConfirm
	m.extractedPlan = app.PlanPayload{
		Goal: "g", Epic: app.PlanItem{Title: "E", Priority: 1}, Task: app.PlanItem{Title: "T", Priority: 1}, Subtasks: []app.PlanItem{{Title: "S", Priority: 2}},
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = next.(Model)
	if !got.busy {
		t.Fatalf("expected busy true")
	}
	if cmd == nil {
		t.Fatalf("expected command in confirm mode")
	}
}

func TestAgentSelectErrorKeepsSelectorAndShowsHint(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.mode = modeAgentSelect
	m.selectedAgent = "codex"

	next, _ := m.Update(actionResultMsg{err: assertErr("boom")})
	got := next.(Model)
	if got.mode != modeAgentSelect {
		t.Fatalf("mode = %v", got.mode)
	}
	if got.errorHint == "" {
		t.Fatalf("expected error hint")
	}
}

func TestAgentSelectSuccessClearsHint(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.mode = modeAgentSelect
	m.errorHint = "old"

	next, _ := m.Update(actionResultMsg{status: "Planning pane %3 started with codex (PATH fallback).", paneID: "%3"})
	got := next.(Model)
	if got.mode != modeMain {
		t.Fatalf("mode = %v", got.mode)
	}
	if got.errorHint != "" {
		t.Fatalf("errorHint = %q", got.errorHint)
	}
	if got.pendingPaneID != "%3" {
		t.Fatalf("pendingPaneID = %q", got.pendingPaneID)
	}
}

func TestNavigationSkipsEpicHeaders(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.issues = []model.Issue{
		{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-2", Title: "Task 2", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	}
	m.rows = buildIssueRows(m.issues)
	m.selected = firstSelectableRow(m.rows)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := next.(Model)
	issue, ok := selectedIssueFromRows(got.rows, got.selected)
	if !ok || issue.ID != "bd-2" {
		t.Fatalf("selected issue = %#v, ok=%v", issue, ok)
	}
}

func TestEnterOnHeaderDoesNotDispatch(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.issueSourceAvailable = true
	m.issues = []model.Issue{{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1}}
	m.rows = buildIssueRows(m.issues)
	m.selected = 0 // header row

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("expected no command dispatch from header row")
	}
	if got.busy {
		t.Fatalf("expected not busy")
	}
}

func TestSelectionPreservedAcrossRefreshRows(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-2", Title: "Task 2", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-2")

	next, _ := m.Update(issuesLoadedMsg{
		issues: []model.Issue{
			{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
			{ID: "bd-2", Title: "Task 2", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		},
		state: app.IssueSourceState{Available: true},
	})
	got := next.(Model)
	issue, ok := selectedIssueFromRows(got.rows, got.selected)
	if !ok || issue.ID != "bd-2" {
		t.Fatalf("selected issue = %#v, ok=%v", issue, ok)
	}
}

func TestEnterDoesNotRestartWhenBatchActive(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.issueSourceAvailable = true
	m.batchActive = true
	m.selectedTaskIssueIDs = map[string]struct{}{"bd-1": {}}
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = firstSelectableRow(m.rows)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("expected no command dispatch")
	}
	if got.status != "Batch launch in progress." {
		t.Fatalf("status = %q", got.status)
	}
}

func TestEnterBatchDispatchesImmediateMixedStart(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.issueSourceAvailable = true
	m.taskModeSelected = 1 // self-run
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-a", Title: "Task A", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-b", Title: "Task B", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.blockedBy = map[string]string{"bd-b": "bd-a"}
	m.selectedTaskIssueIDs = map[string]struct{}{"bd-a": {}, "bd-b": {}}
	m.selected = firstSelectableRow(m.rows)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd == nil {
		t.Fatalf("expected command dispatch")
	}
	if !got.batchActive {
		t.Fatalf("expected active batch")
	}
	if !got.busy {
		t.Fatalf("expected busy true")
	}
}

func TestBatchLaunchItemsMarksBlockedTasksAsWaiting(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-a", Title: "Task A", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-b", Title: "Task B", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.blockedBy = map[string]string{"bd-b": "bd-a"}
	ids := []string{"bd-a", "bd-b"}
	items := m.batchLaunchItems(ids)
	if len(items) != 2 {
		t.Fatalf("items len = %d", len(items))
	}
	if items[0].issueID != "bd-a" || items[0].blockerID != "" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].issueID != "bd-b" || items[1].blockerID != "bd-a" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
}

func TestBatchLaunchResultStatusCounts(t *testing.T) {
	t.Parallel()
	m := NewModel(&app.Service{})
	m.batchActive = true
	m.busy = true

	next, cmd := m.Update(batchLaunchResultMsg{
		mode:    model.RunModePlan,
		started: map[string]string{"bd-1": "%2"},
		waiting: map[string]string{"bd-2": "%3"},
		failed:  map[string]string{"bd-3": "task already running in pane %4"},
	})
	got := next.(Model)
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if got.batchActive {
		t.Fatalf("expected batch complete")
	}
	if !strings.Contains(got.status, "started=1 waiting=1 failed=1") {
		t.Fatalf("unexpected status: %q", got.status)
	}
}

func TestViewportScrollKeys(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-2", Title: "Task 2", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-3", Title: "Task 3", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-4", Title: "Task 4", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-5", Title: "Task 5", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-6", Title: "Task 6", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.taskViewportWidth = 56
	m.taskViewportHeight = 6

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got := next.(Model)
	if got.taskViewportYOffset == 0 {
		t.Fatalf("expected offset to increase on pgdown")
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyHome})
	got = next.(Model)
	if got.taskViewportYOffset != 0 {
		t.Fatalf("expected home to reset offset, got %d", got.taskViewportYOffset)
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnd})
	got = next.(Model)
	if got.taskViewportYOffset == 0 {
		t.Fatalf("expected end to jump to bottom")
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	got = next.(Model)
	if got.taskViewportYOffset >= got.maxViewportOffset() {
		t.Fatalf("expected pgup to move up from bottom")
	}
}

func TestViewportFollowsSelectedIssueOnNavigation(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-1", Title: "Task 1", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-2", Title: "Task 2", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-3", Title: "Task 3", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-4", Title: "Task 4", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-5", Title: "Task 5", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = firstSelectableRow(m.rows)
	m.taskViewportWidth = 56
	m.taskViewportHeight = 4
	m.refreshTaskViewport(true)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := next.(Model)
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = next.(Model)
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = next.(Model)

	if got.taskViewportYOffset == 0 {
		t.Fatalf("expected viewport to move down while navigating selection")
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

func assertErr(msg string) error { return testErr(msg) }

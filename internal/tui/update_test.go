package tui

import (
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

type testErr string

func (e testErr) Error() string { return string(e) }

func assertErr(msg string) error { return testErr(msg) }

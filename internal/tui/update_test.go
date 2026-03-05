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

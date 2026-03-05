package waitpane

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewContainsFramedStatusAndFields(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	m := model{
		opts:      Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"},
		startedAt: now.Add(-5 * time.Second),
		now:       now,
		spinIndex: 1,
	}
	out := m.View()
	if !strings.Contains(out, "Blocked Task") {
		t.Fatalf("missing title: %s", out)
	}
	if !strings.Contains(out, "Issue    : bd-1") {
		t.Fatalf("missing issue id: %s", out)
	}
	if !strings.Contains(out, "Blocked  : bd-0") {
		t.Fatalf("missing blocker id: %s", out)
	}
	if !strings.Contains(out, "Branch   : task/bd-1") {
		t.Fatalf("missing branch: %s", out)
	}
	if !strings.Contains(out, "[/] Waiting for blocker to close...") {
		t.Fatalf("missing spinner status: %s", out)
	}
	if !strings.Contains(out, "+") || !strings.Contains(out, "|") {
		t.Fatalf("expected framed panel: %s", out)
	}
}

func TestTickAdvancesSpinner(t *testing.T) {
	t.Parallel()
	start := time.Now().UTC()
	m := model{
		opts:      Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"},
		startedAt: start,
		now:       start,
		spinIndex: 0,
	}
	next, _ := m.Update(tickMsg(start.Add(200 * time.Millisecond)))
	got := next.(model)
	if got.spinIndex != 1 {
		t.Fatalf("spinIndex = %d, want 1", got.spinIndex)
	}
}

func TestQuitKeysExit(t *testing.T) {
	t.Parallel()
	m := model{opts: Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected quit command for q")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command for ctrl+c")
	}
}

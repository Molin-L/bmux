package waitpane

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewContainsDashboardStatusAndFields(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	m := model{
		opts:          Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"},
		startedAt:     now.Add(-5 * time.Second),
		now:           now,
		spinIndex:     1,
		blockerStatus: "open",
		lastCheckedAt: now.Add(-2 * time.Second),
		width:         80,
		height:        16,
	}
	out := m.View()
	if !strings.Contains(out, "Blocked Task") {
		t.Fatalf("missing title: %s", out)
	}
	if !strings.Contains(out, "Issue ID") || !strings.Contains(out, "bd-1") {
		t.Fatalf("missing issue id: %s", out)
	}
	if !strings.Contains(out, "Blocked By") || !strings.Contains(out, "bd-0") {
		t.Fatalf("missing blocker id: %s", out)
	}
	if !strings.Contains(out, "Branch") || !strings.Contains(out, "task/bd-1") {
		t.Fatalf("missing branch: %s", out)
	}
	if !strings.Contains(out, "Status") || !strings.Contains(out, "open") {
		t.Fatalf("missing blocker status section: %s", out)
	}
	if !strings.Contains(out, "Press q or Ctrl+C to close this waiting pane.") {
		t.Fatalf("missing help hint: %s", out)
	}
	if strings.Contains(out, "+---") {
		t.Fatalf("old ASCII panel frame still present: %s", out)
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

func TestClosedBlockerShowsPromotionHint(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	m := model{
		opts:          Options{IssueID: "bd-9", BlockedBy: "bd-8", Branch: "task/bd-9"},
		startedAt:     now.Add(-10 * time.Second),
		now:           now,
		blockerStatus: "closed",
		lastCheckedAt: now,
	}
	out := m.View()
	if !strings.Contains(out, "Blocker closed; waiting for bmux promotion") {
		t.Fatalf("missing promotion hint: %s", out)
	}
}

func TestBlockerStatusMsgUpdatesState(t *testing.T) {
	t.Parallel()
	m := model{
		opts:          Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"},
		blockerStatus: "unknown",
	}
	checkedAt := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	next, _ := m.Update(blockerStatusMsg{
		status:    "in_progress",
		checkedAt: checkedAt,
		err:       errors.New("check failed"),
	})
	got := next.(model)
	if got.blockerStatus != "in_progress" {
		t.Fatalf("blockerStatus = %q, want in_progress", got.blockerStatus)
	}
	if !got.lastCheckedAt.Equal(checkedAt) {
		t.Fatalf("lastCheckedAt = %v, want %v", got.lastCheckedAt, checkedAt)
	}
	if got.lastCheckErr == "" {
		t.Fatalf("expected lastCheckErr to be set")
	}
}

func TestParseBlockerStatusJSONSupportsObjectAndArray(t *testing.T) {
	t.Parallel()
	status, err := parseBlockerStatusJSON([]byte(`{"id":"bd-1","status":"open"}`))
	if err != nil {
		t.Fatalf("parse object: %v", err)
	}
	if status != "open" {
		t.Fatalf("status = %q, want open", status)
	}

	status, err = parseBlockerStatusJSON([]byte(`[{"id":"bd-2","status":"closed"}]`))
	if err != nil {
		t.Fatalf("parse array: %v", err)
	}
	if status != "closed" {
		t.Fatalf("status = %q, want closed", status)
	}
}

func TestParseBlockerStatusJSONRejectsMissingStatus(t *testing.T) {
	t.Parallel()
	_, err := parseBlockerStatusJSON([]byte(`{"id":"bd-1"}`))
	if err == nil {
		t.Fatalf("expected parse error for missing status")
	}
}

func TestFetchBlockerStatusCmdReturnsStatusMessage(t *testing.T) {
	t.Parallel()
	m := model{
		opts: Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"},
		fetchStatus: func(context.Context, string) (string, error) {
			return "closed", nil
		},
	}
	cmd := m.fetchBlockerStatusCmd()
	if cmd == nil {
		t.Fatalf("expected fetch command")
	}
	msg := cmd()
	statusMsg, ok := msg.(blockerStatusMsg)
	if !ok {
		t.Fatalf("msg type = %T, want blockerStatusMsg", msg)
	}
	if statusMsg.status != "closed" {
		t.Fatalf("status = %q, want closed", statusMsg.status)
	}
	if statusMsg.checkedAt.IsZero() {
		t.Fatalf("expected non-zero checkedAt")
	}
}

func TestWindowSizeMsgUpdatesModelSize(t *testing.T) {
	t.Parallel()
	m := model{opts: Options{IssueID: "bd-1", BlockedBy: "bd-0", Branch: "task/bd-1"}}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 91, Height: 27})
	got := next.(model)
	if got.width != 91 || got.height != 27 {
		t.Fatalf("size = %dx%d, want 91x27", got.width, got.height)
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

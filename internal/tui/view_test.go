package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
)

func TestViewRendersWrappedCardsWithTreeIndentAndBlockedBy(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.taskViewportWidth = 90
	m.taskViewportHeight = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-task", Title: "Task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-sub", Title: "Subtask", Status: "open", Priority: 2, EpicID: "bd-epic", EpicTitle: "Epic", ParentID: "bd-task", HierarchyDepth: 2},
		{ID: "bd-orphan", Title: "Orphan", Status: "open", Priority: 3, HierarchyDepth: 0},
	})
	m.blockedBy = map[string]string{"bd-task": "bd-parent"}
	m.selected = rowIndexByIssueID(m.rows, "bd-task")
	m.selectedTaskIssueID = "bd-task"

	out := stripANSI(m.View())
	if !strings.Contains(out, "Tasks (wrapped, no truncation)") {
		t.Fatalf("missing wrapped tasks header in view:\n%s", out)
	}
	if !strings.Contains(out, "Epic: Epic [bd-epic]") {
		t.Fatalf("missing epic header in view:\n%s", out)
	}
	if !strings.Contains(out, "Epic: No Epic") {
		t.Fatalf("missing no-epic header in view:\n%s", out)
	}
	if !strings.Contains(out, "▶● id=bd-task") {
		t.Fatalf("missing selected markers in view:\n%s", out)
	}
	if !strings.Contains(out, "blocked_by=bd-parent") {
		t.Fatalf("missing blocked-by value in view:\n%s", out)
	}
	if !strings.Contains(out, "run=-") {
		t.Fatalf("expected live-only run placeholder in view:\n%s", out)
	}
	if strings.Contains(out, "run=completed(") {
		t.Fatalf("run should never render completed state from local storage:\n%s", out)
	}
	if !strings.Contains(out, "title: └─ Task") {
		t.Fatalf("missing depth-1 tree indentation in view:\n%s", out)
	}
	if !strings.Contains(out, "title: │  └─ Subtask") {
		t.Fatalf("missing depth-2 tree indentation in view:\n%s", out)
	}
}

func TestViewNoTruncationForLongFields(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.taskViewportWidth = 42
	m.taskViewportHeight = 30
	m.rows = buildIssueRows([]model.Issue{
		{
			ID:             "bd-task",
			Title:          "VeryLongUnbrokenTitleToken_ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789_tail",
			Status:         "open",
			Priority:       1,
			EpicID:         "bd-epic",
			EpicTitle:      "Epic",
			HierarchyDepth: 1,
		},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-task")

	out := stripANSI(m.View())
	if strings.Contains(out, "…") {
		t.Fatalf("unexpected truncation ellipsis in view:\n%s", out)
	}
	if !strings.Contains(out, "0123456789_t") || !strings.Contains(out, "ail") {
		t.Fatalf("missing tail fragments of long title in wrapped view:\n%s", out)
	}
}

func TestViewRendersPlanConfirmIndentedHierarchy(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.mode = modePlanConfirm
	m.extractedPlan = app.PlanPayload{
		Goal: "g",
		Epic: app.PlanItem{Title: "Epic", Priority: 1},
		Task: app.PlanItem{Title: "Task", Priority: 2},
		Subtasks: []app.PlanItem{
			{Title: "Sub1", Priority: 2},
			{Title: "Sub2", Priority: 3},
		},
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "- Epic: Epic") {
		t.Fatalf("missing epic line in view:\n%s", out)
	}
	if !strings.Contains(out, "  - Task: Task") {
		t.Fatalf("missing task line in view:\n%s", out)
	}
	if !strings.Contains(out, "    - Subtask: Sub1") || !strings.Contains(out, "    - Subtask: Sub2") {
		t.Fatalf("missing subtask lines in view:\n%s", out)
	}
}

func TestViewRendersCurrentMode(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.taskModeSelected = 1
	m.selectedTaskIssueID = "bd-1"

	out := stripANSI(m.View())
	if !strings.Contains(out, "mode Self-run") {
		t.Fatalf("missing current mode:\n%s", out)
	}
	if !strings.Contains(out, "task bd-1") {
		t.Fatalf("missing selected task:\n%s", out)
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

package tui

import (
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
)

func TestViewRendersEpicGroupsAndIndentedIssueList(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-task", Title: "Task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-sub", Title: "Subtask", Status: "open", Priority: 2, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 2},
		{ID: "bd-orphan", Title: "Orphan", Status: "open", Priority: 3, HierarchyDepth: 0},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-task")

	out := m.View()
	if !strings.Contains(out, "Epic: Epic [bd-epic]") {
		t.Fatalf("missing epic header in view:\n%s", out)
	}
	if !strings.Contains(out, "Epic: No Epic") {
		t.Fatalf("missing no-epic header in view:\n%s", out)
	}
	if !strings.Contains(out, ">   - [bd-task]") {
		t.Fatalf("missing depth-1 indent for task in view:\n%s", out)
	}
	if !strings.Contains(out, "      - [bd-sub]") {
		t.Fatalf("missing depth-2 indent for subtask in view:\n%s", out)
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

	out := m.View()
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

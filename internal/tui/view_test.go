package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

func TestViewRendersTreeRowsWithSelectionAndBlockedBy(t *testing.T) {
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
	m.selectedTaskIssueIDs = map[string]struct{}{"bd-task": {}}

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
	if !strings.Contains(out, "▶● bd-task Task") {
		t.Fatalf("missing selected markers in view:\n%s", out)
	}
	if !strings.Contains(out, "id=bd-task | p=1 | status=open | run=- | blocked_by=bd-parent") {
		t.Fatalf("missing selected issue metadata in view:\n%s", out)
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
	if !strings.Contains(out, "▶● bd-task Task") {
		t.Fatalf("missing depth-1 tree indentation in view:\n%s", out)
	}
	if !strings.Contains(out, "○   bd-sub Subtask") {
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
	m.selectedTaskIssueIDs = map[string]struct{}{"bd-1": {}}

	out := stripANSI(m.View())
	if !strings.Contains(out, "mode Self-run") {
		t.Fatalf("missing current mode:\n%s", out)
	}
	if !strings.Contains(out, "task bd-1") {
		t.Fatalf("missing selected task:\n%s", out)
	}
}

func TestModeLabelUsesApeForApeMode(t *testing.T) {
	t.Parallel()
	if got, want := modeLabel(model.RunModeApe), "Ape"; got != want {
		t.Fatalf("modeLabel(ape) = %q, want %q", got, want)
	}
}

func TestViewRendersMultiSelectBadgeAndHelpText(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.selectedTaskIssueIDs = map[string]struct{}{
		"bd-1": {},
		"bd-2": {},
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "tasks 2 selected") {
		t.Fatalf("missing multi-select badge:\n%s", out)
	}
	if !strings.Contains(out, "Space select/deselect task(s)") {
		t.Fatalf("missing updated help text:\n%s", out)
	}
}

func TestViewRendersStatusDetailsAsCappedFooterBlock(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.status = "Merged bd-1 (task/bd-1 -> main)."
	m.statusDetails = []string{
		"Step 1: worktree merge: main -> task/bd-1 (abc123 -> def456) [ok]",
		"Step 2: repo merge: task/bd-1 -> main (111111 -> 222222) [ok]",
		"Step 3: close issue: bd-1 -> bd (222222 -> 222222) [ok]",
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "Status: Merged bd-1 (task/bd-1 -> main).") {
		t.Fatalf("missing status headline:\n%s", out)
	}
	if !strings.Contains(out, "Step 2: repo merge: task/bd-1 -> main") {
		t.Fatalf("missing detail lines:\n%s", out)
	}
	if strings.Contains(out, "Step 3: close issue: bd-1 -> bd") {
		t.Fatalf("expected footer detail lines to be capped at 2:\n%s", out)
	}
}

func TestViewPinsStatusFooterAtBottomAfterPromptPanel(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.width = 120
	m.height = 24
	m.status = "Loaded 3 issues"
	m.prompt = "Draft PR summary"

	out := stripANSI(m.View())
	statusIdx := strings.LastIndex(out, "Status: Loaded 3 issues")
	promptIdx := strings.LastIndex(out, "PR Prompt:")
	if statusIdx < 0 || promptIdx < 0 {
		t.Fatalf("missing status or prompt block:\n%s", out)
	}
	if statusIdx <= promptIdx {
		t.Fatalf("expected status block below prompt block:\n%s", out)
	}
	if !strings.Contains(out, "\n\nStatus: Loaded 3 issues") {
		t.Fatalf("expected one blank-line separation above status footer:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "Status: Loaded 3 issues") {
		t.Fatalf("expected status footer to be bottom-most block:\n%s", out)
	}
}

func TestViewRendersMergeConflictConfirmInline(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.mode = modeMergeConflictConfirm
	m.pendingMergeConflict = &pendingMergeConflict{
		issueID: "bd-9",
		runMode: model.RunModePlan,
		conflict: &app.MergeConflictError{
			Stage:        "Step 1",
			SourceBranch: "main",
			TargetBranch: "task/bd-9",
		},
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "Merge conflict for bd-9.") {
		t.Fatalf("missing conflict header:\n%s", out)
	}
	if !strings.Contains(out, "Enter=create conflict task") || !strings.Contains(out, "Esc=skip") {
		t.Fatalf("missing conflict actions:\n%s", out)
	}
}

func TestViewRendersClaimedHandoffConfirmInline(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.mode = modeClaimedHandoffConfirm
	m.pendingClaimedHandoff = &pendingClaimedHandoff{
		mode: model.RunModePlan,
		claimed: []claimedIssueInfo{
			{issueID: "bd-1", assignee: "Alice"},
			{issueID: "bd-2", assignee: "Bob"},
		},
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "WARNING: selected issues are already claimed.") {
		t.Fatalf("missing warning header:\n%s", out)
	}
	if !strings.Contains(out, "Enter=handoff and continue") || !strings.Contains(out, "Esc=cancel launch") {
		t.Fatalf("missing handoff actions:\n%s", out)
	}
	if !strings.Contains(out, "- bd-1 (assignee: Alice)") || !strings.Contains(out, "- bd-2 (assignee: Bob)") {
		t.Fatalf("missing claimed issue lines:\n%s", out)
	}
}

func TestViewRendersQuitConfirmInline(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.mode = modeQuitConfirm
	m.quitConfirmPanelCount = 2

	out := stripANSI(m.View())
	if !strings.Contains(out, "Quit bmux?") {
		t.Fatalf("missing quit confirm header:\n%s", out)
	}
	if !strings.Contains(out, "2 active bmux panels detected.") {
		t.Fatalf("missing panel count:\n%s", out)
	}
	if !strings.Contains(out, "Enter/q/Ctrl+C=confirm quit, Esc=cancel.") {
		t.Fatalf("missing quit confirm actions:\n%s", out)
	}
}

func TestViewRendersPendingRunWithBlockingInfo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC()
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:          "bd-task",
		Mode:             model.RunModePlan,
		Agent:            "codex",
		PaneID:           "%9",
		Pending:          true,
		BlockedByIssueID: "bd-blocker",
		WaitStartedAt:    now,
		StartedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	svc := app.NewService(app.Options{
		RepoRoot:    root,
		WorktreeDir: root + "/.worktrees",
		Store:       store,
	})

	m := NewModel(svc)
	m.taskViewportWidth = 120
	m.taskViewportHeight = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-task", Title: "Task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-task")
	out := stripANSI(m.View())
	if !strings.Contains(out, "run=waiting(plan)") {
		t.Fatalf("expected waiting run state in view:\n%s", out)
	}
	if !strings.Contains(out, "blocked_by=bd-blocker") {
		t.Fatalf("expected blocked_by from pending run in view:\n%s", out)
	}
}

func TestViewCompactModeHidesInlineMetadataAndShowsDetailsPanel(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.taskViewportWidth = 63
	m.taskViewportHeight = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-task", Title: "Task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-task")
	m.blockedBy = map[string]string{"bd-task": "bd-parent"}

	out := stripANSI(m.View())
	if strings.Contains(out, "id=bd-task | p=1 | status=open | run=-") {
		t.Fatalf("compact view should not render inline metadata rows:\n%s", out)
	}
	if !strings.Contains(out, "Selected Task") {
		t.Fatalf("compact view should render selected task details block:\n%s", out)
	}
	if !strings.Contains(out, "blocked_by=bd-parent") {
		t.Fatalf("details block should include blocked_by:\n%s", out)
	}
}

func TestViewCompactModePinsSelectedTaskDetailsAboveStatusFooter(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.width = 63
	m.height = 24
	m.taskViewportWidth = 63
	m.taskViewportHeight = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-task", Title: "Task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-task")
	m.status = "Loaded 1 issues"

	out := stripANSI(m.View())
	listIdx := strings.Index(out, "Tasks (wrapped, no truncation)")
	detailsIdx := strings.LastIndex(out, "Selected Task")
	statusIdx := strings.LastIndex(out, "Status: Loaded 1 issues")
	if listIdx < 0 || detailsIdx < 0 || statusIdx < 0 {
		t.Fatalf("missing expected blocks:\n%s", out)
	}
	if detailsIdx <= listIdx {
		t.Fatalf("selected task details should be below task list:\n%s", out)
	}
	if statusIdx <= detailsIdx {
		t.Fatalf("status footer should remain below selected task details:\n%s", out)
	}
}

func TestViewCompactModeShowsRunSymbols(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC()
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:   "bd-run",
		Mode:      model.RunModePlan,
		Agent:     "codex",
		PaneID:    "%2",
		Pending:   false,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed running run: %v", err)
	}
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:          "bd-pending",
		Mode:             model.RunModePlan,
		Agent:            "codex",
		PaneID:           "%3",
		Pending:          true,
		BlockedByIssueID: "bd-run",
		StartedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed pending run: %v", err)
	}
	svc := app.NewService(app.Options{
		RepoRoot:    root,
		WorktreeDir: root + "/.worktrees",
		Store:       store,
	})

	m := NewModel(svc)
	m.taskViewportWidth = 63
	m.taskViewportHeight = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-run", Title: "Running task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-pending", Title: "Pending task", Status: "open", Priority: 2, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-run")

	out := stripANSI(m.View())
	if !strings.Contains(out, "⏳") {
		t.Fatalf("compact view should show hourglass marker for pending run:\n%s", out)
	}
	if !strings.Contains(out, "⠋") {
		t.Fatalf("compact view should show spinner marker for running task:\n%s", out)
	}
}

func TestViewUsesVerboseModeAtThresholdWidth(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.taskViewportWidth = 64
	m.taskViewportHeight = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-task", Title: "Task", Status: "open", Priority: 1, EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-task")

	out := stripANSI(m.View())
	if !strings.Contains(out, "id=bd-task | p=1 | status=open") {
		t.Fatalf("width=64 should use verbose rendering:\n%s", out)
	}
}

func TestViewRendersStatusBucketsInFooterInExpectedOrder(t *testing.T) {
	t.Parallel()
	m := NewModel(nil)
	m.taskViewportWidth = 100
	m.taskViewportHeight = 12
	m.width = 100
	m.height = 24
	m.rows = buildIssueRows([]model.Issue{
		{ID: "bd-open", Title: "Open", Status: "open", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-closed", Title: "Closed", Status: "closed", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-running", Title: "Running", Status: "in_progress", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})
	m.selected = rowIndexByIssueID(m.rows, "bd-running")

	out := stripANSI(m.View())
	bucketsLine := "Task Statuses: in_progress | open | closed"
	bucketsIdx := strings.Index(out, bucketsLine)
	if bucketsIdx < 0 {
		t.Fatalf("missing footer task statuses line:\n%s", out)
	}
	if strings.Contains(out, "Status: in_progress") || strings.Contains(out, "Status: open") || strings.Contains(out, "Status: closed") {
		t.Fatalf("expected status headers removed from task list:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), bucketsLine) {
		t.Fatalf("expected footer task statuses to be bottom-most when no global status:\n%s", out)
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

package agentexec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

func newExecutorWithMetas(root string, beads *fakeBeads, tmux *fakeTmux, metas map[string]model.TaskBranchMeta) *Executor {
	if beads == nil {
		beads = &fakeBeads{}
	}
	if tmux == nil {
		tmux = &fakeTmux{paneID: "%2"}
	}
	return New(Options{
		RepoRoot:     root,
		Store:        state.New(root),
		Beads:        beads,
		Tmux:         tmux,
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			if meta, ok := metas[issueID]; ok {
				return meta, nil
			}
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc123",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{}, IssueSourceState{Available: true}, nil
		},
	})
}

func TestStartTaskModePlanUsesSafeFlags(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%8"}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-9", Title: "Plan task", Status: "open"}}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-9": {IssueID: "bd-9", Branch: "task/bd-9-plan-task", WorktreePath: root + "/.worktrees/task__bd-9-plan-task", BaseBranch: "main", BaseCommit: "abc123"},
	})

	run, err := e.StartTaskMode(context.Background(), "bd-9", model.RunModePlan)
	if err != nil {
		t.Fatalf("start task mode: %v", err)
	}
	if run.Mode != model.RunModePlan {
		t.Fatalf("unexpected run: %+v", run)
	}
	if len(tmux.sent) == 0 {
		t.Fatalf("expected launch command")
	}
	if len(beads.claimed) != 1 || beads.claimed[0] != "bd-9" {
		t.Fatalf("expected claim for bd-9, got %#v", beads.claimed)
	}
	if !strings.Contains(tmux.sent[0], "--sandbox workspace-write") || !strings.Contains(tmux.sent[0], "--ask-for-approval on-request") {
		t.Fatalf("expected plan safety flags in %q", tmux.sent[0])
	}
	if got, want := tmux.titles["%8"], "bd-9-plan-task"; got != want {
		t.Fatalf("pane title = %q, want %q", got, want)
	}
}

func TestStartTaskModeSelfRunUsesYoloFlag(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%9"}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-10", Title: "Self run", Status: "open"}}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-10": {IssueID: "bd-10", Branch: "task/bd-10-self-run", WorktreePath: root + "/.worktrees/task__bd-10-self-run", BaseBranch: "main", BaseCommit: "abc123"},
	})

	if _, err := e.StartTaskMode(context.Background(), "bd-10", model.RunModeSelfRun); err != nil {
		t.Fatalf("start task mode: %v", err)
	}
	if len(tmux.sent) == 0 {
		t.Fatalf("expected launch command")
	}
	if len(beads.claimed) != 1 || beads.claimed[0] != "bd-10" {
		t.Fatalf("expected claim for bd-10, got %#v", beads.claimed)
	}
	if !strings.Contains(tmux.sent[0], "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected yolo flag in %q", tmux.sent[0])
	}
}

func TestStartTaskModeApeClaimsIssue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%10"}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-ape", Title: "Ape run", Status: "open"}}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-ape": {IssueID: "bd-ape", Branch: "task/bd-ape-run", WorktreePath: root + "/.worktrees/task__bd-ape-run", BaseBranch: "main", BaseCommit: "abc123"},
	})

	if _, err := e.StartTaskMode(context.Background(), "bd-ape", model.RunModeApe); err != nil {
		t.Fatalf("start task mode: %v", err)
	}
	if len(beads.claimed) != 1 || beads.claimed[0] != "bd-ape" {
		t.Fatalf("expected claim for bd-ape, got %#v", beads.claimed)
	}
}

func TestStartTaskModeRejectsDuplicateRunningTask(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC()
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID: "bd-11", Mode: model.RunModeSelfRun, Agent: "codex",
		PaneID: "%5", StartedAt: now, UpdatedAt: now, ExpectedProcess: "codex",
	}); err != nil {
		t.Fatalf("seed run state: %v", err)
	}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-11", Title: "Dup"}}
	e := New(Options{
		RepoRoot:     root,
		Store:        store,
		Beads:        beads,
		Tmux:         &fakeTmux{paneID: "%5", panes: []string{"%1", "%5"}},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{IssueID: issueID, Branch: "task/bd-11-dup", WorktreePath: root + "/.worktrees/task__bd-11-dup", BaseBranch: "main", BaseCommit: "abc123"}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{}, IssueSourceState{Available: true}, nil
		},
	})

	_, err := e.StartTaskMode(context.Background(), "bd-11", model.RunModeSelfRun)
	if err == nil {
		t.Fatalf("expected duplicate lock error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already running in pane %5") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestStartTaskModeWaitingCreatesPendingRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%12"}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-wait", Title: "Wait task", Status: "open"}}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-wait": {IssueID: "bd-wait", Branch: "task/bd-wait-task", WorktreePath: root + "/.worktrees/task__bd-wait-task", BaseBranch: "main", BaseCommit: "abc123"},
	})

	run, err := e.StartTaskModeWaiting(context.Background(), "bd-wait", model.RunModeSelfRun, "bd-blocker")
	if err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	if !run.Pending || run.BlockedByIssueID != "bd-blocker" {
		t.Fatalf("unexpected waiting run: %+v", run)
	}
	if len(beads.claimed) != 0 {
		t.Fatalf("blocked task should not be claimed yet: %#v", beads.claimed)
	}
	if len(tmux.sent) == 0 {
		t.Fatalf("expected waiting command")
	}
	waitCmd := tmux.sent[0]
	if !strings.Contains(waitCmd, "--wait-blocked") ||
		!strings.Contains(waitCmd, "--issue-id") ||
		!strings.Contains(waitCmd, "bd-wait") ||
		!strings.Contains(waitCmd, "--blocked-by") ||
		!strings.Contains(waitCmd, "bd-blocker") ||
		!strings.Contains(waitCmd, "--branch") ||
		!strings.Contains(waitCmd, "task/bd-wait-task") {
		t.Fatalf("unexpected waiting command: %q", waitCmd)
	}
	if strings.Contains(waitCmd, "while true; do") {
		t.Fatalf("waiting command should not use raw shell spinner loop: %q", waitCmd)
	}
	if got, want := tmux.titles["%12"], "bd-wait-wait-task"; got != want {
		t.Fatalf("pane title = %q, want %q", got, want)
	}
}

func TestStartTaskModeFailsWhenSettingTitleOnNewPaneFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%18", titleErr: errors.New("title failed")}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-18", Title: "Title fail", Status: "open"}}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-18": {IssueID: "bd-18", Branch: "task/bd-18-title-fail", WorktreePath: root + "/.worktrees/task__bd-18-title-fail", BaseBranch: "main", BaseCommit: "abc123"},
	})

	_, err := e.StartTaskMode(context.Background(), "bd-18", model.RunModePlan)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "failed to set pane title") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tmux.killed) == 0 || tmux.killed[0] != "%18" {
		t.Fatalf("expected created pane to be killed, got %#v", tmux.killed)
	}
}

func TestStartTaskModeWaitingFailsWhenSettingTitleFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%19", titleErr: errors.New("title failed")}
	beads := &fakeBeads{issue: model.Issue{ID: "bd-19", Title: "Waiting title fail", Status: "open"}}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-19": {IssueID: "bd-19", Branch: "task/bd-19-waiting-title-fail", WorktreePath: root + "/.worktrees/task__bd-19-waiting-title-fail", BaseBranch: "main", BaseCommit: "abc123"},
	})

	_, err := e.StartTaskModeWaiting(context.Background(), "bd-19", model.RunModePlan, "bd-blocker")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "failed to set pane title") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tmux.killed) == 0 || tmux.killed[0] != "%19" {
		t.Fatalf("expected created pane to be killed, got %#v", tmux.killed)
	}
}

func TestPromotePendingRunsStartsTaskInSamePaneWhenBlockerClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%13"}
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-promote", Title: "Promote task", Status: "open"},
		showByID: map[string]model.Issue{
			"bd-blocker": {ID: "bd-blocker", Status: "closed"},
		},
	}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-promote": {IssueID: "bd-promote", Branch: "task/bd-promote-task", WorktreePath: root + "/.worktrees/task__bd-promote-task", BaseBranch: "main", BaseCommit: "abc123"},
	})

	if _, err := e.StartTaskModeWaiting(context.Background(), "bd-promote", model.RunModePlan, "bd-blocker"); err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	promoted, waiting, err := e.PromotePendingRuns(context.Background())
	if err != nil {
		t.Fatalf("promote pending runs: %v", err)
	}
	if promoted != 1 || waiting != 0 {
		t.Fatalf("promoted=%d waiting=%d", promoted, waiting)
	}
	if len(beads.claimed) != 1 || beads.claimed[0] != "bd-promote" {
		t.Fatalf("expected claim during promotion, got %#v", beads.claimed)
	}
	if len(tmux.sent) < 3 {
		t.Fatalf("expected waiting cmd + ctrl-c + launch, got %#v", tmux.sent)
	}
	if tmux.sent[1] != "C-c" {
		t.Fatalf("expected ctrl-c before launch, got %#v", tmux.sent)
	}
	if !strings.Contains(tmux.sent[2], " codex ") && !strings.Contains(tmux.sent[2], "; codex ") {
		t.Fatalf("expected codex launch in same pane, got %#v", tmux.sent)
	}

	runMeta, ok, err := e.store.RunLockByIssueID("bd-promote")
	if err != nil {
		t.Fatalf("get run meta: %v", err)
	}
	if !ok {
		t.Fatal("expected run metadata")
	}
	if runMeta.Pending || runMeta.BlockedByIssueID != "" || runMeta.ExpectedProcess != "codex" {
		t.Fatalf("unexpected promoted run meta: %+v", runMeta)
	}
}

func TestPromotePendingRunsFailsWhenSettingTitleOnReusedPaneFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%20"}
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-20", Title: "Promote title fail", Status: "open"},
		showByID: map[string]model.Issue{
			"bd-blocker": {ID: "bd-blocker", Status: "closed"},
		},
	}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-20": {IssueID: "bd-20", Branch: "task/bd-20-promote-title-fail", WorktreePath: root + "/.worktrees/task__bd-20-promote-title-fail", BaseBranch: "main", BaseCommit: "abc123"},
	})

	if _, err := e.StartTaskModeWaiting(context.Background(), "bd-20", model.RunModePlan, "bd-blocker"); err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	tmux.titleErr = errors.New("title failed")

	promoted, waiting, err := e.PromotePendingRuns(context.Background())
	if err == nil {
		t.Fatalf("expected promote error")
	}
	if promoted != 0 || waiting != 1 {
		t.Fatalf("promoted=%d waiting=%d", promoted, waiting)
	}
	if !strings.Contains(err.Error(), "failed to set pane title") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tmux.killed) != 0 {
		t.Fatalf("reused pane should not be killed: %#v", tmux.killed)
	}
}

func TestPromotePendingRunsKeepsWaitingWhenBlockerNotClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%14"}
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-still", Title: "Still waiting", Status: "open"},
		showByID: map[string]model.Issue{
			"bd-blocker": {ID: "bd-blocker", Status: "open"},
		},
	}
	e := newExecutorWithMetas(root, beads, tmux, map[string]model.TaskBranchMeta{
		"bd-still": {IssueID: "bd-still", Branch: "task/bd-still-waiting", WorktreePath: root + "/.worktrees/task__bd-still-waiting", BaseBranch: "main", BaseCommit: "abc123"},
	})

	if _, err := e.StartTaskModeWaiting(context.Background(), "bd-still", model.RunModePlan, "bd-blocker"); err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	promoted, waiting, err := e.PromotePendingRuns(context.Background())
	if err != nil {
		t.Fatalf("promote pending runs: %v", err)
	}
	if promoted != 0 || waiting != 1 {
		t.Fatalf("promoted=%d waiting=%d", promoted, waiting)
	}
	if len(beads.claimed) != 0 {
		t.Fatalf("should not claim while blocker still open, got %#v", beads.claimed)
	}
	if len(tmux.sent) != 1 {
		t.Fatalf("expected only waiting command, got %#v", tmux.sent)
	}
}

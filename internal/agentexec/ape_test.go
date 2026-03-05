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

func TestTickApeKeepsSessionActiveWhileRunLockStillExists(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC()

	if err := store.SetApeState(&model.ApeState{
		SessionID:        "ape-1",
		Active:           true,
		LaunchedIssueIDs: []string{"bd-42"},
		FinishedIssueIDs: []string{},
		ActiveIssueIDs:   []string{"bd-42"},
		StartedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed ape state: %v", err)
	}
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:         "bd-42",
		Mode:            model.RunModeApe,
		Agent:           "codex",
		PaneID:          "%42",
		StartedAt:       now,
		UpdatedAt:       now,
		ExpectedProcess: "codex",
		ApeSessionID:    "ape-1",
	}); err != nil {
		t.Fatalf("seed run lock: %v", err)
	}

	e := New(Options{
		RepoRoot:     root,
		Store:        store,
		Beads:        &fakeBeads{},
		Tmux:         &fakeTmux{paneID: "%42", panes: []string{"%42"}},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{IssueID: issueID, Branch: "task/" + issueID, WorktreePath: root + "/.worktrees/" + issueID, BaseBranch: "main", BaseCommit: "abc"}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{}, IssueSourceState{Available: true}, nil
		},
	})

	status, done, err := e.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "running=1") {
		t.Fatalf("expected running=1 in status, got %q", status)
	}

	ape, ok, err := store.ApeState()
	if err != nil {
		t.Fatalf("read ape state: %v", err)
	}
	if !ok {
		t.Fatal("expected ape state")
	}
	if !ape.Active {
		t.Fatalf("ape active = false, want true: %+v", ape)
	}
	if len(ape.ActiveIssueIDs) != 1 || ape.ActiveIssueIDs[0] != "bd-42" {
		t.Fatalf("active issues = %#v, want [\"bd-42\"]", ape.ActiveIssueIDs)
	}
}

func TestTickApeUsesBlocksDependencyFromTargetID(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		depsByIssue: map[string][]model.Dependency{
			"bd-201": {{Type: "blocks", TargetID: "bd-202"}},
			"bd-202": []model.Dependency{},
		},
	}

	e := New(Options{
		RepoRoot:     root,
		Store:        store,
		Beads:        beads,
		Tmux:         &fakeTmux{paneID: "%201", panes: []string{"%201"}},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{
				{ID: "bd-201", Title: "Blocked", Status: "open"},
				{ID: "bd-202", Title: "Blocker", Status: "open"},
			}, IssueSourceState{Available: true}, nil
		},
	})

	if _, err := e.StartApe(context.Background()); err != nil {
		t.Fatalf("start ape: %v", err)
	}
	status, done, err := e.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "blocked=1") {
		t.Fatalf("expected blocked=1 in status, got %q", status)
	}
	if len(beads.claimed) != 1 || beads.claimed[0] != "bd-202" {
		t.Fatalf("expected only blocker claimed, got %#v", beads.claimed)
	}
}

func TestTickApeIgnoresParentChildForBlocking(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		depsByIssue: map[string][]model.Dependency{
			"bd-parent": []model.Dependency{},
			"bd-child":  []model.Dependency{},
		},
	}

	e := New(Options{
		RepoRoot:     root,
		Store:        store,
		Beads:        beads,
		Tmux:         &fakeTmux{paneID: "%301", panes: []string{"%301"}},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{
				{ID: "bd-parent", Title: "Parent", Status: "open"},
				{ID: "bd-child", Title: "Child", Status: "open", ParentID: "bd-parent"},
			}, IssueSourceState{Available: true}, nil
		},
	})

	if _, err := e.StartApe(context.Background()); err != nil {
		t.Fatalf("start ape: %v", err)
	}
	status, done, err := e.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "blocked=0") {
		t.Fatalf("expected blocked=0 in status, got %q", status)
	}
	if len(beads.claimed) != 2 {
		t.Fatalf("expected both parent and child claimed, got %#v", beads.claimed)
	}
}

func TestTickApeKeepsIssuePendingWhenLaunchFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-fail", Title: "Launch fail", Status: "open"},
	}
	e := New(Options{
		RepoRoot: root,
		Store:    store,
		Beads:    beads,
		Tmux: &fakeTmux{
			splitErr: errors.New("split failed"),
			panes:    []string{"%1"},
		},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{{ID: "bd-fail", Title: "Launch fail", Status: "open"}}, IssueSourceState{Available: true}, nil
		},
	})

	if _, err := e.StartApe(context.Background()); err != nil {
		t.Fatalf("start ape: %v", err)
	}
	status, done, err := e.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "finished=0") {
		t.Fatalf("expected finished=0 in status, got %q", status)
	}
	if len(beads.claimed) != 0 {
		t.Fatalf("should not claim issue when pane launch fails, got %#v", beads.claimed)
	}

	ape, ok, err := store.ApeState()
	if err != nil {
		t.Fatalf("read ape state: %v", err)
	}
	if !ok || !ape.Active {
		t.Fatalf("ape state = %+v, want active", ape)
	}
	if len(ape.FinishedIssueIDs) != 0 {
		t.Fatalf("finished issues = %#v, want empty", ape.FinishedIssueIDs)
	}
}

func TestTickApeDoesNotClaimWhenCommandSendFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-fail-send", Title: "Launch fail send", Status: "open"},
	}
	e := New(Options{
		RepoRoot: root,
		Store:    store,
		Beads:    beads,
		Tmux: &fakeTmux{
			paneID:  "%51",
			sendErr: errors.New("send failed"),
			panes:   []string{"%1"},
		},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{{ID: "bd-fail-send", Title: "Launch fail send", Status: "open"}}, IssueSourceState{Available: true}, nil
		},
	})

	if _, err := e.StartApe(context.Background()); err != nil {
		t.Fatalf("start ape: %v", err)
	}
	status, done, err := e.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "finished=0") {
		t.Fatalf("expected finished=0 in status, got %q", status)
	}
	if len(beads.claimed) != 0 {
		t.Fatalf("should not claim issue when command send fails, got %#v", beads.claimed)
	}
}

func TestTickApeReportsLaunchFailureInStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-fail", Title: "Launch fail", Status: "open"},
	}
	e := New(Options{
		RepoRoot: root,
		Store:    store,
		Beads:    beads,
		Tmux: &fakeTmux{
			splitErr: errors.New("split failed"),
			panes:    []string{"%1"},
		},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{{ID: "bd-fail", Title: "Launch fail", Status: "open"}}, IssueSourceState{Available: true}, nil
		},
	})

	if _, err := e.StartApe(context.Background()); err != nil {
		t.Fatalf("start ape: %v", err)
	}
	status, done, err := e.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "launch_failed=1") {
		t.Fatalf("expected launch_failed=1 in status, got %q", status)
	}
	if !strings.Contains(status, "bd-fail") {
		t.Fatalf("expected issue id in launch failure status, got %q", status)
	}
}

func TestReconcileRunLocksKeepsFreshProcessMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC()
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:         "bd-fresh",
		Mode:            model.RunModeApe,
		Agent:           "codex",
		PaneID:          "%77",
		StartedAt:       now,
		UpdatedAt:       now,
		ExpectedProcess: "codex",
		ApeSessionID:    "ape-1",
	}); err != nil {
		t.Fatalf("seed run lock: %v", err)
	}

	e := New(Options{
		RepoRoot: root,
		Store:    store,
		Beads:    &fakeBeads{},
		Tmux: &fakeTmux{
			panes:          []string{"%77"},
			currentByPane:  map[string]string{"%77": "zsh"},
			currentCommand: "zsh",
		},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{}, IssueSourceState{Available: true}, nil
		},
	})

	if err := e.ReconcileRunLocks(context.Background()); err != nil {
		t.Fatalf("reconcile run locks: %v", err)
	}
	if _, ok, err := store.RunLockByIssueID("bd-fresh"); err != nil {
		t.Fatalf("lookup run lock: %v", err)
	} else if !ok {
		t.Fatal("fresh mismatch run lock should be retained during startup grace window")
	}
}

func TestReconcileRunLocksRemovesStaleProcessMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	started := time.Now().UTC().Add(-30 * time.Second)
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:         "bd-stale",
		Mode:            model.RunModeApe,
		Agent:           "codex",
		PaneID:          "%78",
		StartedAt:       started,
		UpdatedAt:       started,
		ExpectedProcess: "codex",
		ApeSessionID:    "ape-1",
	}); err != nil {
		t.Fatalf("seed run lock: %v", err)
	}

	e := New(Options{
		RepoRoot: root,
		Store:    store,
		Beads:    &fakeBeads{},
		Tmux: &fakeTmux{
			panes:          []string{"%78"},
			currentByPane:  map[string]string{"%78": "zsh"},
			currentCommand: "zsh",
		},
		CodexCommand: "codex",
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
			return model.TaskBranchMeta{
				IssueID:      issueID,
				Branch:       "task/" + issueID,
				WorktreePath: root + "/.worktrees/" + issueID,
				BaseBranch:   "main",
				BaseCommit:   "abc",
			}, nil
		},
		ReadyIssues: func(context.Context) ([]model.Issue, IssueSourceState, error) {
			return []model.Issue{}, IssueSourceState{Available: true}, nil
		},
	})

	if err := e.ReconcileRunLocks(context.Background()); err != nil {
		t.Fatalf("reconcile run locks: %v", err)
	}
	if _, ok, err := store.RunLockByIssueID("bd-stale"); err != nil {
		t.Fatalf("lookup run lock: %v", err)
	} else if ok {
		t.Fatal("stale mismatch run lock should be removed after startup grace window")
	}
}

package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type fakeBeads struct {
	issue      model.Issue
	deps       []model.Dependency
	metaWrites map[string]map[string]string
}

func (f *fakeBeads) Ready(context.Context) ([]model.Issue, error) { return []model.Issue{f.issue}, nil }
func (f *fakeBeads) List(context.Context, map[string]string) ([]model.Issue, error) {
	return []model.Issue{f.issue}, nil
}
func (f *fakeBeads) Show(context.Context, string) (model.Issue, error) {
	return f.issue, nil
}
func (f *fakeBeads) Dependencies(context.Context, string) ([]model.Dependency, error) {
	return f.deps, nil
}
func (f *fakeBeads) UpdateMetadata(_ context.Context, issueID string, metadata map[string]string) error {
	if f.metaWrites == nil {
		f.metaWrites = map[string]map[string]string{}
	}
	f.metaWrites[issueID] = metadata
	return nil
}
func (f *fakeBeads) Close(context.Context, string, string) error { return nil }

type fakeGit struct {
	headBranch    string
	headCommit    string
	addedPath     string
	addedBranch   string
	addedStart    string
	merged        [][2]string
	removedPath   string
	deletedBranch string
}

func (g *fakeGit) CurrentHead(context.Context, string) (string, string, error) {
	return g.headBranch, g.headCommit, nil
}
func (g *fakeGit) AddWorktree(_ context.Context, _, path, branch, startRef string) error {
	g.addedPath = path
	g.addedBranch = branch
	g.addedStart = startRef
	return nil
}
func (g *fakeGit) RemoveWorktree(context.Context, string, string) error { return nil }
func (g *fakeGit) Merge(_ context.Context, repoPath, branch string) error {
	g.merged = append(g.merged, [2]string{repoPath, branch})
	return nil
}
func (g *fakeGit) DeleteBranch(context.Context, string, string) error { return nil }
func (g *fakeGit) Push(context.Context, string, string) error         { return nil }

type fakePlanner struct{ d app.BranchDecision }

func (p fakePlanner) ResolveBranch(model.Issue, []model.TaskBranchMeta) app.BranchDecision {
	return p.d
}

type fakePromptBuilder struct{ out string }

func (b fakePromptBuilder) Build(model.Issue, model.TaskBranchMeta) string { return b.out }

func TestOpenTaskCreatesNewWorktreeAndStoresMetadata(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)

	beads := &fakeBeads{issue: model.Issue{ID: "bd-1", Title: "Auth flow"}}
	git := &fakeGit{headBranch: "main", headCommit: "abc123"}
	planner := fakePlanner{d: app.BranchDecision{Branch: "task/bd-1-auth-flow"}}

	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         store,
		Beads:         beads,
		Git:           git,
		Planner:       planner,
		PromptBuilder: fakePromptBuilder{out: "prompt"},
	})

	meta, err := svc.OpenTask(context.Background(), "bd-1")
	if err != nil {
		t.Fatalf("open task: %v", err)
	}
	if meta.BaseBranch != "main" || meta.BaseCommit != "abc123" {
		t.Fatalf("unexpected base capture: %+v", meta)
	}
	if git.addedStart != "abc123" {
		t.Fatalf("worktree start ref = %q, want %q", git.addedStart, "abc123")
	}
	if meta.Branch != "task/bd-1-auth-flow" {
		t.Fatalf("branch = %q", meta.Branch)
	}
}

func TestOpenTaskReusesExistingBranch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	_ = store.Upsert(model.TaskBranchMeta{
		IssueID:      "bd-parent",
		Branch:       "task/bd-parent-auth",
		WorktreePath: root + "/.worktrees/task__bd-parent-auth",
		BaseBranch:   "main",
		BaseCommit:   "abc",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	})

	beads := &fakeBeads{issue: model.Issue{ID: "bd-child", Title: "Child task"}}
	git := &fakeGit{headBranch: "main", headCommit: "abc123"}
	planner := fakePlanner{d: app.BranchDecision{Branch: "task/bd-parent-auth", ReuseExisting: true, SourceIssueID: "bd-parent"}}

	svc := app.NewService(app.Options{RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store, Beads: beads, Git: git, Planner: planner, PromptBuilder: fakePromptBuilder{out: "prompt"}})
	meta, err := svc.OpenTask(context.Background(), "bd-child")
	if err != nil {
		t.Fatalf("open task: %v", err)
	}
	if meta.Branch != "task/bd-parent-auth" {
		t.Fatalf("branch = %q", meta.Branch)
	}
	if git.addedBranch != "" {
		t.Fatalf("expected no new worktree, got %q", git.addedBranch)
	}
}

func TestGeneratePRPromptUsesStoredMeta(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	_ = store.Upsert(model.TaskBranchMeta{IssueID: "bd-1", Branch: "task/bd-1-auth", BaseBranch: "release/x", BaseCommit: "abc", WorktreePath: root + "/.worktrees/task__bd-1-auth", Status: "active", CreatedAt: time.Now().UTC()})

	beads := &fakeBeads{issue: model.Issue{ID: "bd-1", Title: "Auth"}}
	svc := app.NewService(app.Options{RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store, Beads: beads, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{out: "TARGET release/x"}})

	prompt, err := svc.GeneratePRPrompt(context.Background(), "bd-1")
	if err != nil {
		t.Fatalf("generate prompt: %v", err)
	}
	if prompt != "TARGET release/x" {
		t.Fatalf("prompt = %q", prompt)
	}
}

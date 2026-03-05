package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/errorsx"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type fakeBeads struct {
	issue       model.Issue
	ready       []model.Issue
	readyErr    error
	deps        []model.Dependency
	showByID    map[string]model.Issue
	showErrByID map[string]error
	showCalls   []string
	metaWrites  map[string]map[string]string
	created     []model.CreateIssueRequest
	createOut   []model.Issue
	createErrAt int
}

func (f *fakeBeads) Ready(context.Context) ([]model.Issue, error) {
	if f.readyErr != nil {
		return nil, f.readyErr
	}
	if f.ready != nil {
		return f.ready, nil
	}
	return []model.Issue{f.issue}, nil
}
func (f *fakeBeads) List(context.Context, map[string]string) ([]model.Issue, error) {
	return []model.Issue{f.issue}, nil
}
func (f *fakeBeads) Show(_ context.Context, issueID string) (model.Issue, error) {
	f.showCalls = append(f.showCalls, issueID)
	if err, ok := f.showErrByID[issueID]; ok {
		return model.Issue{}, err
	}
	if issue, ok := f.showByID[issueID]; ok {
		return issue, nil
	}
	if f.issue.ID == issueID {
		return f.issue, nil
	}
	if f.issue.ID == "" {
		out := f.issue
		out.ID = issueID
		return out, nil
	}
	return model.Issue{ID: issueID}, nil
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
func (f *fakeBeads) CreateIssue(_ context.Context, req model.CreateIssueRequest) (model.Issue, error) {
	f.created = append(f.created, req)
	if f.createErrAt > 0 && len(f.created) == f.createErrAt {
		return model.Issue{}, errors.New("create failed")
	}
	if len(f.createOut) >= len(f.created) {
		return f.createOut[len(f.created)-1], nil
	}
	return model.Issue{ID: "bd-created"}, nil
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

type fakeTmux struct {
	paneID     string
	captured   string
	splitErr   error
	sendErr    error
	captureErr error
	bufferErr  error
	sent       []string
	splitDir   string
	splitCWD   string
	pasted     []string
}

func (t *fakeTmux) SplitPane(_ context.Context, direction, cwd string) (string, error) {
	t.splitDir = direction
	t.splitCWD = cwd
	if t.splitErr != nil {
		return "", t.splitErr
	}
	if t.paneID == "" {
		t.paneID = "%2"
	}
	return t.paneID, nil
}

func (t *fakeTmux) SplitPaneOnTarget(_ context.Context, direction, cwd, _ string) (string, error) {
	return t.SplitPane(context.Background(), direction, cwd)
}

func (t *fakeTmux) SendKeys(_ context.Context, _ string, text string, _ bool) error {
	if t.sendErr != nil {
		return t.sendErr
	}
	t.sent = append(t.sent, text)
	return nil
}

func (t *fakeTmux) CapturePane(context.Context, string, int) (string, error) {
	if t.captureErr != nil {
		return "", t.captureErr
	}
	return t.captured, nil
}

func (t *fakeTmux) CurrentPaneID(context.Context) (string, error) { return "%1", nil }
func (t *fakeTmux) ListPanes(context.Context, string) ([]string, error) {
	if strings.TrimSpace(t.paneID) != "" {
		return []string{"%1", t.paneID}, nil
	}
	return []string{"%1", "%2"}, nil
}
func (t *fakeTmux) SetWindowOptionsForSidebar(context.Context, string, int) error { return nil }
func (t *fakeTmux) SelectLayoutMainVertical(context.Context, string) error        { return nil }
func (t *fakeTmux) SetBuffer(_ context.Context, _ string, content string) error {
	if t.bufferErr != nil {
		return t.bufferErr
	}
	t.pasted = append(t.pasted, content)
	return nil
}
func (t *fakeTmux) PasteBuffer(context.Context, string, string) error { return nil }
func (t *fakeTmux) DeleteBuffer(context.Context, string) error        { return nil }
func (t *fakeTmux) GetPaneCurrentCommand(context.Context, string) (string, error) {
	return "codex", nil
}

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

func TestReadyIssuesStateEmptyIsAvailable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         &fakeBeads{ready: []model.Issue{}},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ReadyIssuesState(context.Background())
	if err != nil {
		t.Fatalf("ready issues state: %v", err)
	}
	if !source.Available {
		t.Fatalf("expected source available")
	}
	if len(issues) != 0 {
		t.Fatalf("issues len = %d, want 0", len(issues))
	}
}

func TestReadyIssuesStateBDUnavailable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := app.NewService(app.Options{
		RepoRoot:    root,
		WorktreeDir: root + "/.worktrees",
		Store:       state.New(root),
		Beads: &fakeBeads{readyErr: &errorsx.CommandError{
			Command:  "bd",
			Args:     []string{"ready", "--json"},
			Dir:      root,
			StdErr:   "command not found: bd",
			ExitCode: 127,
			Err:      errors.New("exit status 127"),
		}},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ReadyIssuesState(context.Background())
	if err != nil {
		t.Fatalf("ready issues state: %v", err)
	}
	if source.Available {
		t.Fatalf("expected source unavailable")
	}
	if source.Reason == "" {
		t.Fatalf("expected unavailable reason")
	}
	if len(issues) != 0 {
		t.Fatalf("issues len = %d, want 0", len(issues))
	}
}

func TestReadyIssuesStateUnexpectedError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	expected := errors.New("parse failure")
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         &fakeBeads{readyErr: expected},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ReadyIssuesState(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v, want %v", err, expected)
	}
	if !source.Available {
		t.Fatalf("expected source to remain available for unexpected errors")
	}
	if issues != nil {
		t.Fatalf("issues = %#v, want nil", issues)
	}
}

func TestReadyIssuesStateResolvesEpicAndDepthFromReadyData(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := app.NewService(app.Options{
		RepoRoot:    root,
		WorktreeDir: root + "/.worktrees",
		Store:       state.New(root),
		Beads: &fakeBeads{ready: []model.Issue{
			{ID: "bd-task", Title: "Task", IssueType: "task", ParentID: "bd-epic"},
			{ID: "bd-epic", Title: "Epic", IssueType: "epic"},
			{ID: "bd-sub", Title: "Subtask", IssueType: "task", ParentID: "bd-task"},
		}},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ReadyIssuesState(context.Background())
	if err != nil {
		t.Fatalf("ready issues state: %v", err)
	}
	if !source.Available {
		t.Fatalf("expected source available")
	}
	if issues[0].EpicID != "bd-epic" || issues[0].HierarchyDepth != 1 {
		t.Fatalf("issue[0] hierarchy = (%q,%d)", issues[0].EpicID, issues[0].HierarchyDepth)
	}
	if issues[1].EpicID != "bd-epic" || issues[1].HierarchyDepth != 0 {
		t.Fatalf("issue[1] hierarchy = (%q,%d)", issues[1].EpicID, issues[1].HierarchyDepth)
	}
	if issues[2].EpicID != "bd-epic" || issues[2].HierarchyDepth != 2 {
		t.Fatalf("issue[2] hierarchy = (%q,%d)", issues[2].EpicID, issues[2].HierarchyDepth)
	}
}

func TestReadyIssuesStateFetchesMissingParentChain(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		ready: []model.Issue{
			{ID: "bd-sub", Title: "Subtask", IssueType: "task", ParentID: "bd-task"},
		},
		showByID: map[string]model.Issue{
			"bd-task": {ID: "bd-task", Title: "Task", IssueType: "task", ParentID: "bd-epic"},
			"bd-epic": {ID: "bd-epic", Title: "Epic", IssueType: "epic"},
		},
	}
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         beads,
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, _, err := svc.ReadyIssuesState(context.Background())
	if err != nil {
		t.Fatalf("ready issues state: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues len = %d", len(issues))
	}
	if issues[0].EpicID != "bd-epic" || issues[0].HierarchyDepth != 2 {
		t.Fatalf("hierarchy = (%q,%d)", issues[0].EpicID, issues[0].HierarchyDepth)
	}
	if len(beads.showCalls) != 2 || beads.showCalls[0] != "bd-task" || beads.showCalls[1] != "bd-epic" {
		t.Fatalf("show calls = %#v", beads.showCalls)
	}
}

func TestReadyIssuesStateGracefullyFallsBackWhenParentFetchFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		ready: []model.Issue{
			{ID: "bd-child", Title: "Child", IssueType: "task", ParentID: "bd-parent"},
		},
		showErrByID: map[string]error{
			"bd-parent": errors.New("boom"),
		},
	}
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         beads,
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ReadyIssuesState(context.Background())
	if err != nil {
		t.Fatalf("ready issues state: %v", err)
	}
	if !source.Available {
		t.Fatalf("expected available source")
	}
	if issues[0].EpicID != "" {
		t.Fatalf("epic id = %q, want empty", issues[0].EpicID)
	}
	if issues[0].HierarchyDepth != 1 {
		t.Fatalf("depth = %d, want 1", issues[0].HierarchyDepth)
	}
}

func TestReadyIssuesStateHandlesParentCycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		ready: []model.Issue{
			{ID: "bd-a", Title: "A", IssueType: "task", ParentID: "bd-b"},
		},
		showByID: map[string]model.Issue{
			"bd-b": {ID: "bd-b", Title: "B", IssueType: "task", ParentID: "bd-a"},
		},
	}
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         beads,
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, _, err := svc.ReadyIssuesState(context.Background())
	if err != nil {
		t.Fatalf("ready issues state: %v", err)
	}
	if issues[0].EpicID != "" {
		t.Fatalf("epic id = %q, want empty", issues[0].EpicID)
	}
	if issues[0].HierarchyDepth != 2 {
		t.Fatalf("depth = %d, want 2", issues[0].HierarchyDepth)
	}
}

func TestCreateHierarchyFromPlanHappyPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		createOut: []model.Issue{{ID: "bd-epic"}, {ID: "bd-task"}, {ID: "bd-sub1"}, {ID: "bd-sub2"}},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	plan := app.PlanPayload{
		Goal:     "goal",
		Epic:     app.PlanItem{Title: "Epic", Priority: 1},
		Task:     app.PlanItem{Title: "Task", Priority: 2},
		Subtasks: []app.PlanItem{{Title: "Sub1", Priority: 2}, {Title: "Sub2", Priority: 3}},
	}
	res, err := svc.CreateHierarchyFromPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("create hierarchy: %v", err)
	}
	if res.EpicID != "bd-epic" || res.TaskID != "bd-task" || len(res.SubtaskIDs) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if beads.created[1].ParentID != "bd-epic" {
		t.Fatalf("task parent = %q", beads.created[1].ParentID)
	}
	if beads.created[2].ParentID != "bd-task" {
		t.Fatalf("subtask parent = %q", beads.created[2].ParentID)
	}
}

func TestCreateHierarchyFromPlanStopsOnFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		createOut:   []model.Issue{{ID: "bd-epic"}},
		createErrAt: 2,
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})
	plan := app.PlanPayload{
		Goal: "goal", Epic: app.PlanItem{Title: "Epic", Priority: 1},
		Task:     app.PlanItem{Title: "Task", Priority: 2},
		Subtasks: []app.PlanItem{{Title: "Sub1", Priority: 2}},
	}
	res, err := svc.CreateHierarchyFromPlan(context.Background(), plan)
	if err == nil {
		t.Fatalf("expected error")
	}
	if res.EpicID != "bd-epic" || res.TaskID != "" {
		t.Fatalf("unexpected partial result: %+v", res)
	}
}

func TestCreateHierarchyFromPlanRejectsInvalidPayload(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})
	_, err := svc.CreateHierarchyFromPlan(context.Background(), app.PlanPayload{
		Goal: "g",
		Epic: app.PlanItem{Title: "E", Priority: 1},
		Task: app.PlanItem{Title: "T", Priority: 2},
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if len(beads.created) != 0 {
		t.Fatalf("expected no create calls")
	}
}

func TestExtractPlanJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{captured: "x\nBMUX_PLAN_JSON_BEGIN\n{\"goal\":\"g\",\"epic\":{\"title\":\"E\",\"description\":\"\",\"priority\":1},\"task\":{\"title\":\"T\",\"description\":\"\",\"priority\":2},\"subtasks\":[{\"title\":\"S\",\"description\":\"\",\"priority\":2}]}\nBMUX_PLAN_JSON_END\n"}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: &fakeBeads{}, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{}, Tmux: tmux,
	})
	plan, err := svc.ExtractPlanJSON(context.Background(), "%2")
	if err != nil {
		t.Fatalf("extract plan: %v", err)
	}
	if plan.Epic.Title != "E" || len(plan.Subtasks) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestStartPlanningPane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%3"}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: &fakeBeads{}, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
		Tmux: tmux, SplitDirection: "below", ClaudeCommand: "claude --plan",
	})
	paneID, _, err := svc.StartPlanningPane(context.Background(), "claude")
	if err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if paneID != "%3" {
		t.Fatalf("pane id = %q", paneID)
	}
	if tmux.splitDir != "below" {
		t.Fatalf("split dir = %q", tmux.splitDir)
	}
	if len(tmux.sent) != 2 {
		t.Fatalf("send calls = %d", len(tmux.sent))
	}
}

func TestStartPlanningPaneCodexAutoTrustAndPromptRetry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		paneID:   "%7",
		captured: "Do you trust this workspace?",
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: &fakeBeads{}, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
		Tmux: tmux, CodexCommand: "codex", TmuxLayout: "sidebar", ControlWidth: 40,
	})
	paneID, status, err := svc.StartPlanningPane(context.Background(), "codex")
	if err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if paneID != "%7" {
		t.Fatalf("pane id = %q", paneID)
	}
	if !strings.Contains(status, "Trust prompt detected") {
		t.Fatalf("status missing trust handling: %q", status)
	}
	if len(tmux.pasted) != 0 {
		t.Fatalf("expected no tmux buffer paste prompt transport, got %#v", tmux.pasted)
	}
	if len(tmux.sent) < 2 || !strings.HasPrefix(tmux.sent[0], "codex ") {
		t.Fatalf("unexpected sent keys: %#v", tmux.sent)
	}
	if !strings.Contains(tmux.sent[0], "--sandbox workspace-write") || !strings.Contains(tmux.sent[0], "--ask-for-approval on-request") {
		t.Fatalf("missing default planning flags: %q", tmux.sent[0])
	}
	if !strings.Contains(tmux.sent[0], "create bd issues directly") {
		t.Fatalf("expected direct bd-creation prompt in codex launch command: %q", tmux.sent[0])
	}
}

func TestStartTaskModePlanUsesSafeFlags(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%8"}
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-9", Title: "Plan task", Status: "open"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-9-plan-task"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	run, err := svc.StartTaskMode(context.Background(), "bd-9", model.RunModePlan)
	if err != nil {
		t.Fatalf("start task mode: %v", err)
	}
	if run.Mode != model.RunModePlan {
		t.Fatalf("unexpected run: %+v", run)
	}
	if len(tmux.sent) == 0 {
		t.Fatalf("expected launch command")
	}
	if !strings.Contains(tmux.sent[0], "--sandbox workspace-write") || !strings.Contains(tmux.sent[0], "--ask-for-approval on-request") {
		t.Fatalf("expected plan safety flags in %q", tmux.sent[0])
	}
}

func TestStartTaskModeSelfRunUsesYoloFlag(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%9"}
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-10", Title: "Self run", Status: "open"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-10-self-run"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	if _, err := svc.StartTaskMode(context.Background(), "bd-10", model.RunModeSelfRun); err != nil {
		t.Fatalf("start task mode: %v", err)
	}
	if len(tmux.sent) == 0 {
		t.Fatalf("expected launch command")
	}
	if !strings.Contains(tmux.sent[0], "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected yolo flag in %q", tmux.sent[0])
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

	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads:         &fakeBeads{issue: model.Issue{ID: "bd-11", Title: "Dup"}},
		Git:           &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-11-dup"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: &fakeTmux{paneID: "%5"}, CodexCommand: "codex",
	})

	_, err := svc.StartTaskMode(context.Background(), "bd-11", model.RunModeSelfRun)
	if err == nil {
		t.Fatalf("expected duplicate lock error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already running in pane %5") {
		t.Fatalf("unexpected err: %v", err)
	}
}

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
	list        []model.Issue
	listErr     error
	listFilters map[string]string
	deps        []model.Dependency
	showByID    map[string]model.Issue
	showErrByID map[string]error
	showCalls   []string
	metaWrites  map[string]map[string]string
	claimed     []string
	closed      []closeCall
	closeErr    error
	created     []model.CreateIssueRequest
	createOut   []model.Issue
	createErrAt int
}

type closeCall struct {
	issueID string
	reason  string
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
func (f *fakeBeads) List(_ context.Context, filters map[string]string) ([]model.Issue, error) {
	f.listFilters = map[string]string{}
	for k, v := range filters {
		f.listFilters[k] = v
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.list != nil {
		return f.list, nil
	}
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
func (f *fakeBeads) Claim(_ context.Context, issueID string) error {
	f.claimed = append(f.claimed, issueID)
	return nil
}
func (f *fakeBeads) Close(_ context.Context, issueID, reason string) error {
	f.closed = append(f.closed, closeCall{issueID: issueID, reason: reason})
	if f.closeErr != nil {
		return f.closeErr
	}
	return nil
}

type fakeGit struct {
	headBranch    string
	headCommit    string
	addedPath     string
	addedBranch   string
	addedStart    string
	merged        [][2]string
	removedPaths  []string
	deletedBranch []string
	removeErr     error
	deleteErr     error
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
func (g *fakeGit) RemoveWorktree(_ context.Context, _ string, path string) error {
	if g.removeErr != nil {
		return g.removeErr
	}
	g.removedPaths = append(g.removedPaths, path)
	return nil
}
func (g *fakeGit) Merge(_ context.Context, repoPath, branch string) error {
	g.merged = append(g.merged, [2]string{repoPath, branch})
	return nil
}
func (g *fakeGit) DeleteBranch(_ context.Context, _ string, branch string) error {
	if g.deleteErr != nil {
		return g.deleteErr
	}
	g.deletedBranch = append(g.deletedBranch, branch)
	return nil
}
func (g *fakeGit) Push(context.Context, string, string) error { return nil }

type fakePlanner struct{ d app.BranchDecision }

func (p fakePlanner) ResolveBranch(model.Issue, []model.TaskBranchMeta) app.BranchDecision {
	return p.d
}

type fakePromptBuilder struct{ out string }

func (b fakePromptBuilder) Build(model.Issue, model.TaskBranchMeta) string { return b.out }

type fakeTmux struct {
	paneID          string
	captured        string
	splitErr        error
	sendErr         error
	captureErr      error
	bufferErr       error
	titleErr        error
	sent            []string
	splitDir        string
	splitCWD        string
	splitTarget     string
	splitCommand    string
	pasted          []string
	killed          []string
	panes           []string
	titles          map[string]string
	terminalWidth   int
	terminalHeight  int
	windowWidth     int
	windowHeight    int
	layouts         []string
	sidebarSetCalls int
	windowSizeCalls int
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

func (t *fakeTmux) SplitPaneOnTarget(_ context.Context, direction, cwd, target string) (string, error) {
	t.splitTarget = target
	return t.SplitPane(context.Background(), direction, cwd)
}

func (t *fakeTmux) SplitPaneOnTargetWithCommand(_ context.Context, direction, cwd, target, command string) (string, error) {
	t.splitTarget = target
	t.splitCommand = command
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
	if len(t.panes) > 0 {
		out := make([]string, len(t.panes))
		copy(out, t.panes)
		return out, nil
	}
	if strings.TrimSpace(t.paneID) != "" {
		return []string{"%1", t.paneID}, nil
	}
	return []string{"%1", "%2"}, nil
}
func (t *fakeTmux) SetWindowOptionsForSidebar(context.Context, string, int) error {
	t.sidebarSetCalls++
	return nil
}
func (t *fakeTmux) SelectLayoutMainVertical(context.Context, string) error {
	t.layouts = append(t.layouts, "main-vertical")
	return nil
}
func (t *fakeTmux) SelectLayout(_ context.Context, _ string, layout string) error {
	t.layouts = append(t.layouts, layout)
	return nil
}
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
func (t *fakeTmux) GetWindowDimensions(context.Context) (int, int, error) {
	w := t.windowWidth
	h := t.windowHeight
	if w == 0 {
		w = 180
	}
	if h == 0 {
		h = 50
	}
	return w, h, nil
}
func (t *fakeTmux) GetTerminalDimensions(context.Context) (int, int, error) {
	w := t.terminalWidth
	h := t.terminalHeight
	if w == 0 {
		w = 180
	}
	if h == 0 {
		h = 50
	}
	return w, h, nil
}
func (t *fakeTmux) SetWindowSizeManual(_ context.Context, _ string, width, height int) error {
	t.windowWidth = width
	t.windowHeight = height
	t.windowSizeCalls++
	return nil
}
func (t *fakeTmux) SetPaneTitle(_ context.Context, paneID, title string) error {
	if t.titleErr != nil {
		return t.titleErr
	}
	if t.titles == nil {
		t.titles = map[string]string{}
	}
	t.titles[paneID] = title
	return nil
}
func (t *fakeTmux) GetPaneTitle(_ context.Context, paneID string) (string, error) {
	if t.titles == nil {
		return "", nil
	}
	return t.titles[paneID], nil
}
func (t *fakeTmux) KillPane(_ context.Context, paneID string) error {
	t.killed = append(t.killed, paneID)
	if len(t.panes) == 0 {
		return nil
	}
	next := make([]string, 0, len(t.panes))
	for _, pane := range t.panes {
		if pane != paneID {
			next = append(next, pane)
		}
	}
	t.panes = next
	return nil
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

func TestMergeTaskClosesIssueAfterMerge(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	meta := model.TaskBranchMeta{
		IssueID:      "bd-merge",
		Branch:       "task/bd-merge",
		WorktreePath: root + "/.worktrees/task__bd-merge",
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.Upsert(meta); err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	beads := &fakeBeads{}
	git := &fakeGit{}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: git, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	got, err := svc.MergeTask(context.Background(), "bd-merge", false)
	if err != nil {
		t.Fatalf("merge task: %v", err)
	}
	if got.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Status)
	}
	if len(git.merged) != 2 {
		t.Fatalf("merge calls = %#v", git.merged)
	}
	if len(beads.closed) != 1 {
		t.Fatalf("close calls = %#v", beads.closed)
	}
	if beads.closed[0].issueID != "bd-merge" || beads.closed[0].reason != "Completed via bmux" {
		t.Fatalf("unexpected close call: %#v", beads.closed[0])
	}

	stored, ok, err := store.ByIssueID("bd-merge")
	if err != nil {
		t.Fatalf("read stored meta: %v", err)
	}
	if !ok {
		t.Fatal("expected stored meta")
	}
	if stored.Status != "merged" {
		t.Fatalf("stored status = %q, want merged", stored.Status)
	}
}

func TestMergeTaskCloseFailurePersistsMergedPendingClose(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	meta := model.TaskBranchMeta{
		IssueID:      "bd-merge-fail",
		Branch:       "task/bd-merge-fail",
		WorktreePath: root + "/.worktrees/task__bd-merge-fail",
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.Upsert(meta); err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	beads := &fakeBeads{closeErr: errors.New("bd close failed")}
	git := &fakeGit{}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: git, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	_, err := svc.MergeTask(context.Background(), "bd-merge-fail", false)
	if err == nil {
		t.Fatalf("expected close error")
	}
	if !strings.Contains(err.Error(), "bd close failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(git.merged) != 2 {
		t.Fatalf("merge calls = %#v", git.merged)
	}
	if len(beads.closed) != 1 {
		t.Fatalf("close calls = %#v", beads.closed)
	}

	stored, ok, err := store.ByIssueID("bd-merge-fail")
	if err != nil {
		t.Fatalf("read stored meta: %v", err)
	}
	if !ok {
		t.Fatal("expected stored meta")
	}
	if stored.Status != "merged_pending_close" {
		t.Fatalf("stored status = %q, want merged_pending_close", stored.Status)
	}
}

func TestMergeTaskSkipsCleanupWhenBranchShared(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	sharedBranch := "task/bd-shared"
	sharedWorktree := root + "/.worktrees/task__bd-shared"
	first := model.TaskBranchMeta{
		IssueID:      "bd-shared-1",
		Branch:       sharedBranch,
		WorktreePath: sharedWorktree,
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	}
	second := model.TaskBranchMeta{
		IssueID:      "bd-shared-2",
		Branch:       sharedBranch,
		WorktreePath: sharedWorktree,
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.Upsert(first); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if err := store.Upsert(second); err != nil {
		t.Fatalf("seed second: %v", err)
	}

	beads := &fakeBeads{}
	git := &fakeGit{}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: git, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	got, err := svc.MergeTask(context.Background(), "bd-shared-1", true)
	if err != nil {
		t.Fatalf("merge task: %v", err)
	}
	if got.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Status)
	}
	if len(git.removedPaths) != 0 {
		t.Fatalf("cleanup should be skipped for shared worktree, removed=%#v", git.removedPaths)
	}
	if len(git.deletedBranch) != 0 {
		t.Fatalf("cleanup should be skipped for shared branch, deleted=%#v", git.deletedBranch)
	}
}

func TestMergeTaskCleansUpWhenUnshared(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	meta := model.TaskBranchMeta{
		IssueID:      "bd-cleanup",
		Branch:       "task/bd-cleanup",
		WorktreePath: root + "/.worktrees/task__bd-cleanup",
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.Upsert(meta); err != nil {
		t.Fatalf("seed meta: %v", err)
	}
	if err := store.Upsert(model.TaskBranchMeta{
		IssueID:      "bd-old",
		Branch:       "task/bd-cleanup",
		WorktreePath: root + "/.worktrees/task__bd-cleanup",
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "merged",
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed old meta: %v", err)
	}

	beads := &fakeBeads{}
	git := &fakeGit{}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: git, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	got, err := svc.MergeTask(context.Background(), "bd-cleanup", true)
	if err != nil {
		t.Fatalf("merge task: %v", err)
	}
	if got.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Status)
	}
	if len(git.removedPaths) != 1 || git.removedPaths[0] != meta.WorktreePath {
		t.Fatalf("removed paths = %#v", git.removedPaths)
	}
	if len(git.deletedBranch) != 1 || git.deletedBranch[0] != meta.Branch {
		t.Fatalf("deleted branches = %#v", git.deletedBranch)
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

func TestListIssuesStateAllStatusesAndFilters(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		list: []model.Issue{
			{ID: "bd-inprog", Title: "In Progress", IssueType: "task", Status: "in_progress", ParentID: "bd-epic"},
			{ID: "bd-epic", Title: "Epic", IssueType: "epic", Status: "open"},
			{ID: "bd-closed", Title: "Closed", IssueType: "task", Status: "closed", ParentID: "bd-epic"},
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

	issues, source, err := svc.ListIssuesState(context.Background())
	if err != nil {
		t.Fatalf("list issues state: %v", err)
	}
	if !source.Available {
		t.Fatalf("expected source available")
	}
	if len(issues) != 3 {
		t.Fatalf("issues len = %d, want 3", len(issues))
	}
	if got := beads.listFilters["all"]; got != "true" {
		t.Fatalf("list all filter = %q, want true", got)
	}
	if got := beads.listFilters["limit"]; got != "0" {
		t.Fatalf("list limit filter = %q, want 0", got)
	}
	if issues[2].EpicID != "bd-epic" || issues[2].HierarchyDepth != 1 {
		t.Fatalf("closed issue hierarchy = (%q,%d)", issues[2].EpicID, issues[2].HierarchyDepth)
	}
}

func TestListIssuesStateBDUnavailable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := app.NewService(app.Options{
		RepoRoot:    root,
		WorktreeDir: root + "/.worktrees",
		Store:       state.New(root),
		Beads: &fakeBeads{listErr: &errorsx.CommandError{
			Command:  "bd",
			Args:     []string{"list", "--all", "--limit", "0", "--json"},
			Dir:      root,
			StdErr:   "command not found: bd",
			ExitCode: 127,
			Err:      errors.New("exit status 127"),
		}},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ListIssuesState(context.Background())
	if err != nil {
		t.Fatalf("list issues state: %v", err)
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

func TestListIssuesStateUnexpectedError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	expected := errors.New("list parse failure")
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         &fakeBeads{listErr: expected},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	issues, source, err := svc.ListIssuesState(context.Background())
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
		Tmux: tmux, SplitDirection: "below", TmuxLayout: "sidebar", ClaudeCommand: "claude --plan",
	})
	paneID, _, err := svc.StartPlanningPane(context.Background(), "claude")
	if err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if paneID != "%3" {
		t.Fatalf("pane id = %q", paneID)
	}
	if tmux.splitDir != "right" {
		t.Fatalf("split dir = %q", tmux.splitDir)
	}
	if len(tmux.sent) != 2 {
		t.Fatalf("send calls = %d", len(tmux.sent))
	}
	if len(tmux.titles) != 0 {
		t.Fatalf("planning pane should not set task pane title: %#v", tmux.titles)
	}
}

func TestStartPlanningPaneSidebarTargetsLastNonSpacerPane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		paneID: "%7",
		panes:  []string{"%1", "%2", "%9"},
		titles: map[string]string{"%9": "bmux-spacer"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: &fakeBeads{}, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
		Tmux: tmux, TmuxLayout: "sidebar", ClaudeCommand: "claude --plan",
	})
	if _, _, err := svc.StartPlanningPane(context.Background(), "claude"); err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if tmux.splitTarget != "%2" {
		t.Fatalf("split target = %q, want %q", tmux.splitTarget, "%2")
	}
}

func TestReconcileRunsTriggersLayoutRecalculate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		panes:          []string{"%1", "%2", "%3"},
		terminalWidth:  200,
		terminalHeight: 50,
		windowWidth:    200,
		windowHeight:   50,
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: &fakeBeads{}, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
		Tmux: tmux, TmuxLayout: "sidebar", ControlWidth: 40, MinPaneWidth: 50, MaxPaneWidth: 80,
	})
	if err := svc.ReconcileRuns(context.Background()); err != nil {
		t.Fatalf("reconcile runs: %v", err)
	}
	if len(tmux.layouts) == 0 {
		t.Fatalf("expected layout recalc to apply layout")
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
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-ape", Title: "Ape run", Status: "open"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-ape-run"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	if _, err := svc.StartTaskMode(context.Background(), "bd-ape", model.RunModeApe); err != nil {
		t.Fatalf("start task mode: %v", err)
	}
	if len(beads.claimed) != 1 || beads.claimed[0] != "bd-ape" {
		t.Fatalf("expected claim for bd-ape, got %#v", beads.claimed)
	}
}

func TestTickApeKeepsSessionActiveWhileRunLockStillExists(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC()

	if err := store.SetChaosState(&model.ChaosState{
		SessionID:        "chaos-1",
		Active:           true,
		LaunchedIssueIDs: []string{"bd-42"},
		FinishedIssueIDs: []string{},
		ActiveIssueIDs:   []string{"bd-42"},
		StartedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed chaos state: %v", err)
	}
	if err := store.RunLockUpsert(model.TaskRunMeta{
		IssueID:         "bd-42",
		Mode:            model.RunModeApe,
		Agent:           "codex",
		PaneID:          "%42",
		StartedAt:       now,
		UpdatedAt:       now,
		ExpectedProcess: "codex",
		ChaosSessionID:  "chaos-1",
	}); err != nil {
		t.Fatalf("seed run lock: %v", err)
	}

	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads:         &fakeBeads{ready: []model.Issue{}},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
		Tmux:          &fakeTmux{paneID: "%42"},
		CodexCommand:  "codex",
	})

	status, done, err := svc.TickApe(context.Background())
	if err != nil {
		t.Fatalf("tick ape: %v", err)
	}
	if done {
		t.Fatalf("done = true, want false; status=%q", status)
	}
	if !strings.Contains(status, "running=1") {
		t.Fatalf("expected running=1 in status, got %q", status)
	}

	chaos, ok, err := store.ChaosState()
	if err != nil {
		t.Fatalf("read chaos state: %v", err)
	}
	if !ok {
		t.Fatal("expected chaos state")
	}
	if !chaos.Active {
		t.Fatalf("chaos active = false, want true: %+v", chaos)
	}
	if len(chaos.ActiveIssueIDs) != 1 || chaos.ActiveIssueIDs[0] != "bd-42" {
		t.Fatalf("active issues = %#v, want [\"bd-42\"]", chaos.ActiveIssueIDs)
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

func TestStartTaskModeWaitingCreatesPendingRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%12"}
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-wait", Title: "Wait task", Status: "open"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-wait-task"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	run, err := svc.StartTaskModeWaiting(context.Background(), "bd-wait", model.RunModeSelfRun, "bd-blocker")
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
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-18", Title: "Title fail", Status: "open"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-18-title-fail"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	_, err := svc.StartTaskMode(context.Background(), "bd-18", model.RunModePlan)
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
	beads := &fakeBeads{
		issue: model.Issue{ID: "bd-19", Title: "Waiting title fail", Status: "open"},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-19-waiting-title-fail"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	_, err := svc.StartTaskModeWaiting(context.Background(), "bd-19", model.RunModePlan, "bd-blocker")
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
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-promote-task"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	if _, err := svc.StartTaskModeWaiting(context.Background(), "bd-promote", model.RunModePlan, "bd-blocker"); err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	promoted, waiting, err := svc.PromotePendingRuns(context.Background())
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

	runMeta, ok, err := svc.GetTaskRunMeta("bd-promote")
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
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-20-promote-title-fail"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	if _, err := svc.StartTaskModeWaiting(context.Background(), "bd-20", model.RunModePlan, "bd-blocker"); err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	tmux.titleErr = errors.New("title failed")

	promoted, waiting, err := svc.PromotePendingRuns(context.Background())
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
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: beads, Git: &fakeGit{headBranch: "main", headCommit: "abc123"},
		Planner:       fakePlanner{d: app.BranchDecision{Branch: "task/bd-still-waiting"}},
		PromptBuilder: fakePromptBuilder{}, Tmux: tmux, CodexCommand: "codex",
	})

	if _, err := svc.StartTaskModeWaiting(context.Background(), "bd-still", model.RunModePlan, "bd-blocker"); err != nil {
		t.Fatalf("start waiting mode: %v", err)
	}
	promoted, waiting, err := svc.PromotePendingRuns(context.Background())
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

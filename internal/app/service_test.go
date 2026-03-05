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
	depAdds     []depAddCall
	depAddErr   error
	created     []model.CreateIssueRequest
	createOut   []model.Issue
	createErrAt int
}

type closeCall struct {
	issueID string
	reason  string
}

type depAddCall struct {
	issueID     string
	blockedByID string
	depType     string
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
func (f *fakeBeads) AddDependency(_ context.Context, issueID, blockedByID, depType string) error {
	f.depAdds = append(f.depAdds, depAddCall{
		issueID:     issueID,
		blockedByID: blockedByID,
		depType:     depType,
	})
	if f.depAddErr != nil {
		return f.depAddErr
	}
	return nil
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
	headByPath    map[string]gitHead
	headErrByPath map[string]error
	addedPath     string
	addedBranch   string
	addedStart    string
	merged        [][2]string
	mergeErrByKey map[string]error
	removedPaths  []string
	deletedBranch []string
	removeErr     error
	deleteErr     error
}

type gitHead struct {
	branch string
	commit string
}

func (g *fakeGit) CurrentHead(_ context.Context, path string) (string, string, error) {
	if err, ok := g.headErrByPath[path]; ok {
		return "", "", err
	}
	if g.headByPath != nil {
		if head, ok := g.headByPath[path]; ok {
			return head.branch, head.commit, nil
		}
	}
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
	key := repoPath + "|" + branch
	if err, ok := g.mergeErrByKey[key]; ok {
		return err
	}
	g.merged = append(g.merged, [2]string{repoPath, branch})
	if g.headByPath != nil {
		if head, ok := g.headByPath[repoPath]; ok {
			head.commit = head.commit + "m"
			g.headByPath[repoPath] = head
		}
	}
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
	if got.Meta.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Meta.Status)
	}
	if len(got.Steps) < 3 {
		t.Fatalf("expected merge detail steps, got %d", len(got.Steps))
	}
	lines := got.DetailLines()
	if len(lines) == 0 || !strings.Contains(lines[0], "Step 1") {
		t.Fatalf("expected step detail lines, got %#v", lines)
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

func TestMergeTaskConflictReturnsTypedErrorAndKeepsIssueOpen(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	meta := model.TaskBranchMeta{
		IssueID:      "bd-conflict",
		Branch:       "task/bd-conflict",
		WorktreePath: root + "/.worktrees/task__bd-conflict",
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.Upsert(meta); err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	conflictErr := &errorsx.CommandError{
		Command:  "git",
		Args:     []string{"merge", "main", "--no-edit"},
		Dir:      meta.WorktreePath,
		StdErr:   "CONFLICT (content): Merge conflict in README.md\nAutomatic merge failed; fix conflicts and then commit the result.",
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	}
	beads := &fakeBeads{}
	git := &fakeGit{
		headByPath: map[string]gitHead{
			meta.WorktreePath: {branch: meta.Branch, commit: "abc123"},
			root:              {branch: meta.BaseBranch, commit: "def456"},
		},
		mergeErrByKey: map[string]error{
			meta.WorktreePath + "|" + meta.BaseBranch: conflictErr,
		},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: git, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	got, err := svc.MergeTask(context.Background(), "bd-conflict", true)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	var conflict *app.MergeConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected MergeConflictError, got %T (%v)", err, err)
	}
	if len(beads.closed) != 0 {
		t.Fatalf("close should not be called on conflict")
	}
	if len(got.Steps) != 1 || got.Steps[0].Success {
		t.Fatalf("unexpected steps: %#v", got.Steps)
	}
	stored, ok, readErr := store.ByIssueID("bd-conflict")
	if readErr != nil {
		t.Fatalf("read stored meta: %v", readErr)
	}
	if !ok {
		t.Fatal("expected stored meta")
	}
	if stored.Status != "active" {
		t.Fatalf("stored status = %q, want active", stored.Status)
	}
}

func TestCreateMergeConflictTaskCreatesAndLinksBlockingIssue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		createOut: []model.Issue{{ID: "bd-conflict-fix"}},
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	created, err := svc.CreateMergeConflictTask(context.Background(), "bd-1", &app.MergeConflictError{
		Stage:        "Step 2: repo merge",
		SourceBranch: "task/bd-1",
		TargetBranch: "main",
		RepoPath:     root,
		StdErr:       "CONFLICT (content): Merge conflict in file.txt",
	})
	if err != nil {
		t.Fatalf("create merge conflict task: %v", err)
	}
	if created.ID != "bd-conflict-fix" {
		t.Fatalf("created id = %q", created.ID)
	}
	if len(beads.created) != 1 {
		t.Fatalf("create calls = %#v", beads.created)
	}
	if beads.created[0].Type != "chore" || beads.created[0].Priority != 0 {
		t.Fatalf("unexpected create request: %#v", beads.created[0])
	}
	if strings.TrimSpace(beads.created[0].ParentID) != "" {
		t.Fatalf("expected no parent id, got %q", beads.created[0].ParentID)
	}
	if len(beads.depAdds) != 1 {
		t.Fatalf("dep add calls = %#v", beads.depAdds)
	}
	if beads.depAdds[0].issueID != "bd-1" || beads.depAdds[0].blockedByID != "bd-conflict-fix" || beads.depAdds[0].depType != "blocks" {
		t.Fatalf("unexpected dep add call: %#v", beads.depAdds[0])
	}
}

func TestCreateMergeConflictTaskLinkFailureReturnsManualHint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := state.New(root)
	beads := &fakeBeads{
		createOut: []model.Issue{{ID: "bd-conflict-fix"}},
		depAddErr: errors.New("dep add failed"),
	}
	svc := app.NewService(app.Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: store,
		Beads: beads, Git: &fakeGit{}, Planner: fakePlanner{}, PromptBuilder: fakePromptBuilder{},
	})

	_, err := svc.CreateMergeConflictTask(context.Background(), "bd-1", &app.MergeConflictError{
		Stage:        "Step 1: worktree merge",
		SourceBranch: "main",
		TargetBranch: "task/bd-1",
		RepoPath:     root,
		StdErr:       "CONFLICT",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "Manual recovery:") || !strings.Contains(err.Error(), "bd dep add") {
		t.Fatalf("unexpected error: %v", err)
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
	if got.Meta.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Meta.Status)
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
	if got.Meta.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Meta.Status)
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

func TestComputeBlockedByUsesBlocksDependencyOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         &fakeBeads{},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	blockedBy, err := svc.ComputeBlockedBy(context.Background(), []model.Issue{
		{ID: "bd-101", Title: "Blocker", Status: "open"},
		{
			ID:     "bd-103",
			Title:  "Blocked",
			Status: "open",
			Dependencies: []model.Dependency{
				{Type: "blocks", TargetID: "bd-101"},
			},
		},
	})
	if err != nil {
		t.Fatalf("compute blocked-by: %v", err)
	}
	if got := blockedBy["bd-103"]; got != "bd-101" {
		t.Fatalf("blockedBy[bd-103] = %q, want bd-101", got)
	}
}

func TestComputeBlockedByIgnoresParentChildWithoutBlocks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := app.NewService(app.Options{
		RepoRoot:      root,
		WorktreeDir:   root + "/.worktrees",
		Store:         state.New(root),
		Beads:         &fakeBeads{},
		Git:           &fakeGit{},
		Planner:       fakePlanner{},
		PromptBuilder: fakePromptBuilder{},
	})

	blockedBy, err := svc.ComputeBlockedBy(context.Background(), []model.Issue{
		{ID: "bd-parent", Title: "Parent", Status: "open"},
		{ID: "bd-child", Title: "Child", Status: "open", ParentID: "bd-parent"},
	})
	if err != nil {
		t.Fatalf("compute blocked-by: %v", err)
	}
	if got := blockedBy["bd-child"]; got != "" {
		t.Fatalf("blockedBy[bd-child] = %q, want empty", got)
	}
}

func TestComputeBlockedByFallsBackToFetchedDependenciesWhenEmbeddedUnusable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beads := &fakeBeads{
		deps: []model.Dependency{
			{Type: "blocks", Direction: "outgoing", IssueID: "bd-101"},
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

	blockedBy, err := svc.ComputeBlockedBy(context.Background(), []model.Issue{
		{ID: "bd-101", Title: "Blocker", Status: "open"},
		{
			ID:     "bd-103",
			Title:  "Blocked",
			Status: "open",
			Dependencies: []model.Dependency{
				{Type: "blocks"},
			},
		},
	})
	if err != nil {
		t.Fatalf("compute blocked-by: %v", err)
	}
	if got := blockedBy["bd-103"]; got != "bd-101" {
		t.Fatalf("blockedBy[bd-103] = %q, want bd-101", got)
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

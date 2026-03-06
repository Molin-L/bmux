package agentexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/layout"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

func newTestExecutor(root string, beads *fakeBeads, tmux *fakeTmux, codexCmd, claudeCmd, tmuxLayout string) *Executor {
	if beads == nil {
		beads = &fakeBeads{}
	}
	if tmux == nil {
		tmux = &fakeTmux{paneID: "%2"}
	}
	if tmuxLayout == "" {
		tmuxLayout = "sidebar"
	}
	return New(Options{
		RepoRoot:       root,
		Store:          state.New(root),
		Beads:          beads,
		Tmux:           tmux,
		SplitDirection: "below",
		TmuxLayout:     tmuxLayout,
		ControlWidth:   40,
		MinPaneWidth:   50,
		MaxPaneWidth:   80,
		ClaudeCommand:  claudeCmd,
		CodexCommand:   codexCmd,
		OpenTask: func(_ context.Context, issueID string) (model.TaskBranchMeta, error) {
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

func TestStartPlanningPane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{paneID: "%3"}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "", "claude --plan", "sidebar")

	paneID, _, err := e.StartPlanningPane(context.Background(), "claude")
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

func TestStartPlanningPaneSidebarTargetsLastNonAuxPane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		paneID: "%7",
		panes:  []string{"%1", "%2", "%9", "%8"},
		titles: map[string]string{"%9": layout.SpacerPaneTitle, "%8": layout.IdlePaneTitle},
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "", "claude --plan", "sidebar")
	if _, _, err := e.StartPlanningPane(context.Background(), "claude"); err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if tmux.splitTarget != "%2" {
		t.Fatalf("split target = %q, want %q", tmux.splitTarget, "%2")
	}
}

func TestResolveControlPaneIDPrefersBmuxWhenCurrentPaneIsIdlePane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		panes:         []string{"%1", "%9"},
		currentPaneID: "%9",
		titles: map[string]string{
			"%9": layout.IdlePaneTitle,
		},
		currentByPane: map[string]string{
			"%1": "bmux",
			"%9": "cat",
		},
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex", "", "sidebar")
	controlPaneID, err := e.resolveControlPaneID(context.Background())
	if err != nil {
		t.Fatalf("resolve control pane id: %v", err)
	}
	if controlPaneID != "%1" {
		t.Fatalf("control pane id = %q, want %q", controlPaneID, "%1")
	}
}

func TestResolveControlPaneIDSkipsAuxiliaryCurrentPaneWithoutBmuxMatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		panes:         []string{"%9", "%3", "%4"},
		currentPaneID: "%9",
		titles: map[string]string{
			"%9": layout.IdlePaneTitle,
		},
		currentByPane: map[string]string{
			"%9": "cat",
			"%3": "zsh",
			"%4": "bash",
		},
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex", "", "sidebar")
	controlPaneID, err := e.resolveControlPaneID(context.Background())
	if err != nil {
		t.Fatalf("resolve control pane id: %v", err)
	}
	if controlPaneID != "%3" {
		t.Fatalf("control pane id = %q, want %q", controlPaneID, "%3")
	}
}

func TestReconcileRunsDoesNotLoopIdlePaneWhenCurrentPaneIsAuxiliary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		panes:         []string{"%1", "%9"},
		currentPaneID: "%9",
		titles: map[string]string{
			"%9": layout.IdlePaneTitle,
		},
		currentByPane: map[string]string{
			"%1": "bmux",
			"%9": "cat",
		},
		terminalWidth:  200,
		terminalHeight: 50,
		windowWidth:    200,
		windowHeight:   50,
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex", "", "sidebar")
	if err := e.ReconcileRuns(context.Background()); err != nil {
		t.Fatalf("first reconcile runs: %v", err)
	}
	if err := e.ReconcileRuns(context.Background()); err != nil {
		t.Fatalf("second reconcile runs: %v", err)
	}
	if len(tmux.killed) != 1 || tmux.killed[0] != "%9" {
		t.Fatalf("expected one stale idle-pane cleanup kill, got %#v", tmux.killed)
	}
	if tmux.splitDir != "" {
		t.Fatalf("expected no idle-pane recreate split, got split dir %q", tmux.splitDir)
	}
	if _, ok := tmux.titles["%9"]; ok {
		t.Fatalf("idle pane title should be removed after cleanup")
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
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex", "", "sidebar")
	if err := e.ReconcileRuns(context.Background()); err != nil {
		t.Fatalf("reconcile runs: %v", err)
	}
	if len(tmux.layouts) == 0 {
		t.Fatalf("expected layout recalc to apply layout")
	}
}

func TestReconcileRunsPropagatesLayoutRecalculateError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		panes:          []string{"%1", "%2"},
		terminalWidth:  200,
		terminalHeight: 50,
		windowWidth:    200,
		windowHeight:   50,
		layoutErr:      errors.New("layout failed"),
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex", "", "sidebar")
	err := e.ReconcileRuns(context.Background())
	if err == nil {
		t.Fatalf("expected reconcile runs error")
	}
	if !strings.Contains(err.Error(), "apply layout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartPlanningPaneFailsAndKillsPaneWhenLayoutRecalcFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		paneID:         "%7",
		panes:          []string{"%1", "%2"},
		terminalWidth:  200,
		terminalHeight: 50,
		windowWidth:    200,
		windowHeight:   50,
		layoutErr:      errors.New("layout failed"),
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "", "claude --plan", "sidebar")
	_, _, err := e.StartPlanningPane(context.Background(), "claude")
	if err == nil {
		t.Fatalf("expected planning pane start error")
	}
	if !strings.Contains(err.Error(), "apply layout") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tmux.killed) == 0 || tmux.killed[0] != "%7" {
		t.Fatalf("expected created pane to be killed after recalc failure, got %#v", tmux.killed)
	}
	if len(tmux.sent) != 0 {
		t.Fatalf("expected no command sent when pane creation fails, got %#v", tmux.sent)
	}
}

func TestStartPlanningPaneCodexAutoTrustAndPromptRetry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{
		paneID:   "%7",
		captured: "Do you trust this workspace?",
	}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex", "", "sidebar")

	paneID, status, err := e.StartPlanningPane(context.Background(), "codex")
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

func TestStartPlanningPaneUsesConfigCommand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex --plan", "", "sidebar")
	orig := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = orig }()

	_, status, err := e.StartPlanningPane(context.Background(), "codex")
	if err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if !strings.Contains(status, "config command") {
		t.Fatalf("status = %q", status)
	}
	if len(tmux.sent) == 0 || !strings.HasPrefix(tmux.sent[0], "codex --plan") {
		t.Fatalf("sent = %#v", tmux.sent)
	}
	if !strings.Contains(tmux.sent[0], "--sandbox workspace-write") || !strings.Contains(tmux.sent[0], "--ask-for-approval on-request") {
		t.Fatalf("missing default planning flags in %q", tmux.sent[0])
	}
}

func TestStartPlanningPaneUsesPathFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "", "", "sidebar")
	orig := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/codex", nil }
	defer func() { lookPath = orig }()

	_, status, err := e.StartPlanningPane(context.Background(), "codex")
	if err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if !strings.Contains(status, "PATH fallback") {
		t.Fatalf("status = %q", status)
	}
	if len(tmux.sent) == 0 || !strings.HasPrefix(tmux.sent[0], "codex ") {
		t.Fatalf("sent = %#v", tmux.sent)
	}
	if !strings.Contains(tmux.sent[0], "--sandbox workspace-write") || !strings.Contains(tmux.sent[0], "--ask-for-approval on-request") {
		t.Fatalf("missing default planning flags in %q", tmux.sent[0])
	}
}

func TestStartPlanningPaneMissingCommandError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := newTestExecutor(root, &fakeBeads{}, &fakeTmux{}, "", "", "sidebar")
	orig := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = orig }()

	_, _, err := e.StartPlanningPane(context.Background(), "codex")
	if err == nil {
		t.Fatalf("expected error")
	}
	want := "agent command is not configured and 'codex' is not in PATH (set agents.codex.command)"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

func TestStartPlanningPanePreservesConfiguredPolicyFlags(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &fakeTmux{}
	e := newTestExecutor(root, &fakeBeads{}, tmux, "codex --sandbox workspace-write --ask-for-approval never", "", "sidebar")
	orig := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/codex", nil }
	defer func() { lookPath = orig }()

	_, _, err := e.StartPlanningPane(context.Background(), "codex")
	if err != nil {
		t.Fatalf("start planning pane: %v", err)
	}
	if len(tmux.sent) == 0 {
		t.Fatalf("expected sent command")
	}
	cmd := tmux.sent[0]
	if strings.Count(cmd, "--sandbox ") != 1 || strings.Count(cmd, "--ask-for-approval ") != 1 {
		t.Fatalf("policy flags duplicated in %q", cmd)
	}
	if !strings.Contains(cmd, "--sandbox workspace-write") || !strings.Contains(cmd, "--ask-for-approval never") {
		t.Fatalf("configured flags not preserved in %q", cmd)
	}
}

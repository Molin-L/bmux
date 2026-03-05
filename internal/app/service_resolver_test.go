package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type stubBeads struct{}

func (stubBeads) Ready(context.Context) ([]model.Issue, error)                     { return nil, nil }
func (stubBeads) List(context.Context, map[string]string) ([]model.Issue, error)   { return nil, nil }
func (stubBeads) Show(context.Context, string) (model.Issue, error)                { return model.Issue{}, nil }
func (stubBeads) Dependencies(context.Context, string) ([]model.Dependency, error) { return nil, nil }
func (stubBeads) CreateIssue(context.Context, model.CreateIssueRequest) (model.Issue, error) {
	return model.Issue{}, nil
}
func (stubBeads) Claim(context.Context, string) error                             { return nil }
func (stubBeads) UpdateMetadata(context.Context, string, map[string]string) error { return nil }
func (stubBeads) Close(context.Context, string, string) error                     { return nil }

type stubGit struct{}

func (stubGit) CurrentHead(context.Context, string) (string, string, error) {
	return "main", "abc", nil
}
func (stubGit) AddWorktree(context.Context, string, string, string, string) error {
	return nil
}
func (stubGit) RemoveWorktree(context.Context, string, string) error { return nil }
func (stubGit) Merge(context.Context, string, string) error          { return nil }
func (stubGit) DeleteBranch(context.Context, string, string) error   { return nil }
func (stubGit) Push(context.Context, string, string) error           { return nil }

type stubPlanner struct{}

func (stubPlanner) ResolveBranch(model.Issue, []model.TaskBranchMeta) BranchDecision {
	return BranchDecision{}
}

type stubPromptBuilder struct{}

func (stubPromptBuilder) Build(model.Issue, model.TaskBranchMeta) string { return "" }

type stubTmux struct {
	sent []string
}

func (t *stubTmux) SplitPane(context.Context, string, string) (string, error) { return "%9", nil }
func (t *stubTmux) SplitPaneOnTarget(context.Context, string, string, string) (string, error) {
	return "%9", nil
}
func (t *stubTmux) SplitPaneOnTargetWithCommand(context.Context, string, string, string, string) (string, error) {
	return "%9", nil
}
func (t *stubTmux) SendKeys(_ context.Context, _ string, text string, _ bool) error {
	t.sent = append(t.sent, text)
	return nil
}
func (*stubTmux) CapturePane(context.Context, string, int) (string, error) { return "", nil }
func (*stubTmux) CurrentPaneID(context.Context) (string, error)            { return "%1", nil }
func (*stubTmux) ListPanes(context.Context, string) ([]string, error)      { return []string{"%1"}, nil }
func (*stubTmux) SetWindowOptionsForSidebar(context.Context, string, int) error {
	return nil
}
func (*stubTmux) SelectLayoutMainVertical(context.Context, string) error { return nil }
func (*stubTmux) SelectLayout(context.Context, string, string) error     { return nil }
func (*stubTmux) SetBuffer(context.Context, string, string) error        { return nil }
func (*stubTmux) PasteBuffer(context.Context, string, string) error      { return nil }
func (*stubTmux) DeleteBuffer(context.Context, string) error             { return nil }
func (*stubTmux) GetPaneCurrentCommand(context.Context, string) (string, error) {
	return "codex", nil
}
func (*stubTmux) GetWindowDimensions(context.Context) (int, int, error) {
	return 180, 50, nil
}
func (*stubTmux) GetTerminalDimensions(context.Context) (int, int, error) {
	return 180, 50, nil
}
func (*stubTmux) SetWindowSizeManual(context.Context, string, int, int) error { return nil }
func (*stubTmux) SetPaneTitle(context.Context, string, string) error          { return nil }
func (*stubTmux) GetPaneTitle(context.Context, string) (string, error)        { return "", nil }
func (*stubTmux) KillPane(context.Context, string) error                      { return nil }

func TestStartPlanningPaneUsesConfigCommand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tmux := &stubTmux{}
	svc := NewService(Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: stubBeads{}, Git: stubGit{}, Planner: stubPlanner{}, PromptBuilder: stubPromptBuilder{},
		Tmux: tmux, CodexCommand: "codex --plan",
	})
	orig := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = orig }()

	_, status, err := svc.StartPlanningPane(context.Background(), "codex")
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
	tmux := &stubTmux{}
	svc := NewService(Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: stubBeads{}, Git: stubGit{}, Planner: stubPlanner{}, PromptBuilder: stubPromptBuilder{},
		Tmux: tmux,
	})
	orig := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/codex", nil }
	defer func() { lookPath = orig }()

	_, status, err := svc.StartPlanningPane(context.Background(), "codex")
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
	svc := NewService(Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: stubBeads{}, Git: stubGit{}, Planner: stubPlanner{}, PromptBuilder: stubPromptBuilder{},
		Tmux: &stubTmux{},
	})
	orig := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = orig }()

	_, _, err := svc.StartPlanningPane(context.Background(), "codex")
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
	tmux := &stubTmux{}
	svc := NewService(Options{
		RepoRoot: root, WorktreeDir: root + "/.worktrees", Store: state.New(root),
		Beads: stubBeads{}, Git: stubGit{}, Planner: stubPlanner{}, PromptBuilder: stubPromptBuilder{},
		Tmux: tmux, CodexCommand: "codex --sandbox workspace-write --ask-for-approval never",
	})
	orig := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/codex", nil }
	defer func() { lookPath = orig }()

	_, _, err := svc.StartPlanningPane(context.Background(), "codex")
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

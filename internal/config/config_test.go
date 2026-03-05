package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Molin-L/bmux/internal/config"
)

func TestLoadPrecedence_ProjectOverGlobalOverDefault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	home := t.TempDir()

	globalDir := filepath.Join(home, ".bmux")
	projectDir := filepath.Join(root, ".bmux")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	globalYAML := []byte("worktree_dir: /tmp/global-wt\nbranch_prefix: global/\ntmux:\n  split_direction: below\n  auto_attach: true\n  layout: sidebar\n  control_pane_width: 55\n  session_prefix: bmux-global-\nagents:\n  claude:\n    command: claude --plan\n")
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), globalYAML, 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	projectYAML := []byte("branch_prefix: project/\nplanning:\n  prompt_template: custom prompt\nagents:\n  codex:\n    command: codex --mode plan\ntmux:\n  auto_attach: false\n")
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), projectYAML, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg, err := config.Load(root, home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.WorktreeDir, "/tmp/global-wt"; got != want {
		t.Fatalf("worktree_dir = %q, want %q", got, want)
	}
	if got, want := cfg.BranchPrefix, "project/"; got != want {
		t.Fatalf("branch_prefix = %q, want %q", got, want)
	}
	if got, want := cfg.Tmux.SplitDirection, "below"; got != want {
		t.Fatalf("tmux.split_direction = %q, want %q", got, want)
	}
	if got, want := cfg.Tmux.Layout, "sidebar"; got != want {
		t.Fatalf("tmux.layout = %q, want %q", got, want)
	}
	if got, want := cfg.Tmux.ControlPaneWidth, 55; got != want {
		t.Fatalf("tmux.control_pane_width = %d, want %d", got, want)
	}
	if got, want := cfg.Tmux.SessionPrefix, "bmux-global-"; got != want {
		t.Fatalf("tmux.session_prefix = %q, want %q", got, want)
	}
	if cfg.Tmux.AutoAttach {
		t.Fatalf("tmux.auto_attach = true, want false")
	}
	if got, want := cfg.Agents.Claude.Command, "claude --plan"; got != want {
		t.Fatalf("agents.claude.command = %q, want %q", got, want)
	}
	if got, want := cfg.Agents.Codex.Command, "codex --mode plan"; got != want {
		t.Fatalf("agents.codex.command = %q, want %q", got, want)
	}
	if got, want := cfg.Planning.PromptTemplate, "custom prompt"; got != want {
		t.Fatalf("planning.prompt_template = %q, want %q", got, want)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	home := t.TempDir()

	cfg, err := config.Load(root, home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	wantDir := filepath.Join(root, ".worktrees")
	if cfg.WorktreeDir != wantDir {
		t.Fatalf("default worktree_dir = %q, want %q", cfg.WorktreeDir, wantDir)
	}
	if cfg.BranchPrefix != "task/" {
		t.Fatalf("default branch_prefix = %q, want %q", cfg.BranchPrefix, "task/")
	}
	if cfg.Tmux.SplitDirection != "right" {
		t.Fatalf("default tmux.split_direction = %q, want %q", cfg.Tmux.SplitDirection, "right")
	}
	if !cfg.Tmux.AutoAttach {
		t.Fatalf("default tmux.auto_attach = false, want true")
	}
	if cfg.Tmux.Layout != "sidebar" {
		t.Fatalf("default tmux.layout = %q, want %q", cfg.Tmux.Layout, "sidebar")
	}
	if cfg.Tmux.ControlPaneWidth != 40 {
		t.Fatalf("default tmux.control_pane_width = %d, want %d", cfg.Tmux.ControlPaneWidth, 40)
	}
	if cfg.Tmux.SessionPrefix != "bmux-" {
		t.Fatalf("default tmux.session_prefix = %q, want %q", cfg.Tmux.SessionPrefix, "bmux-")
	}
}

func TestLoadInvalidSplitDirectionFallsBack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	home := t.TempDir()
	projectDir := filepath.Join(root, ".bmux")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), []byte("tmux:\n  split_direction: diagonal\n  layout: grid\n  control_pane_width: -1\n  session_prefix: \"\"\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg, err := config.Load(root, home)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Tmux.SplitDirection != "right" {
		t.Fatalf("tmux.split_direction = %q, want %q", cfg.Tmux.SplitDirection, "right")
	}
	if cfg.Tmux.Layout != "sidebar" {
		t.Fatalf("tmux.layout = %q, want %q", cfg.Tmux.Layout, "sidebar")
	}
	if cfg.Tmux.ControlPaneWidth != 40 {
		t.Fatalf("tmux.control_pane_width = %d, want %d", cfg.Tmux.ControlPaneWidth, 40)
	}
	if cfg.Tmux.SessionPrefix != "bmux-" {
		t.Fatalf("tmux.session_prefix = %q, want %q", cfg.Tmux.SessionPrefix, "bmux-")
	}
}

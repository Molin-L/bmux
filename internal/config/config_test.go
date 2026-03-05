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

	globalYAML := []byte("worktree_dir: /tmp/global-wt\nbranch_prefix: global/\n")
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), globalYAML, 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	projectYAML := []byte("branch_prefix: project/\n")
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
}

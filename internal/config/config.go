package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultBranchPrefix = "task/"
)

type Config struct {
	WorktreeDir      string `yaml:"worktree_dir"`
	BranchPrefix     string `yaml:"branch_prefix"`
	Entrypoint       string `yaml:"entrypoint"`
	SubtaskShareMode string `yaml:"subtask_share_mode"`
}

func defaultConfig(projectRoot string) Config {
	return Config{
		WorktreeDir:      filepath.Join(projectRoot, ".worktrees"),
		BranchPrefix:     DefaultBranchPrefix,
		Entrypoint:       "tui",
		SubtaskShareMode: "dependency",
	}
}

func Load(projectRoot, homeDir string) (Config, error) {
	if projectRoot == "" {
		return Config{}, errors.New("project root is required")
	}
	if homeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve home dir: %w", err)
		}
		homeDir = home
	}

	cfg := defaultConfig(projectRoot)
	globalPath := filepath.Join(homeDir, ".bmux", "config.yaml")
	projectPath := filepath.Join(projectRoot, ".bmux", "config.yaml")

	if err := mergeFromFile(&cfg, globalPath); err != nil {
		return Config{}, err
	}
	if err := mergeFromFile(&cfg, projectPath); err != nil {
		return Config{}, err
	}

	cfg.WorktreeDir = resolveWorktreeDir(cfg.WorktreeDir, projectRoot)
	if cfg.BranchPrefix == "" {
		cfg.BranchPrefix = DefaultBranchPrefix
	}

	return cfg, nil
}

func mergeFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}

	var next Config
	if err := yaml.Unmarshal(data, &next); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}

	if next.WorktreeDir != "" {
		cfg.WorktreeDir = next.WorktreeDir
	}
	if next.BranchPrefix != "" {
		cfg.BranchPrefix = next.BranchPrefix
	}
	if next.Entrypoint != "" {
		cfg.Entrypoint = next.Entrypoint
	}
	if next.SubtaskShareMode != "" {
		cfg.SubtaskShareMode = next.SubtaskShareMode
	}
	return nil
}

func resolveWorktreeDir(worktreeDir, projectRoot string) string {
	if filepath.IsAbs(worktreeDir) {
		return filepath.Clean(worktreeDir)
	}
	if worktreeDir == "" {
		return filepath.Join(projectRoot, ".worktrees")
	}
	return filepath.Join(projectRoot, worktreeDir)
}

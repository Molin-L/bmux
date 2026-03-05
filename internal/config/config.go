package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultBranchPrefix = "task/"
)

type Config struct {
	WorktreeDir      string         `yaml:"worktree_dir"`
	BranchPrefix     string         `yaml:"branch_prefix"`
	Entrypoint       string         `yaml:"entrypoint"`
	SubtaskShareMode string         `yaml:"subtask_share_mode"`
	Agents           AgentsConfig   `yaml:"agents"`
	Tmux             TmuxConfig     `yaml:"tmux"`
	Planning         PlanningConfig `yaml:"planning"`
}

type AgentsConfig struct {
	Claude AgentConfig `yaml:"claude"`
	Codex  AgentConfig `yaml:"codex"`
}

type AgentConfig struct {
	Command string `yaml:"command"`
}

type TmuxConfig struct {
	SplitDirection   string `yaml:"split_direction"`
	AutoAttach       bool   `yaml:"auto_attach"`
	Layout           string `yaml:"layout"`
	ControlPaneWidth int    `yaml:"control_pane_width"`
	SessionPrefix    string `yaml:"session_prefix"`
}

type PlanningConfig struct {
	PromptTemplate string `yaml:"prompt_template"`
}

func defaultConfig(projectRoot string) Config {
	return Config{
		WorktreeDir:      filepath.Join(projectRoot, ".worktrees"),
		BranchPrefix:     DefaultBranchPrefix,
		Entrypoint:       "tui",
		SubtaskShareMode: "dependency",
		Tmux: TmuxConfig{
			SplitDirection:   "right",
			AutoAttach:       true,
			Layout:           "sidebar",
			ControlPaneWidth: 40,
			SessionPrefix:    "bmux-",
		},
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
	if cfg.Tmux.SplitDirection != "right" && cfg.Tmux.SplitDirection != "below" {
		cfg.Tmux.SplitDirection = "right"
	}
	if cfg.Tmux.Layout != "sidebar" && cfg.Tmux.Layout != "single" {
		cfg.Tmux.Layout = "sidebar"
	}
	if cfg.Tmux.ControlPaneWidth <= 0 {
		cfg.Tmux.ControlPaneWidth = 40
	}
	if strings.TrimSpace(cfg.Tmux.SessionPrefix) == "" {
		cfg.Tmux.SessionPrefix = "bmux-"
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
	if next.Agents.Claude.Command != "" {
		cfg.Agents.Claude.Command = next.Agents.Claude.Command
	}
	if next.Agents.Codex.Command != "" {
		cfg.Agents.Codex.Command = next.Agents.Codex.Command
	}
	if next.Tmux.SplitDirection != "" {
		cfg.Tmux.SplitDirection = next.Tmux.SplitDirection
	}
	if next.Tmux.Layout != "" {
		cfg.Tmux.Layout = next.Tmux.Layout
	}
	if next.Tmux.ControlPaneWidth != 0 {
		cfg.Tmux.ControlPaneWidth = next.Tmux.ControlPaneWidth
	}
	if next.Tmux.SessionPrefix != "" {
		cfg.Tmux.SessionPrefix = next.Tmux.SessionPrefix
	}
	// Respect explicit false values by always copying booleans.
	cfg.Tmux.AutoAttach = next.Tmux.AutoAttach || cfg.Tmux.AutoAttach
	if !next.Tmux.AutoAttach {
		// If key was present with false, yaml unmarshalling sets false; this keeps
		// project/global opt-out possible while default stays true.
		var probe struct {
			Tmux struct {
				AutoAttach *bool `yaml:"auto_attach"`
			} `yaml:"tmux"`
		}
		if err := yaml.Unmarshal(data, &probe); err == nil && probe.Tmux.AutoAttach != nil {
			cfg.Tmux.AutoAttach = *probe.Tmux.AutoAttach
		}
	}
	if next.Planning.PromptTemplate != "" {
		cfg.Planning.PromptTemplate = next.Planning.PromptTemplate
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

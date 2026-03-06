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
	DefaultBranchPrefix     = "task/"
	minPaneWidthMin         = 40
	minPaneWidthMax         = 300
	defaultControlPaneWidth = 55
	defaultMinPaneWidth     = 60
	defaultMaxPaneWidth     = 100
)

type Config struct {
	WorktreeDir      string          `yaml:"worktree_dir"`
	BranchPrefix     string          `yaml:"branch_prefix"`
	Entrypoint       string          `yaml:"entrypoint"`
	SubtaskShareMode string          `yaml:"subtask_share_mode"`
	Agents           AgentsConfig    `yaml:"agents"`
	Tmux             TmuxConfig      `yaml:"tmux"`
	Planning         PlanningConfig  `yaml:"planning"`
	Execution        ExecutionConfig `yaml:"execution"`
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
	MinPaneWidth     int    `yaml:"min_pane_width"`
	MaxPaneWidth     int    `yaml:"max_pane_width"`
}

type PlanningConfig struct {
	PromptTemplate string `yaml:"prompt_template"`
}

type ExecutionConfig struct {
	ApeMaxParallel int                    `yaml:"ape_max_parallel"`
	Prompts        ExecutionPromptsConfig `yaml:"prompts"`
}

type ExecutionPromptsConfig struct {
	Plan    string `yaml:"plan"`
	SelfRun string `yaml:"self_run"`
	Ape     string `yaml:"ape"`
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
			ControlPaneWidth: defaultControlPaneWidth,
			SessionPrefix:    "bmux-",
			MinPaneWidth:     defaultMinPaneWidth,
			MaxPaneWidth:     defaultMaxPaneWidth,
		},
		Execution: ExecutionConfig{
			ApeMaxParallel: 2,
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
		cfg.Tmux.ControlPaneWidth = defaultControlPaneWidth
	}
	if strings.TrimSpace(cfg.Tmux.SessionPrefix) == "" {
		cfg.Tmux.SessionPrefix = "bmux-"
	}
	cfg.Tmux.MinPaneWidth = clampPaneWidth(cfg.Tmux.MinPaneWidth)
	cfg.Tmux.MaxPaneWidth = clampPaneWidth(cfg.Tmux.MaxPaneWidth)
	if cfg.Tmux.MinPaneWidth > cfg.Tmux.MaxPaneWidth {
		cfg.Tmux.MaxPaneWidth = cfg.Tmux.MinPaneWidth
	}
	if cfg.Execution.ApeMaxParallel <= 0 {
		cfg.Execution.ApeMaxParallel = 2
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
	if next.Tmux.MinPaneWidth != 0 {
		cfg.Tmux.MinPaneWidth = next.Tmux.MinPaneWidth
	}
	if next.Tmux.MaxPaneWidth != 0 {
		cfg.Tmux.MaxPaneWidth = next.Tmux.MaxPaneWidth
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
	if next.Execution.ApeMaxParallel != 0 {
		cfg.Execution.ApeMaxParallel = next.Execution.ApeMaxParallel
	}
	if next.Execution.Prompts.Plan != "" {
		cfg.Execution.Prompts.Plan = next.Execution.Prompts.Plan
	}
	if next.Execution.Prompts.SelfRun != "" {
		cfg.Execution.Prompts.SelfRun = next.Execution.Prompts.SelfRun
	}
	if next.Execution.Prompts.Ape != "" {
		cfg.Execution.Prompts.Ape = next.Execution.Prompts.Ape
	}
	return nil
}

func clampPaneWidth(v int) int {
	if v < minPaneWidthMin {
		return minPaneWidthMin
	}
	if v > minPaneWidthMax {
		return minPaneWidthMax
	}
	return v
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

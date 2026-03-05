package app

import (
	"fmt"
	"os"

	"github.com/Molin-L/bmux/internal/beads"
	"github.com/Molin-L/bmux/internal/branching"
	"github.com/Molin-L/bmux/internal/config"
	"github.com/Molin-L/bmux/internal/execx"
	"github.com/Molin-L/bmux/internal/gitx"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/pr"
	"github.com/Molin-L/bmux/internal/state"
	"github.com/Molin-L/bmux/internal/tmux"
)

type plannerAdapter struct {
	planner *branching.Planner
}

func (p plannerAdapter) ResolveBranch(issue model.Issue, metas []model.TaskBranchMeta) BranchDecision {
	decision := p.planner.ResolveBranch(issue, metas)
	return BranchDecision{
		Branch:        decision.Branch,
		ReuseExisting: decision.ReuseExisting,
		SourceIssueID: decision.SourceIssueID,
	}
}

func NewDefaultService(repoRoot string) (*Service, config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	cfg, err := config.Load(repoRoot, home)
	if err != nil {
		return nil, config.Config{}, err
	}

	runner := execx.New(0)
	store := state.New(repoRoot)
	svc := NewService(Options{
		RepoRoot:               repoRoot,
		WorktreeDir:            cfg.WorktreeDir,
		Store:                  store,
		Beads:                  beads.NewClient(repoRoot, runner),
		Git:                    gitx.NewClient(runner),
		Planner:                plannerAdapter{planner: branching.NewPlanner(cfg.BranchPrefix)},
		PromptBuilder:          pr.Builder{},
		Tmux:                   tmux.NewClient(runner),
		SplitDirection:         cfg.Tmux.SplitDirection,
		ClaudeCommand:          cfg.Agents.Claude.Command,
		CodexCommand:           cfg.Agents.Codex.Command,
		PromptTemplate:         cfg.Planning.PromptTemplate,
		TmuxLayout:             cfg.Tmux.Layout,
		ControlWidth:           cfg.Tmux.ControlPaneWidth,
		MinPaneWidth:           cfg.Tmux.MinPaneWidth,
		MaxPaneWidth:           cfg.Tmux.MaxPaneWidth,
		ChaosMaxParallel:       cfg.Execution.ChaosMaxParallel,
		ExecutionPlanPrompt:    cfg.Execution.Prompts.Plan,
		ExecutionSelfRunPrompt: cfg.Execution.Prompts.SelfRun,
		ExecutionChaosPrompt:   cfg.Execution.Prompts.Chaos,
	})
	return svc, cfg, nil
}

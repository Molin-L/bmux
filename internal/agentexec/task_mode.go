package agentexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/beads"
	"github.com/Molin-L/bmux/internal/model"
)

var currentExecutablePath = os.Executable

func (e *Executor) StartPlanningPane(ctx context.Context, agent string) (string, string, error) {
	if e.tmux == nil {
		return "", "", errors.New("tmux integration is not configured")
	}
	agent = strings.ToLower(strings.TrimSpace(agent))
	cmd, source, err := e.resolveAgentCommand(agent)
	if err != nil {
		return "", "", err
	}

	paneID, err := e.createPlanningPane(ctx)
	if err != nil {
		return "", "", err
	}
	var details string
	switch agent {
	case "codex":
		details, err = e.startCodexAgent(ctx, paneID, cmd)
		if err != nil {
			return "", "", err
		}
	default:
		if err := e.tmux.SendKeys(ctx, paneID, cmd, true); err != nil {
			return "", "", err
		}
		if err := e.tmux.SendKeys(ctx, paneID, e.promptTemplate, true); err != nil {
			return "", "", err
		}
		details = "Template prompt sent."
	}
	trailing := "Press c to capture plan JSON."
	if agent == "codex" {
		trailing = "Codex will create bd issues directly in this pane."
	}
	status := fmt.Sprintf("Planning pane %s started with %s (%s). %s %s", paneID, agent, source, details, trailing)
	return paneID, status, nil
}

func (e *Executor) StartTaskMode(ctx context.Context, issueID string, mode model.RunMode) (model.TaskRunMeta, error) {
	return e.startTaskModeInternal(ctx, issueID, mode, "", false, "")
}

func (e *Executor) StartTaskModeWaiting(ctx context.Context, issueID string, mode model.RunMode, blockerID string) (model.TaskRunMeta, error) {
	return e.startTaskModeInternal(ctx, issueID, mode, "", true, blockerID)
}

func (e *Executor) startTaskModeInternal(ctx context.Context, issueID string, mode model.RunMode, apeSessionID string, pending bool, blockerID string) (model.TaskRunMeta, error) {
	if mode != model.RunModePlan && mode != model.RunModeSelfRun && !isApeMode(mode) {
		return model.TaskRunMeta{}, fmt.Errorf("unsupported run mode: %s", mode)
	}
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return model.TaskRunMeta{}, errors.New("issue id is required")
	}
	if e.tmux == nil {
		return model.TaskRunMeta{}, errors.New("tmux integration is not configured")
	}

	_ = e.ReconcileRunLocks(ctx)
	if existing, ok, err := e.store.RunLockByIssueID(issueID); err == nil && ok {
		paneID := strings.TrimSpace(existing.PaneID)
		if paneID == "" {
			paneID = "(unknown)"
		}
		return model.TaskRunMeta{}, fmt.Errorf("task already running in pane %s", paneID)
	}

	if pending {
		return e.startWaitingTask(ctx, issueID, mode, apeSessionID, blockerID)
	}
	claimBeforeStart := mode != model.RunModeApe
	return e.startRunnableTask(ctx, issueID, mode, apeSessionID, "", claimBeforeStart)
}

func (e *Executor) startRunnableTask(ctx context.Context, issueID string, mode model.RunMode, apeSessionID, paneID string, claimBeforeStart bool) (model.TaskRunMeta, error) {
	if claimBeforeStart {
		if err := e.claimIssue(ctx, issueID); err != nil {
			return model.TaskRunMeta{}, err
		}
	}

	if e.openTask == nil {
		return model.TaskRunMeta{}, errors.New("open task callback is not configured")
	}
	taskMeta, err := e.openTask(ctx, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	issue, err := e.beads.Show(ctx, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	deps, _ := e.beads.Dependencies(ctx, issueID)
	issue.Dependencies = deps

	prompt := e.renderModePrompt(mode, issue, taskMeta)
	usePaneID := strings.TrimSpace(paneID)
	createdPane := false
	runLockPersisted := false
	if usePaneID == "" {
		usePaneID, err = e.createPane(ctx, taskMeta.WorktreePath)
		if err != nil {
			return model.TaskRunMeta{}, err
		}
		createdPane = true
	}
	title := buildTaskPaneTitle(issueID, issue.Title)
	if err := e.tmux.SetPaneTitle(ctx, usePaneID, title); err != nil {
		if createdPane {
			_ = e.tmux.KillPane(ctx, usePaneID)
		}
		return model.TaskRunMeta{}, fmt.Errorf("failed to set pane title %q on %s: %w", title, usePaneID, err)
	}
	cmd, _, err := e.resolveAgentCommand("codex")
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	launchCmd, err := e.buildCodexModeCommand(cmd, prompt, mode, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	if err := e.tmux.SendKeys(ctx, usePaneID, launchCmd, true); err != nil {
		if createdPane {
			_ = e.tmux.KillPane(ctx, usePaneID)
		}
		return model.TaskRunMeta{}, err
	}

	if mode == model.RunModeApe {
		if err := e.waitForPaneCommand(ctx, usePaneID, "codex", 5*time.Second); err != nil {
			_ = e.tmux.SendKeys(ctx, usePaneID, "C-c", false)
			if createdPane {
				_ = e.tmux.KillPane(ctx, usePaneID)
			}
			return model.TaskRunMeta{}, err
		}
	} else {
		_ = e.waitForPaneCommand(ctx, usePaneID, "codex", 5*time.Second)
	}

	content, _ := e.tmux.CapturePane(ctx, usePaneID, 100)
	if hasTrustPrompt(content) {
		_ = e.tmux.SendKeys(ctx, usePaneID, "", true)
		time.Sleep(200 * time.Millisecond)
		again, _ := e.tmux.CapturePane(ctx, usePaneID, 100)
		if hasTrustPrompt(again) {
			_ = e.tmux.SendKeys(ctx, usePaneID, "y", true)
			time.Sleep(200 * time.Millisecond)
		}
	}

	now := time.Now().UTC()
	run := model.TaskRunMeta{
		IssueID:         issueID,
		Mode:            mode,
		Agent:           "codex",
		PaneID:          usePaneID,
		StartedAt:       now,
		UpdatedAt:       now,
		ExpectedProcess: "codex",
		ApeSessionID:    apeSessionID,
	}
	if err := e.store.RunLockUpsert(run); err != nil {
		if createdPane {
			_ = e.tmux.KillPane(ctx, usePaneID)
		}
		return model.TaskRunMeta{}, err
	}
	runLockPersisted = true

	if !claimBeforeStart {
		if err := e.claimIssue(ctx, issueID); err != nil {
			_ = e.tmux.SendKeys(ctx, usePaneID, "C-c", false)
			if runLockPersisted {
				_ = e.store.RunLockDelete(issueID)
			}
			if createdPane {
				_ = e.tmux.KillPane(ctx, usePaneID)
			}
			return model.TaskRunMeta{}, err
		}
	}

	return run, nil
}

func (e *Executor) claimIssue(ctx context.Context, issueID string) error {
	if err := e.beads.Claim(ctx, issueID); err != nil {
		claimedBy, ok := beads.ClaimedByFromError(err)
		if !ok {
			return err
		}
		actor, actorErr := beads.ResolveActor(ctx, e.repoRoot)
		if actorErr != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(claimedBy), strings.TrimSpace(actor)) {
			return nil
		}
		return err
	}
	return nil
}

func (e *Executor) startWaitingTask(ctx context.Context, issueID string, mode model.RunMode, apeSessionID, blockerID string) (model.TaskRunMeta, error) {
	blockerID = strings.TrimSpace(blockerID)
	if blockerID == "" {
		return model.TaskRunMeta{}, errors.New("blocker issue id is required for waiting start")
	}

	if e.openTask == nil {
		return model.TaskRunMeta{}, errors.New("open task callback is not configured")
	}
	taskMeta, err := e.openTask(ctx, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	issue, err := e.beads.Show(ctx, issueID)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	paneID, err := e.createPane(ctx, taskMeta.WorktreePath)
	if err != nil {
		return model.TaskRunMeta{}, err
	}
	title := buildTaskPaneTitle(issueID, issue.Title)
	if err := e.tmux.SetPaneTitle(ctx, paneID, title); err != nil {
		_ = e.tmux.KillPane(ctx, paneID)
		return model.TaskRunMeta{}, fmt.Errorf("failed to set pane title %q on %s: %w", title, paneID, err)
	}
	waitCmd := buildWaitingPaneCommand(blockerID, issueID, taskMeta.Branch)
	if err := e.tmux.SendKeys(ctx, paneID, waitCmd, true); err != nil {
		return model.TaskRunMeta{}, err
	}

	now := time.Now().UTC()
	run := model.TaskRunMeta{
		IssueID:          issueID,
		Mode:             mode,
		Agent:            "codex",
		PaneID:           paneID,
		Pending:          true,
		BlockedByIssueID: blockerID,
		WaitStartedAt:    now,
		StartedAt:        now,
		UpdatedAt:        now,
		ApeSessionID:     apeSessionID,
	}
	if err := e.store.RunLockUpsert(run); err != nil {
		return model.TaskRunMeta{}, err
	}
	return run, nil
}

func (e *Executor) renderModePrompt(mode model.RunMode, issue model.Issue, taskMeta model.TaskBranchMeta) string {
	template := e.executionPrompts.selfRun
	switch mode {
	case model.RunModePlan:
		template = e.executionPrompts.plan
	case model.RunModeApe:
		template = e.executionPrompts.ape
	}
	replacer := strings.NewReplacer(
		"{{issue_id}}", issue.ID,
		"{{issue_title}}", issue.Title,
		"{{issue_description}}", issue.Description,
		"{{issue_status}}", issue.Status,
		"{{branch}}", taskMeta.Branch,
		"{{worktree_path}}", taskMeta.WorktreePath,
		"{{base_branch}}", taskMeta.BaseBranch,
		"{{base_commit}}", taskMeta.BaseCommit,
	)
	return strings.TrimSpace(replacer.Replace(template))
}

func buildWaitingPaneCommand(blockerID, issueID, branch string) string {
	execPath := "bmux"
	if path, err := currentExecutablePath(); err == nil && strings.TrimSpace(path) != "" {
		execPath = path
	}
	cmd := []string{
		shellQuote(execPath),
		"--wait-blocked",
		"--issue-id", shellQuote(issueID),
		"--blocked-by", shellQuote(blockerID),
	}
	if strings.TrimSpace(branch) != "" {
		cmd = append(cmd, "--branch", shellQuote(branch))
	}
	return strings.Join(cmd, " ")
}

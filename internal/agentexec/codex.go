package agentexec

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/promptx"
)

var lookPath = exec.LookPath

func (e *Executor) startCodexAgent(ctx context.Context, paneID, cmd string) (string, error) {
	statuses := []string{"Waiting for Codex readiness..."}
	launchCmd, err := e.buildCodexPlanningCommand(cmd, e.promptTemplate)
	if err != nil {
		return "", err
	}
	if err := e.tmux.SendKeys(ctx, paneID, launchCmd, true); err != nil {
		return "", err
	}
	statuses = append(statuses, "Started in planning policy (workspace-write/on-request). Direct bd-creation prompt sent as startup argument.")
	_ = e.waitForPaneCommand(ctx, paneID, "codex", 5*time.Second)

	content, _ := e.tmux.CapturePane(ctx, paneID, 100)
	if hasTrustPrompt(content) {
		statuses = append(statuses, "Trust prompt detected, auto-confirming...")
		_ = e.tmux.SendKeys(ctx, paneID, "", true)
		time.Sleep(200 * time.Millisecond)
		again, _ := e.tmux.CapturePane(ctx, paneID, 100)
		if hasTrustPrompt(again) {
			_ = e.tmux.SendKeys(ctx, paneID, "y", true)
			time.Sleep(200 * time.Millisecond)
		}
	}
	return strings.Join(statuses, " "), nil
}

func (e *Executor) resolveAgentCommand(agent string) (string, string, error) {
	if cmd := strings.TrimSpace(e.agentCommands[agent]); cmd != "" {
		return cmd, "config command", nil
	}
	if _, err := lookPath(agent); err == nil {
		return agent, "PATH fallback", nil
	}
	return "", "", fmt.Errorf("agent command is not configured and '%s' is not in PATH (set agents.%s.command)", agent, agent)
}

func (e *Executor) waitForPaneCommand(ctx context.Context, paneID, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current, err := e.tmux.GetPaneCurrentCommand(ctx, paneID)
		if err == nil && strings.EqualFold(strings.TrimSpace(current), strings.TrimSpace(expected)) {
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return errors.New("timed out waiting for codex startup")
}

func hasTrustPrompt(content string) bool {
	lower := strings.ToLower(content)
	patterns := []string{
		"do you trust",
		"trust this workspace",
		"trust this folder",
		"trust this directory",
		"add this directory to trusted",
		"add to trusted",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (e *Executor) buildCodexPlanningCommand(baseCmd, prompt string) (string, error) {
	baseCmd = strings.TrimSpace(baseCmd)
	if baseCmd == "" {
		return "", errors.New("codex command is empty")
	}
	cmd := baseCmd
	if !hasCLIFlag(cmd, "--sandbox") {
		cmd += " --sandbox workspace-write"
	}
	if !hasCLIFlag(cmd, "--ask-for-approval") {
		cmd += " --ask-for-approval on-request"
	}
	if strings.TrimSpace(prompt) != "" {
		cmd += " " + shellQuote(prompt)
	}
	return cmd, nil
}

func (e *Executor) buildCodexModeCommand(baseCmd, prompt string, mode model.RunMode, issueID string) (string, error) {
	baseCmd = strings.TrimSpace(baseCmd)
	if baseCmd == "" {
		return "", errors.New("codex command is empty")
	}

	cmd := baseCmd
	switch mode {
	case model.RunModePlan:
		if !hasCLIFlag(cmd, "--sandbox") {
			cmd += " --sandbox workspace-write"
		}
		if !hasCLIFlag(cmd, "--ask-for-approval") {
			cmd += " --ask-for-approval on-request"
		}
	case model.RunModeSelfRun, model.RunModeApe:
		if !hasCLIFlag(cmd, "--dangerously-bypass-approvals-and-sandbox") {
			cmd += " --dangerously-bypass-approvals-and-sandbox"
		}
	default:
		return "", fmt.Errorf("unsupported run mode: %s", mode)
	}

	promptArg := shellQuote(prompt)
	if promptPath, err := promptx.WritePromptFile(e.repoRoot, issueID+"-"+string(mode), prompt); err == nil {
		snippet := promptx.BuildReadAndDeleteSnippet(promptPath)
		return fmt.Sprintf(`%s; %s "%s"`, snippet, cmd, "$BMUX_PROMPT_CONTENT"), nil
	}
	return cmd + " " + promptArg, nil
}

func hasCLIFlag(cmd, flag string) bool {
	return strings.Contains(cmd, flag+" ") || strings.Contains(cmd, flag+"=") || strings.HasSuffix(strings.TrimSpace(cmd), flag)
}

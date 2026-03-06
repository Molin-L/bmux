package agentexec

import (
	"context"
	"strings"

	"github.com/Molin-L/bmux/internal/layout"
)

func (e *Executor) createPlanningPane(ctx context.Context) (string, error) {
	return e.createPane(ctx, e.repoRoot)
}

func (e *Executor) createPane(ctx context.Context, cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		cwd = e.repoRoot
	}
	if e.tmuxLayout != "sidebar" {
		return e.tmux.SplitPane(ctx, e.splitDirection, cwd)
	}
	currentPane, err := e.tmux.CurrentPaneID(ctx)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(currentPane)
	panes, err := e.tmux.ListPanes(ctx, "")
	if err == nil {
		for _, pane := range panes {
			p := strings.TrimSpace(pane)
			if p != "" && p != currentPane {
				title, titleErr := e.tmux.GetPaneTitle(ctx, p)
				if titleErr == nil && (title == layout.SpacerPaneTitle || title == layout.IdlePaneTitle) {
					continue
				}
				target = p
			}
		}
	}
	paneID, err := e.tmux.SplitPaneOnTargetWithCommand(ctx, "right", cwd, target, "")
	if err != nil {
		return "", err
	}
	if err := e.recalculateSidebarLayout(ctx, "", true); err != nil {
		_ = e.tmux.KillPane(ctx, paneID)
		return "", err
	}
	return paneID, nil
}

func (e *Executor) recalculateSidebarLayout(ctx context.Context, controlPaneID string, force bool) error {
	if e.layoutManager == nil || e.tmuxLayout != "sidebar" {
		return nil
	}
	if strings.TrimSpace(controlPaneID) == "" {
		paneID, err := e.resolveControlPaneID(ctx)
		if err != nil {
			return err
		}
		controlPaneID = paneID
	}
	if strings.TrimSpace(controlPaneID) == "" {
		return nil
	}
	return e.layoutManager.Recalculate(ctx, controlPaneID, force)
}

func (e *Executor) resolveControlPaneID(ctx context.Context) (string, error) {
	panes, err := e.tmux.ListPanes(ctx, "")
	if err != nil {
		return "", err
	}
	ordered := make([]string, 0, len(panes))
	for _, paneID := range panes {
		if candidate := strings.TrimSpace(paneID); candidate != "" {
			ordered = append(ordered, candidate)
		}
	}
	if len(ordered) == 0 {
		return "", nil
	}

	isAuxiliaryPane := func(paneID string) bool {
		title, titleErr := e.tmux.GetPaneTitle(ctx, paneID)
		if titleErr != nil {
			return false
		}
		switch strings.TrimSpace(title) {
		case layout.SpacerPaneTitle, layout.IdlePaneTitle:
			return true
		default:
			return false
		}
	}

	for _, candidate := range ordered {
		cmd, cmdErr := e.tmux.GetPaneCurrentCommand(ctx, candidate)
		if cmdErr != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(cmd), "bmux") {
			return candidate, nil
		}
	}

	currentPane, currentErr := e.tmux.CurrentPaneID(ctx)
	if currentErr == nil {
		currentPane = strings.TrimSpace(currentPane)
		knownCurrent := false
		for _, candidate := range ordered {
			if candidate == currentPane {
				knownCurrent = true
				break
			}
		}
		if knownCurrent && !isAuxiliaryPane(currentPane) {
			return currentPane, nil
		}
	}

	for _, candidate := range ordered {
		if !isAuxiliaryPane(candidate) {
			return candidate, nil
		}
	}

	return ordered[0], nil
}

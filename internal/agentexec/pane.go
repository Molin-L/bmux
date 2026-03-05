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
				if titleErr == nil && title == layout.SpacerPaneTitle {
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
	_ = e.recalculateSidebarLayout(ctx, currentPane, true)
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
	if len(panes) == 0 {
		return "", nil
	}

	currentPane, currentErr := e.tmux.CurrentPaneID(ctx)
	if currentErr == nil {
		currentPane = strings.TrimSpace(currentPane)
		for _, paneID := range panes {
			if strings.TrimSpace(paneID) == currentPane {
				return currentPane, nil
			}
		}
	}

	for _, paneID := range panes {
		candidate := strings.TrimSpace(paneID)
		if candidate == "" {
			continue
		}
		cmd, cmdErr := e.tmux.GetPaneCurrentCommand(ctx, candidate)
		if cmdErr != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(cmd), "bmux") {
			return candidate, nil
		}
	}

	return strings.TrimSpace(panes[0]), nil
}

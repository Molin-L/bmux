package agentexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/model"
)

func (e *Executor) StartApe(ctx context.Context) (string, error) {
	if e.readyIssues == nil {
		return "", errors.New("ready issues callback is not configured")
	}
	issues, sourceState, err := e.readyIssues(ctx)
	if err != nil {
		return "", err
	}
	if !sourceState.Available {
		if sourceState.Reason == "" {
			return "", errors.New("ready issues source is unavailable")
		}
		return "", fmt.Errorf("ready issues source is unavailable: %s", sourceState.Reason)
	}
	if len(issues) == 0 {
		return "", errors.New("no ready issues for ape mode")
	}
	sessionID := fmt.Sprintf("ape-%d", time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	ape := model.ApeState{
		SessionID:        sessionID,
		Active:           true,
		LaunchedIssueIDs: []string{},
		FinishedIssueIDs: []string{},
		ActiveIssueIDs:   []string{},
		StartedAt:        now,
		UpdatedAt:        now,
	}

	if err := e.store.SetApeState(&ape); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (e *Executor) TickApe(ctx context.Context) (string, bool, error) {
	if e.readyIssues == nil {
		return "", false, errors.New("ready issues callback is not configured")
	}
	ape, ok, err := e.store.ApeState()
	if err != nil {
		return "", false, err
	}
	if !ok || !ape.Active {
		return "Ape idle", true, nil
	}

	if err := e.ReconcileRunLocks(ctx); err != nil {
		return "", false, err
	}

	runList, err := e.store.RunLockAll()
	if err != nil {
		return "", false, err
	}
	runs := map[string]model.TaskRunMeta{}
	for _, run := range runList {
		runs[run.IssueID] = run
	}
	launchedSet := stringsToSet(ape.LaunchedIssueIDs)
	finishedSet := stringsToSet(ape.FinishedIssueIDs)
	activeSet := stringsToSet(ape.ActiveIssueIDs)

	for issueID := range activeSet {
		run, exists := runs[issueID]
		if !exists || run.ApeSessionID != ape.SessionID {
			delete(activeSet, issueID)
			finishedSet[issueID] = struct{}{}
		}
	}
	for _, run := range runs {
		if run.ApeSessionID != ape.SessionID {
			continue
		}
		activeSet[run.IssueID] = struct{}{}
		launchedSet[run.IssueID] = struct{}{}
	}

	readyIssues, sourceState, err := e.readyIssues(ctx)
	if err != nil {
		return "", false, err
	}
	if !sourceState.Available {
		return "Ape paused: ready issues source unavailable", false, nil
	}

	issueSet := map[string]model.Issue{}
	for _, issue := range readyIssues {
		if strings.TrimSpace(issue.ID) == "" {
			continue
		}
		issueSet[issue.ID] = issue
	}

	runningCount := len(activeSet)
	capacity := e.apeMaxParallel - runningCount
	if capacity < 0 {
		capacity = 0
	}

	queued := 0
	blocked := 0
	for _, issue := range readyIssues {
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			continue
		}
		if _, done := finishedSet[issueID]; done {
			continue
		}
		if _, active := activeSet[issueID]; active {
			continue
		}

		blockers := make([]string, 0, 2)
		if parentID := strings.TrimSpace(issue.ParentID); parentID != "" {
			if _, ok := issueSet[parentID]; ok {
				blockers = append(blockers, parentID)
			}
		}
		deps, _ := e.beads.Dependencies(ctx, issueID)
		for _, dep := range deps {
			if dep.Type != "blocks" || dep.Direction != "outgoing" {
				continue
			}
			blockerID := strings.TrimSpace(dep.IssueID)
			if blockerID == "" {
				continue
			}
			if _, ok := issueSet[blockerID]; ok {
				blockers = append(blockers, blockerID)
			}
		}

		ready := true
		for _, blockerID := range dedupeStrings(blockers) {
			if _, done := finishedSet[blockerID]; !done {
				ready = false
				break
			}
		}
		if !ready {
			blocked++
			continue
		}
		if capacity <= 0 {
			queued++
			continue
		}

		if _, err := e.startTaskModeInternal(ctx, issueID, model.RunModeApe, ape.SessionID, false, ""); err != nil {
			finishedSet[issueID] = struct{}{}
			continue
		}
		launchedSet[issueID] = struct{}{}
		activeSet[issueID] = struct{}{}
		capacity--
		runningCount++
	}

	pending := 0
	for _, issue := range readyIssues {
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			continue
		}
		if _, done := finishedSet[issueID]; done {
			continue
		}
		if _, active := activeSet[issueID]; active {
			continue
		}
		pending++
	}
	done := pending == 0 && runningCount == 0
	ape.Active = !done
	ape.LaunchedIssueIDs = setToSortedSlice(launchedSet)
	ape.FinishedIssueIDs = setToSortedSlice(finishedSet)
	ape.ActiveIssueIDs = setToSortedSlice(activeSet)
	if done {
		ape.ActiveIssueIDs = []string{}
	}
	ape.UpdatedAt = time.Now().UTC()
	if err := e.store.SetApeState(&ape); err != nil {
		return "", false, err
	}
	status := fmt.Sprintf(
		"Ape %s: running=%d queued=%d blocked=%d finished=%d",
		ape.SessionID,
		runningCount,
		queued,
		blocked,
		len(finishedSet),
	)
	return status, done, nil
}

func (e *Executor) ReconcileRunLocks(ctx context.Context) error {
	runs, err := e.store.RunLockAll()
	if err != nil {
		return err
	}
	if len(runs) == 0 || e.tmux == nil {
		return nil
	}
	panes, err := e.tmux.ListPanes(ctx, "")
	if err != nil {
		return err
	}
	paneSet := map[string]struct{}{}
	for _, pane := range panes {
		if v := strings.TrimSpace(pane); v != "" {
			paneSet[v] = struct{}{}
		}
	}

	for _, run := range runs {
		remove := false
		if strings.TrimSpace(run.PaneID) == "" {
			remove = true
		} else if _, ok := paneSet[run.PaneID]; !ok {
			remove = true
		} else if strings.TrimSpace(run.ExpectedProcess) != "" {
			currentCmd, cmdErr := e.tmux.GetPaneCurrentCommand(ctx, run.PaneID)
			if cmdErr == nil && !strings.EqualFold(strings.TrimSpace(currentCmd), strings.TrimSpace(run.ExpectedProcess)) {
				remove = true
			}
		}
		if remove {
			if err := e.store.RunLockDelete(run.IssueID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) PromotePendingRuns(ctx context.Context) (int, int, error) {
	runs, err := e.store.RunLockAll()
	if err != nil {
		return 0, 0, err
	}
	if len(runs) == 0 || e.tmux == nil {
		return 0, 0, nil
	}

	promoted := 0
	stillWaiting := 0
	for _, run := range runs {
		if !run.Pending {
			continue
		}
		stillWaiting++
		blockerID := strings.TrimSpace(run.BlockedByIssueID)
		if blockerID == "" {
			continue
		}
		blocker, showErr := e.beads.Show(ctx, blockerID)
		if showErr != nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(blocker.Status), "closed") {
			continue
		}

		issueID := strings.TrimSpace(run.IssueID)
		paneID := strings.TrimSpace(run.PaneID)
		if issueID == "" || paneID == "" {
			continue
		}
		_ = e.tmux.SendKeys(ctx, paneID, "C-c", false)
		time.Sleep(120 * time.Millisecond)
		if _, startErr := e.startRunnableTask(ctx, issueID, run.Mode, run.ApeSessionID, paneID, true); startErr != nil {
			return promoted, stillWaiting, startErr
		}
		promoted++
		stillWaiting--
	}
	return promoted, stillWaiting, nil
}

func (e *Executor) ReconcileRuns(ctx context.Context) error {
	if err := e.ReconcileRunLocks(ctx); err != nil {
		return err
	}
	if _, _, err := e.PromotePendingRuns(ctx); err != nil {
		return err
	}
	_ = e.recalculateSidebarLayout(ctx, "", false)
	return nil
}

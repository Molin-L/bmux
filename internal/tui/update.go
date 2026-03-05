package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Molin-L/bmux/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshTaskViewport(false)
		return m, nil
	case issuesLoadedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Failed to load issues: %v", msg.err)
			return m, nil
		}
		prevIssueID, hadPrevSelection := m.selectedIssueID()
		m.issueSourceAvailable = msg.state.Available
		m.issueSourceReason = msg.state.Reason
		m.issues = msg.issues
		m.rows = buildIssueRows(msg.issues)
		m.pruneSelectedTaskIssueIDs()
		if len(m.issues) == 0 {
			m.selected = 0
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			m.status = "No ready issues. Press r to refresh."
			return m, nil
		}
		if hadPrevSelection {
			m.selected = rowIndexByIssueID(m.rows, prevIssueID)
		}
		if m.selected < 0 || m.selected >= len(m.rows) || m.rows[m.selected].kind != issueRowIssue {
			m.selected = firstSelectableRow(m.rows)
		}
		if m.selected < 0 {
			m.selected = 0
		}
		m.status = fmt.Sprintf("Loaded %d ready issues", len(m.issues))
		m.refreshTaskViewport(true)
		return m, m.loadBlockedByCmd()
	case blockedByLoadedMsg:
		if msg.err == nil {
			m.blockedBy = msg.blockedBy
		}
		m.refreshTaskViewport(false)
		return m, nil
	case runsReconciledMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Run reconciliation failed: %v", msg.err)
		} else if strings.TrimSpace(msg.status) != "" {
			m.status = msg.status
		}
		m.refreshTaskViewport(false)
		return m, tea.Batch(m.reconcileRunsCmd(), m.loadBlockedByCmd())
	case batchLaunchResultMsg:
		m.busy = false
		m.batchActive = false
		started := len(msg.started)
		waiting := len(msg.waiting)
		failed := len(msg.failed)
		m.status = fmt.Sprintf("Batch %s launch complete: started=%d waiting=%d failed=%d", modeLabel(msg.mode), started, waiting, failed)
		if failed > 0 {
			for issueID, reason := range msg.failed {
				m.status = fmt.Sprintf("%s | %s: %s", m.status, issueID, reason)
				break
			}
		}
		if started+waiting == 0 && failed == 0 {
			m.status = fmt.Sprintf("Batch %s launch complete: no tasks launched", modeLabel(msg.mode))
		}
		m.refreshTaskViewport(false)
		return m, nil
	case actionResultMsg:
		m.busy = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Action failed: %v", msg.err)
			if m.mode == modeAgentSelect && m.selectedAgent != "" {
				m.errorHint = fmt.Sprintf("Configure .bmux/config.yaml -> agents.%s.command or install '%s' in PATH.", m.selectedAgent, m.selectedAgent)
			}
			return m, nil
		}
		if msg.status != "" {
			m.status = msg.status
		}
		m.errorHint = ""
		if strings.Contains(strings.ToLower(m.status), "not verified") {
			m.errorHint = "Codex prompt delivery was not verified. Ask Codex to continue with the planning template, then press c."
		}
		m.prompt = msg.prompt
		if strings.TrimSpace(msg.paneID) != "" {
			m.pendingPaneID = strings.TrimSpace(msg.paneID)
			m.mode = modeMain
		}
		return m, nil
	case planCapturedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Capture failed: %v", msg.err)
			m.errorHint = "ask agent to reprint block with markers"
			return m, nil
		}
		m.extractedPlan = msg.plan
		m.mode = modePlanConfirm
		m.status = "Plan captured. Press Enter to create epic/task/subtasks or Esc to cancel."
		m.errorHint = ""
		return m, nil
	case hierarchyCreatedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Create hierarchy failed: %v", msg.err)
			return m, nil
		}
		m.mode = modeMain
		m.status = fmt.Sprintf("Created epic %s, task %s, subtasks=%d", msg.result.EpicID, msg.result.TaskID, len(msg.result.SubtaskIDs))
		return m, nil
	case spinnerTickMsg:
		if len(spinnerFrames) > 0 {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		}
		m.refreshTaskViewport(false)
		return m, m.spinnerTickCmd()
	case tea.KeyMsg:
		if m.mode == modeAgentSelect {
			switch msg.String() {
			case "esc":
				m.mode = modeMain
				m.status = "Create planning pane canceled."
				m.errorHint = ""
				return m, nil
			case "up", "k":
				if m.agentSelected > 0 {
					m.agentSelected--
				}
				return m, nil
			case "down", "j":
				if m.agentSelected < len(m.agentOptions)-1 {
					m.agentSelected++
				}
				return m, nil
			case "enter":
				if m.busy {
					return m, nil
				}
				m.busy = true
				agent := m.agentOptions[m.agentSelected]
				m.selectedAgent = agent
				return m, func() tea.Msg {
					paneID, status, err := m.svc.StartPlanningPane(context.Background(), agent)
					if err != nil {
						return actionResultMsg{err: err}
					}
					return actionResultMsg{status: status, paneID: paneID}
				}
			}
		}
		if m.mode == modePlanConfirm {
			switch msg.String() {
			case "esc":
				m.mode = modeMain
				m.status = "Hierarchy creation canceled."
				return m, nil
			case "enter":
				if m.busy {
					return m, nil
				}
				m.busy = true
				plan := m.extractedPlan
				return m, func() tea.Msg {
					res, err := m.svc.CreateHierarchyFromPlan(context.Background(), plan)
					return hierarchyCreatedMsg{result: res, err: err}
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			next := nextSelectableRow(m.rows, m.selected, -1)
			if next >= 0 {
				m.selected = next
				m.refreshTaskViewport(true)
			}
			return m, nil
		case "down", "j":
			next := nextSelectableRow(m.rows, m.selected, 1)
			if next >= 0 {
				m.selected = next
				m.refreshTaskViewport(true)
			}
			return m, nil
		case "pgup":
			m.scrollViewportByPage(-1)
			return m, nil
		case "pgdown":
			m.scrollViewportByPage(1)
			return m, nil
		case "home":
			m.taskViewportYOffset = 0
			return m, nil
		case "end":
			m.taskViewportYOffset = m.maxViewportOffset()
			return m, nil
		case " ":
			issueID, ok := m.selectedIssueID()
			if !ok {
				return m, nil
			}
			if m.selectedTaskIssueIDs == nil {
				m.selectedTaskIssueIDs = map[string]struct{}{}
			}
			if _, selected := m.selectedTaskIssueIDs[issueID]; selected {
				delete(m.selectedTaskIssueIDs, issueID)
				m.status = fmt.Sprintf("Selected %d task(s).", len(m.selectedTaskIssueIDs))
				m.refreshTaskViewport(false)
				return m, nil
			}
			m.selectedTaskIssueIDs[issueID] = struct{}{}
			m.status = fmt.Sprintf("Selected %d task(s).", len(m.selectedTaskIssueIDs))
			m.refreshTaskViewport(false)
			return m, nil
		case "shift+tab":
			if len(m.taskModeOptions) == 0 {
				return m, nil
			}
			if m.taskModeSelected > 0 {
				m.taskModeSelected--
			} else {
				m.taskModeSelected = len(m.taskModeOptions) - 1
			}
			m.status = fmt.Sprintf("Current mode: %s", modeLabel(m.taskModeOptions[m.taskModeSelected]))
			return m, nil
		case "r":
			m.busy = true
			m.prompt = ""
			m.status = "Refreshing..."
			return m, m.loadIssuesCmd()
		case "n":
			if m.busy {
				return m, nil
			}
			m.mode = modeAgentSelect
			m.agentSelected = 0
			m.selectedAgent = ""
			m.status = "Select an agent for planning pane."
			return m, nil
		case "c":
			if m.busy {
				return m, nil
			}
			paneID := strings.TrimSpace(m.pendingPaneID)
			if paneID == "" {
				m.status = "No planning pane found. Press n first."
				return m, nil
			}
			m.busy = true
			return m, func() tea.Msg {
				plan, err := m.svc.ExtractPlanJSON(context.Background(), paneID)
				return planCapturedMsg{plan: plan, err: err}
			}
		case "enter":
			if m.busy {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			if len(m.taskModeOptions) == 0 {
				return m, nil
			}
			mode := m.taskModeOptions[m.taskModeSelected]
			m.prompt = ""
			if mode == model.RunModeApe {
				m.busy = true
				return m, func() tea.Msg {
					sessionID, err := m.svc.StartApe(context.Background())
					if err != nil {
						return actionResultMsg{err: err}
					}
					return actionResultMsg{status: fmt.Sprintf("Ape started: %s", sessionID)}
				}
			}
			if m.batchActive {
				m.status = "Batch launch in progress."
				return m, nil
			}
			selectedIDs := m.issueIDsInRowOrder(m.selectedTaskIssueIDs)
			if len(selectedIDs) == 0 {
				m.status = "No tasks selected. Press Space to select task(s)."
				return m, nil
			}
			items := m.batchLaunchItems(selectedIDs)
			m.batchActive = true
			m.busy = true
			m.status = fmt.Sprintf("Launching %d task(s)...", len(items))
			return m, m.startBatchLaunchCmd(items, mode)
		case "p":
			if m.busy {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			issueID, ok := m.selectedIssueID()
			if !ok {
				return m, nil
			}
			m.busy = true
			return m, func() tea.Msg {
				prompt, err := m.svc.GeneratePRPrompt(context.Background(), issueID)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Generated PR prompt for %s", issueID), prompt: prompt}
			}
		case "m":
			if m.busy {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			issueID, ok := m.selectedIssueID()
			if !ok {
				return m, nil
			}
			m.busy = true
			return m, func() tea.Msg {
				meta, err := m.svc.MergeTask(context.Background(), issueID, true)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Merged %s and cleaned branch %s", issueID, meta.Branch)}
			}
		case "x":
			if m.busy {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			issueID, ok := m.selectedIssueID()
			if !ok {
				return m, nil
			}
			m.busy = true
			return m, func() tea.Msg {
				meta, err := m.svc.CleanupTask(context.Background(), issueID)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Cleaned task %s (%s)", issueID, meta.Branch)}
			}
		}
	}
	return m, nil
}

func (m Model) loadIssuesCmd() tea.Cmd {
	return func() tea.Msg {
		issues, state, err := m.svc.ReadyIssuesState(context.Background())
		return issuesLoadedMsg{issues: issues, state: state, err: err}
	}
}

func (m Model) reconcileRunsCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(time.Time) tea.Msg {
		if m.svc == nil {
			return runsReconciledMsg{}
		}
		if err := m.svc.ReconcileRuns(context.Background()); err != nil {
			return runsReconciledMsg{err: err}
		}
		status, done, err := m.svc.TickApe(context.Background())
		if err != nil {
			return runsReconciledMsg{err: err}
		}
		if done {
			return runsReconciledMsg{}
		}
		return runsReconciledMsg{status: status}
	})
}

func (m Model) loadBlockedByCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		if m.svc == nil || len(m.issues) == 0 {
			return blockedByLoadedMsg{blockedBy: map[string]string{}}
		}
		blockedBy, err := m.svc.ComputeBlockedBy(context.Background(), m.issues)
		return blockedByLoadedMsg{blockedBy: blockedBy, err: err}
	})
}

func (m Model) spinnerTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func unavailableIssuesStatus(reason string) string {
	if reason == "" {
		return "No ready issues. Issue source is unavailable."
	}
	return fmt.Sprintf("No ready issues. Issue source unavailable: %s.", reason)
}

func (m Model) selectedIssueID() (string, bool) {
	issue, ok := selectedIssueFromRows(m.rows, m.selected)
	if !ok {
		return "", false
	}
	return issue.ID, true
}

func isCompactViewportWidth(width int) bool {
	return width < compactViewportThreshold
}

func compactDetailsHeight(taskAreaHeight int) int {
	height := taskAreaHeight / 4
	if height < compactDetailsMinLines {
		height = compactDetailsMinLines
	}
	if height > compactDetailsMaxLines {
		height = compactDetailsMaxLines
	}
	return height
}

func (m *Model) taskViewportLayout() (int, int, int) {
	width := m.taskViewportWidth
	if m.width > 0 {
		width = m.width - panelStyle.GetHorizontalFrameSize()
	}
	if width <= 0 {
		width = 96
	}
	if width < taskViewportMinWidth {
		width = taskViewportMinWidth
	}

	height := m.taskViewportHeight + m.taskDetailsHeight
	if height <= 0 {
		height = m.taskViewportHeight
	}
	if m.height > 0 {
		reserved := 6
		if m.busy {
			reserved++
		}
		if m.status != "" {
			reserved++
		}
		if m.pendingPaneID != "" {
			reserved++
		}
		if m.errorHint != "" {
			reserved++
		}
		if m.prompt != "" {
			reserved += 6
		}
		if m.mode == modeAgentSelect || m.mode == modePlanConfirm {
			reserved += 5
		}
		height = m.height - reserved - panelStyle.GetVerticalFrameSize()
	}
	if height <= 0 {
		height = 18
	}

	detailsHeight := 0
	if isCompactViewportWidth(width) {
		detailsHeight = compactDetailsHeight(height)
		if height-detailsHeight < 6 {
			detailsHeight = max(0, height-6)
		}
		height -= detailsHeight
	}
	if height < 6 {
		height = 6
	}
	return width, height, detailsHeight
}

func (m *Model) taskViewportSize() (int, int) {
	width, height, _ := m.taskViewportLayout()
	return width, height
}

func (m *Model) refreshTaskViewport(ensureSelection bool) {
	width, height, detailsHeight := m.taskViewportLayout()
	m.taskViewportWidth = width
	m.taskViewportHeight = height
	m.taskDetailsHeight = detailsHeight
	render := m.renderTasksContent(width)
	maxOffset := max(0, render.lineCount-height)
	if ensureSelection && m.selected >= 0 && m.selected < len(render.rowStarts) {
		start := render.rowStarts[m.selected]
		end := render.rowEnds[m.selected]
		if start >= 0 {
			if start < m.taskViewportYOffset {
				m.taskViewportYOffset = start
			}
			bottom := m.taskViewportYOffset + height - 1
			if end > bottom {
				m.taskViewportYOffset = end - height + 1
			}
		}
	}
	if m.taskViewportYOffset < 0 {
		m.taskViewportYOffset = 0
	}
	if m.taskViewportYOffset > maxOffset {
		m.taskViewportYOffset = maxOffset
	}
}

func (m *Model) maxViewportOffset() int {
	width, height := m.taskViewportSize()
	render := m.renderTasksContent(width)
	return max(0, render.lineCount-height)
}

func (m *Model) scrollViewportByPage(direction int) {
	_, height := m.taskViewportSize()
	if height < 2 {
		height = 2
	}
	step := height - 1
	if direction < 0 {
		m.taskViewportYOffset -= step
	} else {
		m.taskViewportYOffset += step
	}
	maxOffset := m.maxViewportOffset()
	if m.taskViewportYOffset < 0 {
		m.taskViewportYOffset = 0
	}
	if m.taskViewportYOffset > maxOffset {
		m.taskViewportYOffset = maxOffset
	}
}

func (m *Model) pruneSelectedTaskIssueIDs() {
	if len(m.selectedTaskIssueIDs) == 0 {
		return
	}
	ready := m.readyIssueIDSet()
	for issueID := range m.selectedTaskIssueIDs {
		if _, ok := ready[issueID]; !ok {
			delete(m.selectedTaskIssueIDs, issueID)
		}
	}
}

func (m *Model) readyIssueIDSet() map[string]struct{} {
	out := map[string]struct{}{}
	for _, row := range m.rows {
		if row.kind != issueRowIssue {
			continue
		}
		issueID := strings.TrimSpace(row.issue.ID)
		if issueID == "" {
			continue
		}
		out[issueID] = struct{}{}
	}
	return out
}

func (m *Model) issueIDsInRowOrder(set map[string]struct{}) []string {
	if len(set) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(set))
	seen := map[string]struct{}{}
	for _, row := range m.rows {
		if row.kind != issueRowIssue {
			continue
		}
		issueID := strings.TrimSpace(row.issue.ID)
		if issueID == "" {
			continue
		}
		if _, ok := set[issueID]; !ok {
			continue
		}
		out = append(out, issueID)
		seen[issueID] = struct{}{}
	}
	extra := make([]string, 0, len(set)-len(seen))
	for issueID := range set {
		if _, ok := seen[issueID]; ok {
			continue
		}
		if strings.TrimSpace(issueID) == "" {
			continue
		}
		extra = append(extra, issueID)
	}
	sort.Strings(extra)
	out = append(out, extra...)
	return out
}

type batchLaunchItem struct {
	issueID   string
	blockerID string
}

func (m *Model) batchLaunchItems(issueIDs []string) []batchLaunchItem {
	items := make([]batchLaunchItem, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		id := strings.TrimSpace(issueID)
		if id == "" {
			continue
		}
		items = append(items, batchLaunchItem{
			issueID:   id,
			blockerID: strings.TrimSpace(m.blockedBy[id]),
		})
	}
	return items
}

func (m Model) startBatchLaunchCmd(items []batchLaunchItem, mode model.RunMode) tea.Cmd {
	launchItems := append([]batchLaunchItem(nil), items...)
	return func() tea.Msg {
		started := map[string]string{}
		waiting := map[string]string{}
		failed := map[string]string{}
		if m.svc == nil {
			for _, item := range launchItems {
				failed[item.issueID] = "service is not configured"
			}
			return batchLaunchResultMsg{mode: mode, started: started, waiting: waiting, failed: failed}
		}
		for _, item := range launchItems {
			var (
				run model.TaskRunMeta
				err error
			)
			if item.blockerID != "" {
				run, err = m.svc.StartTaskModeWaiting(context.Background(), item.issueID, mode, item.blockerID)
			} else {
				run, err = m.svc.StartTaskMode(context.Background(), item.issueID, mode)
			}
			if err != nil {
				failed[item.issueID] = err.Error()
				continue
			}
			if run.Pending {
				waiting[item.issueID] = strings.TrimSpace(run.PaneID)
				continue
			}
			started[item.issueID] = strings.TrimSpace(run.PaneID)
		}
		return batchLaunchResultMsg{mode: mode, started: started, waiting: waiting, failed: failed}
	}
}

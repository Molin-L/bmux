package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			}
			return m, nil
		case "down", "j":
			next := nextSelectableRow(m.rows, m.selected, 1)
			if next >= 0 {
				m.selected = next
			}
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
			issueID, ok := m.selectedIssueID()
			if !ok {
				return m, nil
			}
			m.busy = true
			m.prompt = ""
			return m, func() tea.Msg {
				meta, err := m.svc.OpenTask(context.Background(), issueID)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Task %s mapped to %s", issueID, meta.Branch)}
			}
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

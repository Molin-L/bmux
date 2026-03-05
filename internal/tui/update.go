package tui

import (
	"context"
	"fmt"

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
		m.issueSourceAvailable = msg.state.Available
		m.issueSourceReason = msg.state.Reason
		m.issues = msg.issues
		if len(m.issues) == 0 {
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			m.status = "No ready issues. Press r to refresh."
			return m, nil
		}
		if m.selected >= len(m.issues) {
			m.selected = len(m.issues) - 1
		}
		m.status = fmt.Sprintf("Loaded %d ready issues", len(m.issues))
		return m, nil
	case actionResultMsg:
		m.busy = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Action failed: %v", msg.err)
			return m, nil
		}
		if msg.status != "" {
			m.status = msg.status
		}
		m.prompt = msg.prompt
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.issues)-1 {
				m.selected++
			}
			return m, nil
		case "r":
			m.busy = true
			m.prompt = ""
			m.status = "Refreshing..."
			return m, m.loadIssuesCmd()
		case "enter":
			if m.busy || len(m.issues) == 0 {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			m.busy = true
			m.prompt = ""
			issueID := m.issues[m.selected].ID
			return m, func() tea.Msg {
				meta, err := m.svc.OpenTask(context.Background(), issueID)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Task %s mapped to %s", issueID, meta.Branch)}
			}
		case "p":
			if m.busy || len(m.issues) == 0 {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			m.busy = true
			issueID := m.issues[m.selected].ID
			return m, func() tea.Msg {
				prompt, err := m.svc.GeneratePRPrompt(context.Background(), issueID)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Generated PR prompt for %s", issueID), prompt: prompt}
			}
		case "m":
			if m.busy || len(m.issues) == 0 {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			m.busy = true
			issueID := m.issues[m.selected].ID
			return m, func() tea.Msg {
				meta, err := m.svc.MergeTask(context.Background(), issueID, true)
				if err != nil {
					return actionResultMsg{err: err}
				}
				return actionResultMsg{status: fmt.Sprintf("Merged %s and cleaned branch %s", issueID, meta.Branch)}
			}
		case "x":
			if m.busy || len(m.issues) == 0 {
				return m, nil
			}
			if !m.issueSourceAvailable {
				m.status = unavailableIssuesStatus(m.issueSourceReason)
				return m, nil
			}
			m.busy = true
			issueID := m.issues[m.selected].ID
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

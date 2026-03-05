package tui

import (
	"fmt"
	"strings"
)

func (m Model) View() string {
	var b strings.Builder

	b.WriteString("bmux - Task Branch Orchestrator\n")
	b.WriteString("keys: ↑/↓ move | Enter open/create worktree | n new planning pane | c capture plan | p PR prompt | m merge | x cleanup | r refresh | q quit\n\n")

	if len(m.issues) == 0 {
		b.WriteString("No issues loaded.\n")
	} else {
		for i, issue := range m.issues {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			branch := "(no branch)"
			if meta, ok, _ := m.svc.GetTaskMeta(issue.ID); ok {
				branch = meta.Branch
			}
			b.WriteString(fmt.Sprintf("%s[%s] P%d %-10s %s :: %s\n", prefix, issue.ID, issue.Priority, issue.Status, issue.Title, branch))
		}
	}

	if m.busy {
		b.WriteString("\nWorking...\n")
	}
	if m.status != "" {
		b.WriteString("\nStatus: " + m.status + "\n")
	}
	if m.prompt != "" {
		b.WriteString("\nPR Prompt:\n")
		b.WriteString(m.prompt)
		b.WriteString("\n")
	}
	if m.pendingPaneID != "" {
		b.WriteString("\nPlanning Pane: " + m.pendingPaneID + "\n")
	}
	if m.errorHint != "" {
		b.WriteString("Hint: " + m.errorHint + "\n")
	}
	if m.mode == modeAgentSelect {
		b.WriteString("\nSelect agent for planning pane (Enter confirm, Esc cancel):\n")
		for i, agent := range m.agentOptions {
			prefix := "  "
			if i == m.agentSelected {
				prefix = "> "
			}
			b.WriteString(prefix + agent + "\n")
		}
	}
	if m.mode == modePlanConfirm {
		b.WriteString("\nConfirm hierarchy creation (Enter create, Esc cancel):\n")
		b.WriteString(fmt.Sprintf("Epic: %s\n", m.extractedPlan.Epic.Title))
		b.WriteString(fmt.Sprintf("Task: %s\n", m.extractedPlan.Task.Title))
		b.WriteString(fmt.Sprintf("Subtasks: %d\n", len(m.extractedPlan.Subtasks)))
	}

	return b.String()
}

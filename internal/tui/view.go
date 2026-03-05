package tui

import (
	"fmt"
	"strings"
)

func (m Model) View() string {
	var b strings.Builder

	b.WriteString("bmux - Task Branch Orchestrator\n")
	b.WriteString("keys: ↑/↓ move | Enter open/create worktree | p PR prompt | m merge | x cleanup | r refresh | q quit\n\n")

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

	return b.String()
}

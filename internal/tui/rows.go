package tui

import (
	"strings"

	"github.com/Molin-L/bmux/internal/model"
)

const noEpicLabel = "No Epic"

func buildIssueRows(issues []model.Issue) []issueRow {
	if len(issues) == 0 {
		return nil
	}

	type group struct {
		epicID    string
		epicTitle string
		issues    []model.Issue
	}

	groups := map[string]*group{}
	groupOrder := make([]string, 0, len(issues))

	for _, issue := range issues {
		key := strings.TrimSpace(issue.EpicID)
		if key == "" {
			key = noEpicLabel
		}

		g, ok := groups[key]
		if !ok {
			title := strings.TrimSpace(issue.EpicTitle)
			if key == noEpicLabel {
				title = noEpicLabel
			} else if title == "" {
				title = key
			}
			g = &group{
				epicID:    issue.EpicID,
				epicTitle: title,
			}
			groups[key] = g
			groupOrder = append(groupOrder, key)
		}

		if key != noEpicLabel && strings.TrimSpace(g.epicTitle) == "" {
			g.epicTitle = strings.TrimSpace(issue.EpicTitle)
			if g.epicTitle == "" {
				g.epicTitle = key
			}
		}

		g.issues = append(g.issues, issue)
	}

	rows := make([]issueRow, 0, len(issues)+len(groupOrder))
	for _, key := range groupOrder {
		g := groups[key]
		ordered := orderIssuesByHierarchy(g.issues)
		rows = append(rows, issueRow{
			kind:      issueRowEpicHeader,
			epicID:    strings.TrimSpace(g.epicID),
			epicTitle: strings.TrimSpace(g.epicTitle),
		})
		for _, issue := range ordered {
			rows = append(rows, issueRow{
				kind:  issueRowIssue,
				issue: issue,
			})
		}
	}
	return rows
}

func firstSelectableRow(rows []issueRow) int {
	for i, row := range rows {
		if row.kind == issueRowIssue {
			return i
		}
	}
	return -1
}

func rowIndexByIssueID(rows []issueRow, issueID string) int {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return -1
	}
	for i, row := range rows {
		if row.kind != issueRowIssue {
			continue
		}
		if strings.TrimSpace(row.issue.ID) == issueID {
			return i
		}
	}
	return -1
}

func nextSelectableRow(rows []issueRow, selected, delta int) int {
	if len(rows) == 0 {
		return -1
	}
	if delta == 0 {
		return selected
	}
	for idx := selected + delta; idx >= 0 && idx < len(rows); idx += delta {
		if rows[idx].kind == issueRowIssue {
			return idx
		}
	}
	return selected
}

func selectedIssueFromRows(rows []issueRow, selected int) (model.Issue, bool) {
	if selected < 0 || selected >= len(rows) {
		return model.Issue{}, false
	}
	row := rows[selected]
	if row.kind != issueRowIssue {
		return model.Issue{}, false
	}
	return row.issue, true
}

func orderIssuesByHierarchy(issues []model.Issue) []model.Issue {
	if len(issues) <= 1 {
		return issues
	}

	byID := make(map[string]model.Issue, len(issues))
	orderedIDs := make([]string, 0, len(issues))
	for _, issue := range issues {
		id := strings.TrimSpace(issue.ID)
		if id == "" {
			continue
		}
		orderedIDs = append(orderedIDs, id)
		byID[id] = issue
	}

	children := make(map[string][]string, len(issues))
	for _, issue := range issues {
		id := strings.TrimSpace(issue.ID)
		if id == "" {
			continue
		}
		parentID := issueParentID(issue)
		if parentID == "" {
			continue
		}
		children[parentID] = append(children[parentID], id)
	}

	roots := make([]string, 0, len(issues))
	for _, id := range orderedIDs {
		issue := byID[id]
		parentID := issueParentID(issue)
		if parentID == "" {
			roots = append(roots, id)
			continue
		}
		if _, ok := byID[parentID]; !ok {
			roots = append(roots, id)
		}
	}

	out := make([]model.Issue, 0, len(issues))
	visited := map[string]struct{}{}
	var visit func(string)
	visit = func(id string) {
		if _, seen := visited[id]; seen {
			return
		}
		issue, ok := byID[id]
		if !ok {
			return
		}
		visited[id] = struct{}{}
		out = append(out, issue)
		for _, childID := range children[id] {
			visit(childID)
		}
	}

	for _, id := range roots {
		visit(id)
	}
	for _, id := range orderedIDs {
		visit(id)
	}
	return out
}

func issueParentID(issue model.Issue) string {
	if parentID := strings.TrimSpace(issue.ParentID); parentID != "" {
		return parentID
	}
	for _, dep := range issue.Dependencies {
		if dep.Type != "parent-child" {
			continue
		}
		candidate := strings.TrimSpace(dep.IssueID)
		if candidate != "" && candidate != issue.ID {
			return candidate
		}
		candidate = strings.TrimSpace(dep.TargetID)
		if candidate != "" && candidate != issue.ID {
			return candidate
		}
	}
	return ""
}

package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	epicStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))

	selectedRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	statusStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	busyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	errorHintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

var wrapTokenPattern = regexp.MustCompile(`\S+|\s+`)

type tasksRender struct {
	content   string
	rowStarts []int
	rowEnds   []int
	lineCount int
}

func (m Model) View() string {
	title := titleStyle.Render("bmux")
	help := helpStyle.Render("↑/↓ move • PgUp/PgDn/Home/End scroll • Space select task • Shift+Tab cycle mode • Enter start • n legacy planning • c capture • p PR • m merge • x cleanup • r refresh • q quit")

	meta := []string{}
	if len(m.taskModeOptions) > 0 {
		meta = append(meta, modeBadge(modeLabel(m.taskModeOptions[m.taskModeSelected])))
	}
	if strings.TrimSpace(m.selectedTaskIssueID) != "" {
		meta = append(meta, taskBadge("task "+m.selectedTaskIssueID))
	} else {
		meta = append(meta, taskBadge("task (none)"))
	}
	if !m.issueSourceAvailable {
		meta = append(meta, warnBadge("source unavailable"))
	}

	body := []string{lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", help), lipgloss.JoinHorizontal(lipgloss.Left, meta...)}

	width := m.taskViewportWidth
	if width <= 0 {
		width = 96
	}
	height := m.taskViewportHeight
	if height <= 0 {
		height = 18
	}
	render := m.renderTasksContent(width)
	offset := m.taskViewportYOffset
	maxOffset := max(0, render.lineCount-height)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	tasks := clipViewport(render.content, offset, height)
	body = append(body, panelStyle.Render(tasks))

	if m.busy {
		body = append(body, busyStyle.Render("Working..."))
	}
	if m.status != "" {
		body = append(body, statusStyle.Render("Status: "+m.status))
	}
	if m.pendingPaneID != "" {
		body = append(body, statusStyle.Render("Planning Pane: "+m.pendingPaneID))
	}
	if m.errorHint != "" {
		body = append(body, errorHintStyle.Render("Hint: "+m.errorHint))
	}
	if m.prompt != "" {
		body = append(body, panelStyle.Render("PR Prompt:\n"+m.prompt))
	}

	if m.mode == modeAgentSelect {
		body = append(body, panelStyle.Render(m.renderAgentSelector()))
	}
	if m.mode == modePlanConfirm {
		body = append(body, panelStyle.Render(m.renderPlanConfirm()))
	}

	return strings.Join(body, "\n")
}

func (m Model) renderTasksContent(width int) tasksRender {
	if width <= 0 {
		width = 96
	}
	summary := app.RunSummary{}
	if m.svc != nil {
		issueIDs := make([]string, 0, len(m.issues))
		for _, issue := range m.issues {
			if strings.TrimSpace(issue.ID) != "" {
				issueIDs = append(issueIDs, issue.ID)
			}
		}
		summary = safeRunSummary(m.svc, issueIDs)
	}

	lines := []string{headerStyle.Render("Tasks (wrapped, no truncation)")}
	rowStarts := make([]int, len(m.rows))
	rowEnds := make([]int, len(m.rows))
	for i := range rowStarts {
		rowStarts[i] = -1
		rowEnds[i] = -1
	}

	for i, row := range m.rows {
		switch row.kind {
		case issueRowEpicHeader:
			title := row.epicTitle
			if strings.TrimSpace(row.epicID) != "" {
				title = fmt.Sprintf("%s [%s]", row.epicTitle, row.epicID)
			}
			lines = append(lines, epicStyle.Render("Epic: "+title))
			lines = append(lines, "")
		case issueRowIssue:
			branch := "(no branch)"
			runState := "-"
			blockedBy := "-"
			paneText := "-"
			if m.svc != nil {
				if meta, ok := safeGetTaskMeta(m.svc, row.issue.ID); ok && strings.TrimSpace(meta.Branch) != "" {
					branch = meta.Branch
				}
				if run, ok := safeGetLiveRun(m.svc, row.issue.ID); ok && run.Running {
					runState = fmt.Sprintf("running(%s)", run.Mode)
					if strings.TrimSpace(run.PaneID) != "" {
						paneText = run.PaneID
					}
				}
			}
			if blockerID := strings.TrimSpace(m.blockedBy[row.issue.ID]); blockerID != "" {
				blockedBy = blockerID
			}

			selectedCursor := " "
			if i == m.selected {
				selectedCursor = "▶"
			}
			taskMark := "○"
			if strings.TrimSpace(m.selectedTaskIssueID) == strings.TrimSpace(row.issue.ID) {
				taskMark = "●"
			}

			metaLine := fmt.Sprintf("%s%s id=%s | p=%d | status=%s | run=%s | blocked_by=%s | pane=%s",
				selectedCursor,
				taskMark,
				row.issue.ID,
				row.issue.Priority,
				row.issue.Status,
				runState,
				blockedBy,
				paneText,
			)
			titleText := issueTreePrefix(m.rows, i) + row.issue.Title

			cardLines := []string{}
			cardLines = append(cardLines, wrapWithPrefixes(metaLine, "", "  ", width)...)
			cardLines = append(cardLines, wrapWithPrefixes(titleText, "  title: ", "         ", width)...)
			cardLines = append(cardLines, wrapWithPrefixes(branch, "  branch: ", "          ", width)...)

			start := len(lines)
			if i == m.selected {
				for _, card := range cardLines {
					lines = append(lines, selectedRowStyle.Render(card))
				}
			} else {
				lines = append(lines, cardLines...)
			}
			end := len(lines) - 1
			rowStarts[i] = start
			rowEnds[i] = end
			lines = append(lines, "")
		}
	}
	if summary.ActiveChaos {
		lines = append(lines, statusStyle.Render(fmt.Sprintf("Chaos: running=%d launched=%d finished=%d", summary.Running, summary.Launched, summary.Finished)))
	}
	content := strings.Join(lines, "\n")
	return tasksRender{
		content:   content,
		rowStarts: rowStarts,
		rowEnds:   rowEnds,
		lineCount: len(strings.Split(content, "\n")),
	}
}

func (m Model) renderAgentSelector() string {
	lines := []string{"Select agent for planning pane (Enter confirm, Esc cancel):"}
	for i, agent := range m.agentOptions {
		prefix := "  "
		if i == m.agentSelected {
			prefix = "▶ "
		}
		lines = append(lines, prefix+agent)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderPlanConfirm() string {
	lines := []string{"Confirm hierarchy creation (Enter create, Esc cancel):", "- Epic: " + m.extractedPlan.Epic.Title, "  - Task: " + m.extractedPlan.Task.Title}
	for _, st := range m.extractedPlan.Subtasks {
		lines = append(lines, "    - Subtask: "+st.Title)
	}
	return strings.Join(lines, "\n")
}

func modeBadge(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("31")).Padding(0, 1).Render("mode " + value)
}

func taskBadge(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1).Render(value)
}

func warnBadge(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Padding(0, 1).Render(value)
}

func modeLabel(mode model.RunMode) string {
	switch mode {
	case model.RunModePlan:
		return "Plan"
	case model.RunModeSelfRun:
		return "Self-run"
	case model.RunModeChaos:
		return "Chaos"
	default:
		return string(mode)
	}
}

func issueTreePrefix(rows []issueRow, index int) string {
	if index < 0 || index >= len(rows) || rows[index].kind != issueRowIssue {
		return ""
	}
	depth := max(0, rows[index].issue.HierarchyDepth)
	if depth == 0 {
		return ""
	}

	var b strings.Builder
	for level := 1; level < depth; level++ {
		b.WriteString("│  ")
	}
	if hasSiblingAhead(rows, index, depth) {
		b.WriteString("├─ ")
	} else {
		b.WriteString("└─ ")
	}
	return b.String()
}

func hasSiblingAhead(rows []issueRow, index, depth int) bool {
	for i := index + 1; i < len(rows); i++ {
		if rows[i].kind != issueRowIssue {
			continue
		}
		nextDepth := max(0, rows[i].issue.HierarchyDepth)
		if nextDepth < depth {
			return false
		}
		if nextDepth == depth {
			return true
		}
	}
	return false
}

func wrapWithPrefixes(text, firstPrefix, continuationPrefix string, width int) []string {
	if width <= 0 {
		return []string{firstPrefix + text}
	}

	firstLimit := max(1, width-lipgloss.Width(firstPrefix))
	nextLimit := max(1, width-lipgloss.Width(continuationPrefix))
	parts := wrapText(text, firstLimit, nextLimit)
	lines := make([]string, 0, len(parts))
	for i, part := range parts {
		if i == 0 {
			lines = append(lines, firstPrefix+part)
		} else {
			lines = append(lines, continuationPrefix+part)
		}
	}
	return lines
}

func wrapText(text string, firstLimit, nextLimit int) []string {
	if text == "" {
		return []string{""}
	}
	if firstLimit <= 0 {
		firstLimit = 1
	}
	if nextLimit <= 0 {
		nextLimit = 1
	}

	tokens := wrapTokenPattern.FindAllString(text, -1)
	if len(tokens) == 0 {
		return []string{text}
	}

	lines := []string{}
	var current strings.Builder
	currentWidth := 0
	limit := firstLimit
	firstLine := true

	flush := func() {
		lines = append(lines, strings.TrimRightFunc(current.String(), unicode.IsSpace))
		current.Reset()
		currentWidth = 0
		limit = nextLimit
		firstLine = false
	}

	for _, token := range tokens {
		tokenWidth := lipgloss.Width(token)
		spaceOnly := strings.TrimSpace(token) == ""

		if spaceOnly {
			if currentWidth == 0 {
				continue
			}
			if currentWidth+tokenWidth <= limit {
				current.WriteString(token)
				currentWidth += tokenWidth
				continue
			}
			flush()
			continue
		}

		if currentWidth+tokenWidth <= limit {
			current.WriteString(token)
			currentWidth += tokenWidth
			continue
		}

		if tokenWidth <= limit {
			if currentWidth > 0 {
				flush()
			}
			current.WriteString(token)
			currentWidth = tokenWidth
			continue
		}

		if currentWidth > 0 {
			flush()
		}
		hardParts := hardWrapToken(token, limit, nextLimit)
		for idx, part := range hardParts {
			if idx < len(hardParts)-1 {
				lines = append(lines, part)
				limit = nextLimit
				firstLine = false
				continue
			}
			current.WriteString(part)
			currentWidth = lipgloss.Width(part)
			limit = nextLimit
			firstLine = false
		}
	}

	if currentWidth > 0 || len(lines) == 0 || firstLine {
		lines = append(lines, strings.TrimRightFunc(current.String(), unicode.IsSpace))
	}
	return lines
}

func hardWrapToken(token string, firstLimit, nextLimit int) []string {
	limits := []int{firstLimit}
	out := []string{}
	var part strings.Builder
	partWidth := 0
	limitIndex := 0

	currentLimit := func() int {
		if limitIndex < len(limits) {
			return max(1, limits[limitIndex])
		}
		return max(1, nextLimit)
	}

	for _, r := range token {
		rw := lipgloss.Width(string(r))
		limit := currentLimit()
		if partWidth+rw > limit && partWidth > 0 {
			out = append(out, part.String())
			part.Reset()
			partWidth = 0
			limitIndex++
			limit = currentLimit()
			if rw > limit {
				out = append(out, string(r))
				limitIndex++
				continue
			}
		}
		part.WriteRune(r)
		partWidth += rw
	}
	if part.Len() > 0 {
		out = append(out, part.String())
	}
	if len(out) == 0 {
		return []string{token}
	}
	return out
}

func clipViewport(content string, offset, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return ""
	}
	if height <= 0 {
		height = 1
	}
	maxOffset := max(0, len(lines)-height)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[offset:end], "\n")
}

func safeRunSummary(svc *app.Service, issueIDs []string) (summary app.RunSummary) {
	if svc == nil {
		return app.RunSummary{}
	}
	defer func() {
		if recover() != nil {
			summary = app.RunSummary{}
		}
	}()
	if s, err := svc.RunSummary(issueIDs); err == nil {
		return s
	}
	return app.RunSummary{}
}

func safeGetTaskMeta(svc *app.Service, issueID string) (meta model.TaskBranchMeta, ok bool) {
	if svc == nil {
		return model.TaskBranchMeta{}, false
	}
	defer func() {
		if recover() != nil {
			meta = model.TaskBranchMeta{}
			ok = false
		}
	}()
	got, exists, err := svc.GetTaskMeta(issueID)
	if err != nil || !exists {
		return model.TaskBranchMeta{}, false
	}
	return got, true
}

func safeGetLiveRun(svc *app.Service, issueID string) (run model.LiveRun, ok bool) {
	if svc == nil {
		return model.LiveRun{}, false
	}
	defer func() {
		if recover() != nil {
			run = model.LiveRun{}
			ok = false
		}
	}()
	got, exists, err := svc.GetLiveRun(issueID)
	if err != nil || !exists {
		return model.LiveRun{}, false
	}
	return got, true
}

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

	metaKeyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	metaValueStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	metaEmptyValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	metaSepStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
	help := helpStyle.Render("↑/↓ move • PgUp/PgDn/Home/End scroll • Space select/deselect task(s) • Shift+Tab cycle mode • Enter start • n legacy planning • c capture • p PR • m merge • x cleanup • r refresh • q quit")

	meta := []string{}
	if len(m.taskModeOptions) > 0 {
		meta = append(meta, modeBadge(modeLabel(m.taskModeOptions[m.taskModeSelected])))
	}
	meta = append(meta, taskBadge(m.selectedTasksBadgeText()))
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
					if run.Pending {
						runState = fmt.Sprintf("waiting(%s)", run.Mode)
						if blockerID := strings.TrimSpace(run.BlockedByIssueID); blockerID != "" {
							blockedBy = blockerID
						}
					} else {
						runState = fmt.Sprintf("running(%s)", run.Mode)
					}
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
			if _, selected := m.selectedTaskIssueIDs[strings.TrimSpace(row.issue.ID)]; selected {
				taskMark = "●"
			}

			nodePrefix := fmt.Sprintf("%s%s %s", selectedCursor, taskMark, issueIndentPrefix(m.rows, i))
			nodeIndent := strings.Repeat(" ", lipgloss.Width(nodePrefix))
			metaPrefix := strings.Repeat(" ", max(1, lipgloss.Width(nodePrefix)-2))

			metaLine := fmt.Sprintf("id=%s | p=%d | status=%s | run=%s | blocked_by=%s | pane=%s | branch=%s",
				row.issue.ID, row.issue.Priority, row.issue.Status, runState, blockedBy, paneText, branch)

			rowLines := []string{}
			rowLines = append(rowLines, wrapWithPrefixes(row.issue.Title, nodePrefix, nodeIndent, width)...)
			metaLines := wrapWithPrefixes(metaLine, metaPrefix, metaPrefix, width)
			for idx := range metaLines {
				metaLines[idx] = styleMetaLine(metaLines[idx])
			}
			rowLines = append(rowLines, metaLines...)

			start := len(lines)
			if i == m.selected {
				for _, line := range rowLines {
					lines = append(lines, selectedRowStyle.Render(line))
				}
			} else {
				lines = append(lines, rowLines...)
			}
			end := len(lines) - 1
			rowStarts[i] = start
			rowEnds[i] = end
		}
	}
	if summary.ActiveApe {
		lines = append(lines, statusStyle.Render(fmt.Sprintf("Ape: running=%d launched=%d finished=%d", summary.Running, summary.Launched, summary.Finished)))
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

func (m Model) selectedTasksBadgeText() string {
	count := len(m.selectedTaskIssueIDs)
	switch count {
	case 0:
		return "task (none)"
	case 1:
		for _, issueID := range m.issueIDsInRowOrder(m.selectedTaskIssueIDs) {
			return "task " + issueID
		}
		return "task (none)"
	default:
		return fmt.Sprintf("tasks %d selected", count)
	}
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
	case model.RunModeApe, model.RunModeChaos:
		return "Ape"
	default:
		return string(mode)
	}
}

func issueIndentPrefix(rows []issueRow, index int) string {
	if index < 0 || index >= len(rows) || rows[index].kind != issueRowIssue {
		return ""
	}

	_, _, baseDepth := issueSectionBounds(rows, index)
	depth := normalizedRowDepth(rows[index], baseDepth)
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("  ", depth)
}

func issueSectionBounds(rows []issueRow, index int) (int, int, int) {
	start := index
	for i := index - 1; i >= 0; i-- {
		if rows[i].kind == issueRowEpicHeader {
			start = i + 1
			break
		}
		start = i
	}

	end := index
	for i := index + 1; i < len(rows); i++ {
		if rows[i].kind == issueRowEpicHeader {
			break
		}
		end = i
	}

	baseDepth := 0
	first := true
	for i := start; i <= end; i++ {
		if rows[i].kind != issueRowIssue {
			continue
		}
		d := max(0, rows[i].issue.HierarchyDepth)
		if first || d < baseDepth {
			baseDepth = d
			first = false
		}
	}
	if first {
		baseDepth = 0
	}
	return start, end, baseDepth
}

func normalizedRowDepth(row issueRow, baseDepth int) int {
	depth := max(0, row.issue.HierarchyDepth-baseDepth)
	return depth
}

func styleMetaLine(line string) string {
	parts := strings.Split(line, " | ")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			out = append(out, part)
			continue
		}
		valStyle := metaValueStyle
		if isEmptyMetaValue(v) {
			valStyle = metaEmptyValueStyle
		}
		out = append(out, metaKeyStyle.Render(k+"=")+valStyle.Render(v))
	}
	return strings.Join(out, metaSepStyle.Render(" | "))
}

func isEmptyMetaValue(v string) bool {
	trimmed := strings.TrimSpace(v)
	switch trimmed {
	case "", "-", "(no branch)", "(none)", "none", "null":
		return true
	default:
		return false
	}
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

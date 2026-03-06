package waitpane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Options struct {
	IssueID   string
	BlockedBy string
	Branch    string
}

type tickMsg time.Time
type pollTickMsg time.Time

type blockerStatusMsg struct {
	status    string
	checkedAt time.Time
	err       error
}

type blockerStatusFetcher func(ctx context.Context, blockerID string) (string, error)

type model struct {
	opts          Options
	startedAt     time.Time
	now           time.Time
	spinIndex     int
	width         int
	height        int
	blockerStatus string
	lastCheckedAt time.Time
	lastCheckErr  string
	fetchStatus   blockerStatusFetcher
}

const (
	spinnerTickInterval = 200 * time.Millisecond
	statusPollInterval  = 2 * time.Second
	statusPollTimeout   = 1500 * time.Millisecond
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var (
	waitTitleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	waitSubtitleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	waitPanelStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	waitKeyStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	waitValueStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	waitHelpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	waitWarnStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	waitPromotionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	waitUnknownBadge     = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("244")).Padding(0, 1)
	waitOpenBadge        = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("114")).Padding(0, 1)
	waitInProgressBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Padding(0, 1)
	waitClosedBadge      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("35")).Padding(0, 1)
	waitDefaultStateText = "unknown"
)

func Run(opts Options) error {
	start := time.Now()
	p := tea.NewProgram(model{
		opts:          opts,
		startedAt:     start,
		now:           start,
		blockerStatus: waitDefaultStateText,
		fetchStatus:   fetchBlockerStatus,
	}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinnerTickCmd(),
		m.pollTickCmd(),
		m.fetchBlockerStatusCmd(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		m.now = time.Time(msg)
		m.spinIndex = (m.spinIndex + 1) % len(spinnerFrames)
		return m, m.spinnerTickCmd()
	case pollTickMsg:
		return m, tea.Batch(m.pollTickCmd(), m.fetchBlockerStatusCmd())
	case blockerStatusMsg:
		if status := normalizeBlockerStatus(msg.status); status != "" {
			m.blockerStatus = status
		}
		if !msg.checkedAt.IsZero() {
			m.lastCheckedAt = msg.checkedAt
		}
		if msg.err != nil {
			m.lastCheckErr = strings.TrimSpace(msg.err.Error())
		} else {
			m.lastCheckErr = ""
		}
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	elapsed := m.now.Sub(m.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	spinner := spinnerFrames[m.spinIndex]
	header := lipgloss.JoinHorizontal(
		lipgloss.Left,
		waitTitleStyle.Render("Blocked Task"),
		"  ",
		waitSubtitleStyle.Render(fmt.Sprintf("%s Waiting for blocker to close...", spinner)),
	)

	metaLines := []string{
		renderMetaLine("Issue ID", m.opts.IssueID),
		renderMetaLine("Blocked By", m.opts.BlockedBy),
		renderMetaLine("Branch", m.opts.Branch),
		renderMetaLine("Elapsed", formatElapsed(elapsed)),
	}
	statusLines := []string{
		renderMetaLine("Status", renderStatusBadge(m.blockerStatus)),
		renderMetaLine("Last Checked", renderLastChecked(m.lastCheckedAt)),
	}
	if m.lastCheckErr != "" {
		statusLines = append(statusLines, waitWarnStyle.Render("Status check failed: "+m.lastCheckErr))
	}
	if strings.EqualFold(m.blockerStatus, "closed") {
		statusLines = append(statusLines, waitPromotionStyle.Render("Blocker closed; waiting for bmux promotion."))
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		waitPanelStyle.Render(strings.Join(metaLines, "\n")),
		"",
		waitPanelStyle.Render(strings.Join(statusLines, "\n")),
		"",
		waitHelpStyle.Render("Press q or Ctrl+C to close this waiting pane."),
	)

	if m.width > 0 && m.height > 0 {
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Padding(1, 2).Render(content)
	}
	if m.width > 0 {
		return lipgloss.NewStyle().Width(m.width).Padding(1, 2).Render(content)
	}
	return content
}

func (m model) spinnerTickCmd() tea.Cmd {
	return tea.Tick(spinnerTickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) pollTickCmd() tea.Cmd {
	return tea.Tick(statusPollInterval, func(t time.Time) tea.Msg {
		return pollTickMsg(t)
	})
}

func (m model) fetchBlockerStatusCmd() tea.Cmd {
	fetcher := m.fetchStatus
	if fetcher == nil {
		fetcher = fetchBlockerStatus
	}
	blockerID := strings.TrimSpace(m.opts.BlockedBy)
	return func() tea.Msg {
		checkedAt := time.Now().UTC()
		status := waitDefaultStateText
		if blockerID == "" {
			return blockerStatusMsg{
				status:    status,
				checkedAt: checkedAt,
				err:       errors.New("missing blocker id"),
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), statusPollTimeout)
		defer cancel()
		resolved, err := fetcher(ctx, blockerID)
		if err == nil {
			status = normalizeBlockerStatus(resolved)
			if status == "" {
				status = waitDefaultStateText
			}
		}
		return blockerStatusMsg{
			status:    status,
			checkedAt: checkedAt,
			err:       err,
		}
	}
}

func fetchBlockerStatus(ctx context.Context, blockerID string) (string, error) {
	blockerID = strings.TrimSpace(blockerID)
	if blockerID == "" {
		return "", errors.New("missing blocker id")
	}
	cmd := exec.CommandContext(ctx, "bd", "show", blockerID, "--json")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := strings.TrimSpace(string(exitErr.Stderr))
			if msg != "" {
				return "", errors.New(msg)
			}
		}
		return "", err
	}
	return parseBlockerStatusJSON(out)
}

func parseBlockerStatusJSON(raw []byte) (string, error) {
	type issuePayload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", errors.New("empty blocker payload")
	}

	var one issuePayload
	if err := json.Unmarshal([]byte(trimmed), &one); err == nil {
		if status := normalizeBlockerStatus(one.Status); status != "" {
			return status, nil
		}
	}

	var many []issuePayload
	if err := json.Unmarshal([]byte(trimmed), &many); err == nil {
		for _, item := range many {
			if status := normalizeBlockerStatus(item.Status); status != "" {
				return status, nil
			}
		}
		return "", errors.New("blocker status missing from array payload")
	}
	return "", errors.New("unable to parse blocker payload")
}

func renderMetaLine(key, value string) string {
	return waitKeyStyle.Render(key+": ") + waitValueStyle.Render(strings.TrimSpace(value))
}

func renderLastChecked(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Local().Format("15:04:05")
}

func renderStatusBadge(status string) string {
	switch normalizeBlockerStatus(status) {
	case "open":
		return waitOpenBadge.Render("open")
	case "in_progress":
		return waitInProgressBadge.Render("in_progress")
	case "closed":
		return waitClosedBadge.Render("closed")
	default:
		return waitUnknownBadge.Render("unknown")
	}
}

func normalizeBlockerStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "open", "in_progress", "closed", "blocked", "deferred":
		return status
	default:
		return ""
	}
}

func formatElapsed(d time.Duration) string {
	seconds := int(d.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

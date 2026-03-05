package waitpane

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	IssueID   string
	BlockedBy string
	Branch    string
}

type tickMsg time.Time

type model struct {
	opts      Options
	startedAt time.Time
	now       time.Time
	spinIndex int
}

var spinnerFrames = []string{"|", "/", "-", `\`}

func Run(opts Options) error {
	start := time.Now()
	p := tea.NewProgram(model{
		opts:      opts,
		startedAt: start,
		now:       start,
	})
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		m.now = time.Time(msg)
		m.spinIndex = (m.spinIndex + 1) % len(spinnerFrames)
		return m, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}
	return m, nil
}

func (m model) View() string {
	elapsed := m.now.Sub(m.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	status := fmt.Sprintf("[%s] Waiting for blocker to close...", spinnerFrames[m.spinIndex])
	lines := []string{
		"Blocked Task",
		"",
		"Issue    : " + m.opts.IssueID,
		"Blocked  : " + m.opts.BlockedBy,
		"Branch   : " + m.opts.Branch,
		"Elapsed  : " + formatElapsed(elapsed),
		"",
		status,
		"",
		"Press q or Ctrl+C to close this waiting pane.",
	}
	return drawPanel(lines)
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

func drawPanel(lines []string) string {
	width := 0
	for _, line := range lines {
		if len(line) > width {
			width = len(line)
		}
	}
	if width < 12 {
		width = 12
	}
	var b strings.Builder
	b.WriteString("+" + strings.Repeat("-", width+2) + "+\n")
	for _, line := range lines {
		padding := width - len(line)
		if padding < 0 {
			padding = 0
		}
		b.WriteString("| " + line + strings.Repeat(" ", padding) + " |\n")
	}
	b.WriteString("+" + strings.Repeat("-", width+2) + "+")
	return b.String()
}

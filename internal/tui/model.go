package tui

import (
	"github.com/Molin-L/bmux/internal/app"
	"github.com/Molin-L/bmux/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

type issuesLoadedMsg struct {
	issues []model.Issue
	state  app.IssueSourceState
	err    error
}

type actionResultMsg struct {
	status string
	prompt string
	err    error
}

type Model struct {
	svc                  *app.Service
	issues               []model.Issue
	selected             int
	status               string
	prompt               string
	busy                 bool
	width                int
	height               int
	issueSourceAvailable bool
	issueSourceReason    string
}

func NewModel(svc *app.Service) Model {
	return Model{
		svc:                  svc,
		status:               "Loading ready issues...",
		issueSourceAvailable: true,
	}
}

func (m Model) Init() tea.Cmd {
	return m.loadIssuesCmd()
}

func Run(svc *app.Service) error {
	p := tea.NewProgram(NewModel(svc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

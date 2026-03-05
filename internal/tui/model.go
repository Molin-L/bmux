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

type issueRowKind int

const (
	issueRowEpicHeader issueRowKind = iota
	issueRowIssue
)

type issueRow struct {
	kind      issueRowKind
	epicID    string
	epicTitle string
	issue     model.Issue
}

type actionResultMsg struct {
	status string
	prompt string
	paneID string
	err    error
}

type planCapturedMsg struct {
	plan app.PlanPayload
	err  error
}

type hierarchyCreatedMsg struct {
	result app.HierarchyResult
	err    error
}

type runsReconciledMsg struct {
	status string
	err    error
}

type blockedByLoadedMsg struct {
	blockedBy map[string]string
	err       error
}

type viewMode int

const (
	modeMain viewMode = iota
	modeAgentSelect
	modePlanConfirm
)

type Model struct {
	svc                  *app.Service
	issues               []model.Issue
	rows                 []issueRow
	selected             int
	selectedTaskIssueID  string
	blockedBy            map[string]string
	taskViewportYOffset  int
	taskViewportWidth    int
	taskViewportHeight   int
	status               string
	prompt               string
	busy                 bool
	width                int
	height               int
	issueSourceAvailable bool
	issueSourceReason    string
	mode                 viewMode
	taskModeOptions      []model.RunMode
	taskModeSelected     int
	agentOptions         []string
	agentSelected        int
	pendingPaneID        string
	selectedAgent        string
	extractedPlan        app.PlanPayload
	lastPlanRaw          string
	errorHint            string
}

func NewModel(svc *app.Service) Model {
	return Model{
		svc:                  svc,
		status:               "Loading ready issues...",
		issueSourceAvailable: true,
		mode:                 modeMain,
		taskModeOptions:      []model.RunMode{model.RunModePlan, model.RunModeSelfRun, model.RunModeChaos},
		blockedBy:            map[string]string{},
		taskViewportWidth:    96,
		taskViewportHeight:   18,
		agentOptions:         []string{"claude", "codex"},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadIssuesCmd(), m.reconcileRunsCmd(), m.loadBlockedByCmd())
}

func Run(svc *app.Service) error {
	p := tea.NewProgram(NewModel(svc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

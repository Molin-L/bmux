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
	issueRowStatusHeader
	issueRowIssue
)

type issueRow struct {
	kind        issueRowKind
	statusLabel string
	epicID      string
	epicTitle   string
	issue       model.Issue
}

type actionResultMsg struct {
	status        string
	details       []string
	prompt        string
	paneID        string
	err           error
	refreshIssues bool
	keepStatus    bool
	conflict      *pendingMergeConflict
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

type spinnerTickMsg struct{}

type batchLaunchResultMsg struct {
	mode    model.RunMode
	started map[string]string
	waiting map[string]string
	failed  map[string]string
}

type viewMode int

const (
	modeMain viewMode = iota
	modeAgentSelect
	modePlanConfirm
	modeMergeConflictConfirm
)

const (
	compactViewportThreshold = 64
	taskViewportMinWidth     = 24
	compactDetailsMinLines   = 4
	compactDetailsMaxLines   = 6
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type pendingMergeConflict struct {
	issueID  string
	runMode  model.RunMode
	conflict *app.MergeConflictError
}

type Model struct {
	svc                    *app.Service
	issues                 []model.Issue
	rows                   []issueRow
	selected               int
	selectedTaskIssueIDs   map[string]struct{}
	blockedBy              map[string]string
	batchActive            bool
	taskViewportYOffset    int
	taskViewportWidth      int
	taskViewportHeight     int
	taskDetailsHeight      int
	status                 string
	statusDetails          []string
	prompt                 string
	busy                   bool
	width                  int
	height                 int
	issueSourceAvailable   bool
	issueSourceReason      string
	mode                   viewMode
	taskModeOptions        []model.RunMode
	taskModeSelected       int
	agentOptions           []string
	agentSelected          int
	pendingPaneID          string
	selectedAgent          string
	extractedPlan          app.PlanPayload
	lastPlanRaw            string
	errorHint              string
	spinnerFrame           int
	preserveStatusNextLoad bool
	pendingMergeConflict   *pendingMergeConflict
}

func NewModel(svc *app.Service) Model {
	return Model{
		svc:                  svc,
		status:               "Loading issues...",
		issueSourceAvailable: true,
		mode:                 modeMain,
		taskModeOptions:      []model.RunMode{model.RunModePlan, model.RunModeSelfRun, model.RunModeApe},
		selectedTaskIssueIDs: map[string]struct{}{},
		blockedBy:            map[string]string{},
		taskViewportWidth:    96,
		taskViewportHeight:   18,
		agentOptions:         []string{"claude", "codex"},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadIssuesCmd(), m.reconcileRunsCmd(), m.loadBlockedByCmd(), m.spinnerTickCmd())
}

func Run(svc *app.Service) error {
	p := tea.NewProgram(NewModel(svc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

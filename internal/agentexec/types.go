package agentexec

import (
	"context"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

type BeadsClient interface {
	Show(ctx context.Context, issueID string) (model.Issue, error)
	Dependencies(ctx context.Context, issueID string) ([]model.Dependency, error)
	Claim(ctx context.Context, issueID string) error
}

type TmuxClient interface {
	SplitPane(ctx context.Context, direction, cwd string) (string, error)
	SplitPaneOnTarget(ctx context.Context, direction, cwd, target string) (string, error)
	SplitPaneOnTargetWithCommand(ctx context.Context, direction, cwd, target, command string) (string, error)
	SendKeys(ctx context.Context, paneID, text string, enter bool) error
	CapturePane(ctx context.Context, paneID string, lines int) (string, error)
	CurrentPaneID(ctx context.Context) (string, error)
	ListPanes(ctx context.Context, target string) ([]string, error)
	SetWindowOptionsForSidebar(ctx context.Context, target string, controlWidth int) error
	SelectLayout(ctx context.Context, target, layout string) error
	SelectLayoutMainVertical(ctx context.Context, target string) error
	SetBuffer(ctx context.Context, bufferName, content string) error
	PasteBuffer(ctx context.Context, bufferName, paneID string) error
	DeleteBuffer(ctx context.Context, bufferName string) error
	GetPaneCurrentCommand(ctx context.Context, paneID string) (string, error)
	GetWindowDimensions(ctx context.Context) (int, int, error)
	GetTerminalDimensions(ctx context.Context) (int, int, error)
	SetWindowSizeManual(ctx context.Context, target string, width, height int) error
	SetPaneTitle(ctx context.Context, paneID, title string) error
	GetPaneTitle(ctx context.Context, paneID string) (string, error)
	KillPane(ctx context.Context, paneID string) error
}

type OpenTaskFunc func(ctx context.Context, issueID string) (model.TaskBranchMeta, error)

type IssueSourceState struct {
	Available bool
	Reason    string
}

type ReadyIssuesFunc func(ctx context.Context) ([]model.Issue, IssueSourceState, error)

type Options struct {
	RepoRoot               string
	Store                  *state.Store
	Beads                  BeadsClient
	Tmux                   TmuxClient
	SplitDirection         string
	ClaudeCommand          string
	CodexCommand           string
	PromptTemplate         string
	TmuxLayout             string
	ControlWidth           int
	MinPaneWidth           int
	MaxPaneWidth           int
	ApeMaxParallel       int
	ExecutionPlanPrompt    string
	ExecutionSelfRunPrompt string
	ExecutionApePrompt   string
	OpenTask               OpenTaskFunc
	ReadyIssues            ReadyIssuesFunc
}

package model

import "time"

type Dependency struct {
	Type      string
	IssueID   string
	TargetID  string
	Direction string
}

type RunMode string

const (
	RunModePlan    RunMode = "plan"
	RunModeSelfRun RunMode = "self_run"
	RunModeApe     RunMode = "ape"
)

type Issue struct {
	ID             string
	Title          string
	Description    string
	Status         string
	Priority       int
	IssueType      string `json:"issue_type"`
	ParentID       string `json:"parent"`
	Dependencies   []Dependency
	Metadata       map[string]string
	EpicID         string `json:"-"`
	EpicTitle      string `json:"-"`
	HierarchyDepth int    `json:"-"`
}

type TaskBranchMeta struct {
	IssueID      string    `json:"issue_id"`
	Branch       string    `json:"branch"`
	WorktreePath string    `json:"worktree_path"`
	BaseBranch   string    `json:"base_branch"`
	BaseCommit   string    `json:"base_commit"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
	PRPromptedAt time.Time `json:"pr_prompted_at,omitempty"`
}

type TaskRunMeta struct {
	IssueID          string    `json:"issue_id"`
	Mode             RunMode   `json:"mode"`
	Agent            string    `json:"agent"`
	PaneID           string    `json:"pane_id,omitempty"`
	Pending          bool      `json:"pending,omitempty"`
	BlockedByIssueID string    `json:"blocked_by_issue_id,omitempty"`
	WaitStartedAt    time.Time `json:"wait_started_at,omitempty"`
	StartedAt        time.Time `json:"started_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
	ExpectedProcess  string    `json:"expected_process,omitempty"`
	ApeSessionID   string    `json:"ape_session_id,omitempty"`
}

type LiveRun struct {
	IssueID          string
	Mode             RunMode
	PaneID           string
	Running          bool
	Pending          bool
	BlockedByIssueID string
	Agent            string
	StartedAt        time.Time
}

type ApeState struct {
	SessionID        string              `json:"session_id"`
	Active           bool                `json:"active"`
	PendingIssueIDs  []string            `json:"pending_issue_ids,omitempty"`
	Blockers         map[string][]string `json:"blockers,omitempty"`
	LaunchedIssueIDs []string            `json:"launched_issue_ids,omitempty"`
	FinishedIssueIDs []string            `json:"finished_issue_ids,omitempty"`
	ActiveIssueIDs   []string            `json:"active_issue_ids,omitempty"`
	StartedAt        time.Time           `json:"started_at,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at,omitempty"`
}

type CreateIssueRequest struct {
	Title       string
	Description string
	Type        string
	Priority    int
	ParentID    string
}

package model

import "time"

type Dependency struct {
	Type      string
	IssueID   string
	TargetID  string
	Direction string
}

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

type CreateIssueRequest struct {
	Title       string
	Description string
	Type        string
	Priority    int
	ParentID    string
}

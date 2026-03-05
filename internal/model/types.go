package model

import "time"

type Dependency struct {
	Type      string
	IssueID   string
	TargetID  string
	Direction string
}

type Issue struct {
	ID           string
	Title        string
	Description  string
	Status       string
	Priority     int
	Dependencies []Dependency
	Metadata     map[string]string
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

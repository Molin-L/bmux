package branching_test

import (
	"testing"
	"time"

	"github.com/Molin-L/bmux/internal/branching"
	"github.com/Molin-L/bmux/internal/model"
)

func TestBuildBranchName(t *testing.T) {
	t.Parallel()

	p := branching.NewPlanner("task/")
	got := p.BuildBranchName("bd-a1", "Implement OAuth 2.0 + callback flow")
	want := "task/bd-a1-implement-oauth-2-0-callback-flow"
	if got != want {
		t.Fatalf("branch name = %q, want %q", got, want)
	}
}

func TestResolveBranch_UsesDependencyPriorityAndNewest(t *testing.T) {
	t.Parallel()

	p := branching.NewPlanner("task/")
	issue := model.Issue{
		ID:    "bd-new",
		Title: "child task",
		Dependencies: []model.Dependency{
			{Type: "related", IssueID: "bd-r"},
			{Type: "blocks", IssueID: "bd-b"},
			{Type: "parent-child", IssueID: "bd-p"},
		},
	}

	now := time.Now().UTC()
	metas := []model.TaskBranchMeta{
		{IssueID: "bd-r", Branch: "task/bd-r-rel", Status: "active", CreatedAt: now.Add(-10 * time.Minute)},
		{IssueID: "bd-b", Branch: "task/bd-b-block", Status: "active", CreatedAt: now.Add(-5 * time.Minute)},
		{IssueID: "bd-p", Branch: "task/bd-p-parent", Status: "active", CreatedAt: now.Add(-1 * time.Minute)},
	}

	decision := p.ResolveBranch(issue, metas)
	if !decision.ReuseExisting {
		t.Fatal("expected branch reuse")
	}
	if decision.Branch != "task/bd-p-parent" {
		t.Fatalf("branch = %q, want parent-child branch", decision.Branch)
	}
}

func TestResolveBranch_NewWhenNoActiveCandidates(t *testing.T) {
	t.Parallel()

	p := branching.NewPlanner("task/")
	issue := model.Issue{ID: "bd-z9", Title: "Build TUI"}

	decision := p.ResolveBranch(issue, nil)
	if decision.ReuseExisting {
		t.Fatal("expected new branch")
	}
	if decision.Branch != "task/bd-z9-build-tui" {
		t.Fatalf("branch = %q, want generated branch", decision.Branch)
	}
}

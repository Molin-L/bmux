package pr_test

import (
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/pr"
)

func TestBuildPromptIncludesCapturedBase(t *testing.T) {
	t.Parallel()

	b := pr.Builder{}
	issue := model.Issue{ID: "bd-1", Title: "Auth flow"}
	meta := model.TaskBranchMeta{Branch: "task/bd-1-auth-flow", BaseBranch: "release/2026-03", BaseCommit: "abc123"}

	out := b.Build(issue, meta)
	if !strings.Contains(out, "release/2026-03") {
		t.Fatalf("prompt missing captured base branch: %s", out)
	}
	if !strings.Contains(out, "task/bd-1-auth-flow") {
		t.Fatalf("prompt missing branch name: %s", out)
	}
}

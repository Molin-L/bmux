package pr

import (
	"fmt"
	"strings"

	"github.com/Molin-L/bmux/internal/model"
)

type Builder struct{}

func (Builder) Build(issue model.Issue, meta model.TaskBranchMeta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Please create a PR/MR for issue %s (%s).\n", issue.ID, issue.Title)
	fmt.Fprintf(&b, "Source branch: %s\n", meta.Branch)
	fmt.Fprintf(&b, "Target base branch: %s\n", meta.BaseBranch)
	fmt.Fprintf(&b, "Captured base commit: %s\n\n", meta.BaseCommit)
	fmt.Fprintf(&b, "Checklist:\n")
	fmt.Fprintf(&b, "1. Summarize the implementation clearly.\n")
	fmt.Fprintf(&b, "2. Include testing evidence and risk notes.\n")
	fmt.Fprintf(&b, "3. Reference Beads issue %s.\n", issue.ID)
	fmt.Fprintf(&b, "4. Keep this branch open until merge is explicitly completed in bmux.\n")
	return b.String()
}

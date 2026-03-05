package branching

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Molin-L/bmux/internal/model"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

type BranchDecision struct {
	Branch        string
	ReuseExisting bool
	SourceIssueID string
}

type Planner struct {
	prefix string
}

func NewPlanner(prefix string) *Planner {
	if prefix == "" {
		prefix = "task/"
	}
	return &Planner{prefix: prefix}
}

func (p *Planner) BuildBranchName(issueID, title string) string {
	slug := normalizeSlug(title)
	if slug == "" {
		slug = "task"
	}
	return p.prefix + issueID + "-" + slug
}

func (p *Planner) ResolveBranch(issue model.Issue, metas []model.TaskBranchMeta) BranchDecision {
	depToMeta := make(map[string]model.TaskBranchMeta)
	for _, meta := range metas {
		if meta.Status != "" && meta.Status != "active" {
			continue
		}
		if prev, ok := depToMeta[meta.IssueID]; ok {
			if meta.CreatedAt.After(prev.CreatedAt) {
				depToMeta[meta.IssueID] = meta
			}
			continue
		}
		depToMeta[meta.IssueID] = meta
	}

	order := []string{"parent-child", "blocks", "discovered-from", "related"}
	for _, depType := range order {
		candidates := make([]model.TaskBranchMeta, 0)
		for _, dep := range issue.Dependencies {
			if dep.Type != depType {
				continue
			}
			id := firstNonEmpty(dep.IssueID, dep.TargetID)
			if id == "" {
				continue
			}
			if meta, ok := depToMeta[id]; ok {
				candidates = append(candidates, meta)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
		})
		best := candidates[0]
		return BranchDecision{Branch: best.Branch, ReuseExisting: true, SourceIssueID: best.IssueID}
	}

	return BranchDecision{Branch: p.BuildBranchName(issue.ID, issue.Title), ReuseExisting: false}
}

func normalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x != "" {
			return x
		}
	}
	return ""
}

package agentexec

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Molin-L/bmux/internal/model"
)

var nonAlphaNumericToken = regexp.MustCompile(`[^a-z0-9]+`)

func isApeMode(mode model.RunMode) bool {
	return mode == model.RunModeApe
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func stringsToSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}

func setToSortedSlice(m map[string]struct{}) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func buildTaskPaneTitle(issueID, issueTitle string) string {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		issueID = "task"
	}
	return issueID + "-" + normalizeTaskPaneSlug(issueTitle)
}

func normalizeTaskPaneSlug(title string) string {
	slug := strings.ToLower(strings.TrimSpace(title))
	slug = nonAlphaNumericToken.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		slug = "task"
	}
	return slug
}

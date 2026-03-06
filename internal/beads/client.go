package beads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/Molin-L/bmux/internal/errorsx"
	"github.com/Molin-L/bmux/internal/execx"
	"github.com/Molin-L/bmux/internal/model"
)

type Client struct {
	repoRoot string
	runner   runner
}

func NewClient(repoRoot string, runner *execx.Runner) *Client {
	if runner == nil {
		runner = execx.New(0)
	}
	return &Client{repoRoot: repoRoot, runner: runner}
}

type runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

func (c *Client) Ready(ctx context.Context) ([]model.Issue, error) {
	var out []model.Issue
	if err := c.runJSON(ctx, []string{"ready", "--json"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) List(ctx context.Context, filters map[string]string) ([]model.Issue, error) {
	args := []string{"list"}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--"+k, filters[k])
	}
	args = append(args, "--json")

	var out []model.Issue
	if err := c.runJSON(ctx, args, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Show(ctx context.Context, issueID string) (model.Issue, error) {
	var asArray []model.Issue
	if err := c.runJSON(ctx, []string{"show", issueID, "--json"}, &asArray); err == nil && len(asArray) > 0 {
		return asArray[0], nil
	}

	var out model.Issue
	if err := c.runJSON(ctx, []string{"show", issueID, "--json"}, &out); err != nil {
		return model.Issue{}, err
	}
	if out.ID == "" {
		out.ID = issueID
	}
	return out, nil
}

func (c *Client) Dependencies(ctx context.Context, issueID string) ([]model.Dependency, error) {
	raw := []map[string]any{}
	if err := c.runJSON(ctx, []string{"dep", "list", issueID, "--json"}, &raw); err != nil {
		return nil, err
	}

	deps := make([]model.Dependency, 0, len(raw))
	for _, m := range raw {
		d := parseDependency(issueID, m)
		if d.IssueID == "" {
			continue
		}
		deps = append(deps, d)
	}
	return deps, nil
}

func (c *Client) UpdateMetadata(ctx context.Context, issueID string, metadata map[string]string) error {
	args := []string{"update", issueID}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--set-metadata", fmt.Sprintf("%s=%s", k, metadata[k]))
	}
	args = append(args, "--json")
	_, err := c.runner.Run(ctx, c.repoRoot, "bd", args...)
	return err
}

func (c *Client) CreateIssue(ctx context.Context, req model.CreateIssueRequest) (model.Issue, error) {
	args := []string{
		"create",
		"--title", req.Title,
		"--description", req.Description,
		"--type", req.Type,
		"--priority", strconv.Itoa(req.Priority),
	}
	if strings.TrimSpace(req.ParentID) != "" {
		args = append(args, "--parent", req.ParentID)
	}
	args = append(args, "--json")
	stdout, err := c.runner.Run(ctx, c.repoRoot, "bd", args...)
	if err != nil {
		return model.Issue{}, err
	}
	if strings.TrimSpace(stdout) == "" {
		return model.Issue{}, errors.New("bd create returned empty json")
	}
	var asArray []model.Issue
	if err := json.Unmarshal([]byte(stdout), &asArray); err == nil && len(asArray) > 0 {
		return asArray[0], nil
	}
	var out model.Issue
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		return model.Issue{}, fmt.Errorf("parse bd json (%s): %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func (c *Client) AddDependency(ctx context.Context, issueID, blockedByID, depType string) error {
	issueID = strings.TrimSpace(issueID)
	blockedByID = strings.TrimSpace(blockedByID)
	if issueID == "" || blockedByID == "" {
		return errors.New("issue id and blocked-by id are required")
	}
	depType = strings.TrimSpace(depType)
	if depType == "" {
		depType = "blocks"
	}
	_, err := c.runner.Run(ctx, c.repoRoot, "bd", "dep", "add", issueID, blockedByID, "--type", depType, "--json")
	return err
}

func (c *Client) Claim(ctx context.Context, issueID string) error {
	_, err := c.runner.Run(ctx, c.repoRoot, "bd", "update", issueID, "--claim", "--json")
	return err
}

func (c *Client) Handoff(ctx context.Context, issueID string) error {
	actor, err := ResolveActor(ctx, c.repoRoot)
	if err != nil {
		return err
	}
	_, err = c.runner.Run(ctx, c.repoRoot, "bd", "update", issueID, "--assignee", actor, "--status", "in_progress", "--json")
	return err
}

func (c *Client) Close(ctx context.Context, issueID, reason string) error {
	if reason == "" {
		reason = "Completed via bmux"
	}
	_, err := c.runner.Run(ctx, c.repoRoot, "bd", "close", issueID, "--reason", reason, "--json")
	return err
}

func (c *Client) runJSON(ctx context.Context, args []string, out any) error {
	stdout, err := c.runner.Run(ctx, c.repoRoot, "bd", args...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(stdout) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(stdout), out); err != nil {
		return fmt.Errorf("parse bd json (%s): %w", strings.Join(args, " "), err)
	}
	return nil
}

func BDUnavailableReason(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "bd not found in PATH", true
	}

	var cmdErr *errorsx.CommandError
	if errors.As(err, &cmdErr) {
		if errors.Is(cmdErr.Unwrap(), exec.ErrNotFound) {
			return "bd not found in PATH", true
		}

		stderr := strings.ToLower(strings.TrimSpace(cmdErr.StdErr))
		if cmdErr.ExitCode == 127 || containsAny(stderr, "command not found", "no such file or directory", "not recognized as an internal or external command") {
			return "bd not found in PATH", true
		}

		if containsAny(stderr, "unknown command", "unrecognized option", "flag provided but not defined", "invalid argument") {
			return "bd invocation incompatible with installed version", true
		}
	}

	return "", false
}

func IsNoReadyIssuesError(err error) bool {
	if err == nil {
		return false
	}

	var cmdErr *errorsx.CommandError
	if errors.As(err, &cmdErr) {
		stderr := strings.ToLower(strings.TrimSpace(cmdErr.StdErr))
		return containsAny(
			stderr,
			"no ready issues",
			"no issues found",
			"no matching issues",
			"0 issues",
		)
	}

	return false
}

func containsAny(s string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func parseDependency(currentIssueID string, m map[string]any) model.Dependency {
	depType := firstString(m, "type", "dependency_type", "relation")
	fromID := firstString(m, "from_id", "issue_id", "blocked", "blocked_id")
	toID := firstString(m, "to_id", "depends_on", "depends_on_id", "blocker", "blocker_id", "target_id", "parent_id", "child_id", "related_id")
	compactID := firstString(m, "id")

	direction := ""
	candidate := ""
	switch {
	case fromID == currentIssueID && toID != "":
		direction = "outgoing"
		candidate = toID
	case toID == currentIssueID && fromID != "":
		direction = "incoming"
		candidate = fromID
	case fromID == currentIssueID:
		direction = "outgoing"
	case toID == currentIssueID:
		direction = "incoming"
	case toID != "":
		candidate = toID
	case fromID != "":
		candidate = fromID
	case compactID != "":
		candidate = compactID
	}
	if direction == "" && compactID != "" && candidate != "" && candidate != currentIssueID {
		direction = "outgoing"
	}

	return model.Dependency{
		Type:      depType,
		IssueID:   candidate,
		TargetID:  candidate,
		Direction: direction,
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			switch v := raw.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case float64:
				return strconv.FormatInt(int64(v), 10)
			}
		}
	}
	return ""
}

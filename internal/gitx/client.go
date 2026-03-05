package gitx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Molin-L/bmux/internal/execx"
)

type Client struct {
	runner *execx.Runner
}

func NewClient(runner *execx.Runner) *Client {
	if runner == nil {
		runner = execx.New(0)
	}
	return &Client{runner: runner}
}

func (c *Client) CurrentHead(ctx context.Context, repoRoot string) (string, string, error) {
	branch, err := c.runner.Run(ctx, repoRoot, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", err
	}
	commit, err := c.runner.Run(ctx, repoRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(branch), strings.TrimSpace(commit), nil
}

func (c *Client) AddWorktree(ctx context.Context, repoRoot, path, branch, startRef string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create worktree parent dir: %w", err)
	}
	_, _ = c.runner.Run(ctx, repoRoot, "git", "worktree", "prune")
	_, err := c.runner.Run(ctx, repoRoot, "git", "worktree", "add", path, "-b", branch, startRef)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) RemoveWorktree(ctx context.Context, repoRoot, path string) error {
	_, err := c.runner.Run(ctx, repoRoot, "git", "worktree", "remove", path, "--force")
	return err
}

func (c *Client) Merge(ctx context.Context, repoPath, branch string) error {
	_, err := c.runner.Run(ctx, repoPath, "git", "merge", branch, "--no-edit")
	return err
}

func (c *Client) DeleteBranch(ctx context.Context, repoRoot, branch string) error {
	_, err := c.runner.Run(ctx, repoRoot, "git", "branch", "-d", branch)
	return err
}

func (c *Client) Push(ctx context.Context, repoRoot, branch string) error {
	_, err := c.runner.Run(ctx, repoRoot, "git", "push", "origin", branch)
	return err
}

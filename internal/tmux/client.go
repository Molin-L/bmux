package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Molin-L/bmux/internal/errorsx"
	"github.com/Molin-L/bmux/internal/execx"
)

type runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

type Client struct {
	runner runner
}

func NewClient(r *execx.Runner) *Client {
	if r == nil {
		r = execx.New(0)
	}
	return &Client{runner: r}
}

func (c *Client) SplitPane(ctx context.Context, direction, cwd string) (string, error) {
	return c.SplitPaneOnTarget(ctx, direction, cwd, "")
}

func (c *Client) SplitPaneOnTarget(ctx context.Context, direction, cwd, target string) (string, error) {
	if _, err := lookPath("tmux"); err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return "", errors.New("tmux not found in PATH; `n` requires tmux")
		}
		return "", err
	}
	if strings.TrimSpace(osEnv("TMUX")) == "" {
		return "", errors.New("not inside a tmux client; attach to tmux first")
	}

	args := []string{"split-window", "-P", "-F", "#{pane_id}"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", target)
	}
	if direction == "below" {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	out, err := c.runner.Run(ctx, cwd, "tmux", args...)
	if err != nil {
		return "", err
	}
	paneID := strings.TrimSpace(out)
	if paneID == "" {
		return "", fmt.Errorf("tmux split-window returned empty pane id")
	}
	return paneID, nil
}

func (c *Client) HasSession(ctx context.Context, name string) (bool, error) {
	if strings.TrimSpace(name) == "" {
		return false, errors.New("session name is required")
	}
	_, err := c.runner.Run(ctx, "", "tmux", "has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	var cmdErr *errorsx.CommandError
	if errors.As(err, &cmdErr) {
		stderr := strings.ToLower(strings.TrimSpace(cmdErr.StdErr))
		if strings.Contains(stderr, "can't find session") || strings.Contains(stderr, "no server running") || cmdErr.ExitCode == 1 {
			return false, nil
		}
	}
	return false, err
}

func (c *Client) NewSessionDetached(ctx context.Context, name, cwd, command string) error {
	args := []string{"new-session", "-d", "-s", name}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	if strings.TrimSpace(command) != "" {
		args = append(args, command)
	}
	_, err := c.runner.Run(ctx, cwd, "tmux", args...)
	return err
}

func (c *Client) AttachSession(ctx context.Context, name string) error {
	return attachSessionFn(ctx, name)
}

func (c *Client) CurrentPaneID(ctx context.Context) (string, error) {
	out, err := c.runner.Run(ctx, "", "tmux", "display-message", "-p", "#{pane_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) SetWindowOptionsForSidebar(ctx context.Context, target string, controlWidth int) error {
	if controlWidth <= 0 {
		controlWidth = 40
	}
	base := []string{}
	if strings.TrimSpace(target) != "" {
		base = []string{"-t", target}
	}
	if _, err := c.runner.Run(ctx, "", "tmux", append([]string{"set-option"}, append(base, "pane-border-status", "top")...)...); err != nil {
		return err
	}
	if _, err := c.runner.Run(ctx, "", "tmux", append([]string{"set-window-option"}, append(base, "main-pane-width", strconv.Itoa(controlWidth))...)...); err != nil {
		return err
	}
	_, err := c.runner.Run(ctx, "", "tmux", append([]string{"select-layout"}, append(base, "main-vertical")...)...)
	return err
}

func (c *Client) SelectLayoutMainVertical(ctx context.Context, target string) error {
	args := []string{"select-layout"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", target)
	}
	args = append(args, "main-vertical")
	_, err := c.runner.Run(ctx, "", "tmux", args...)
	return err
}

func (c *Client) ListPanes(ctx context.Context, target string) ([]string, error) {
	args := []string{"list-panes"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", target)
	}
	args = append(args, "-F", "#{pane_id}")
	out, err := c.runner.Run(ctx, "", "tmux", args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	panes := make([]string, 0, len(lines))
	for _, line := range lines {
		if v := strings.TrimSpace(line); v != "" {
			panes = append(panes, v)
		}
	}
	return panes, nil
}

func (c *Client) SendKeys(ctx context.Context, paneID, text string, enter bool) error {
	args := []string{"send-keys", "-t", paneID, text}
	if enter {
		args = append(args, "C-m")
	}
	_, err := c.runner.Run(ctx, "", "tmux", args...)
	return err
}

func (c *Client) CapturePane(ctx context.Context, paneID string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	start := -lines
	out, err := c.runner.Run(ctx, "", "tmux", "capture-pane", "-p", "-t", paneID, "-S", fmt.Sprintf("%d", start))
	if err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) SetBuffer(ctx context.Context, bufferName, content string) error {
	_, err := c.runner.Run(ctx, "", "tmux", "set-buffer", "-b", bufferName, "--", content)
	return err
}

func (c *Client) PasteBuffer(ctx context.Context, bufferName, paneID string) error {
	_, err := c.runner.Run(ctx, "", "tmux", "paste-buffer", "-b", bufferName, "-t", paneID)
	return err
}

func (c *Client) DeleteBuffer(ctx context.Context, bufferName string) error {
	_, err := c.runner.Run(ctx, "", "tmux", "delete-buffer", "-b", bufferName)
	return err
}

func (c *Client) GetPaneCurrentCommand(ctx context.Context, paneID string) (string, error) {
	out, err := c.runner.Run(ctx, "", "tmux", "display-message", "-p", "-t", paneID, "#{pane_current_command}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

var osEnv = func(key string) string {
	return os.Getenv(key)
}

var lookPath = exec.LookPath

var attachSessionFn = func(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

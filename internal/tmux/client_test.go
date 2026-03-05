package tmux

import (
	"context"
	"errors"
	"testing"

	"github.com/Molin-L/bmux/internal/errorsx"
)

type fakeRunner struct {
	out   string
	err   error
	calls []runCall
}

type runCall struct {
	dir  string
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, runCall{dir: dir, name: name, args: args})
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

func TestSplitPaneRequiresTmuxSession(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "" }
	defer func() { osEnv = orig }()
	origPath := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	defer func() { lookPath = origPath }()

	c := &Client{runner: &fakeRunner{}}
	_, err := c.SplitPane(context.Background(), "right", "/tmp")
	if err == nil || err.Error() != "not inside a tmux client; attach to tmux first" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestSplitPaneBuildsRightCommand(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "1" }
	defer func() { osEnv = orig }()
	origPath := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	defer func() { lookPath = origPath }()

	f := &fakeRunner{out: "%2\n"}
	c := &Client{runner: f}

	pane, err := c.SplitPane(context.Background(), "right", "/tmp/repo")
	if err != nil {
		t.Fatalf("split pane: %v", err)
	}
	if pane != "%2" {
		t.Fatalf("pane = %q", pane)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d", len(f.calls))
	}
	got := f.calls[0].args
	want := []string{"split-window", "-P", "-F", "#{pane_id}", "-h", "-c", "/tmp/repo"}
	if len(got) != len(want) {
		t.Fatalf("args len=%d want=%d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("arg[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestSplitPaneBuildsBelowCommand(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "1" }
	defer func() { osEnv = orig }()
	origPath := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	defer func() { lookPath = origPath }()

	f := &fakeRunner{out: "%3"}
	c := &Client{runner: f}
	_, err := c.SplitPane(context.Background(), "below", "/tmp/repo")
	if err != nil {
		t.Fatalf("split pane: %v", err)
	}
	if f.calls[0].args[4] != "-v" {
		t.Fatalf("expected vertical split args: %#v", f.calls[0].args)
	}
}

func TestSendKeysAndCapture(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{out: "x"}
	c := &Client{runner: f}
	if err := c.SendKeys(context.Background(), "%2", "hello", true); err != nil {
		t.Fatalf("send keys: %v", err)
	}
	if _, err := c.CapturePane(context.Background(), "%2", 50); err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d", len(f.calls))
	}
}

func TestSplitPaneOnTargetBuildsTargetFlag(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "1" }
	defer func() { osEnv = orig }()
	origPath := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	defer func() { lookPath = origPath }()

	f := &fakeRunner{out: "%4"}
	c := &Client{runner: f}
	_, err := c.SplitPaneOnTarget(context.Background(), "right", "/tmp/repo", "%3")
	if err != nil {
		t.Fatalf("split pane: %v", err)
	}
	got := f.calls[0].args
	found := false
	for i := 0; i < len(got)-1; i++ {
		if got[i] == "-t" && got[i+1] == "%3" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing target flag: %#v", got)
	}
}

func TestHasSessionNotFound(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{err: &errorsx.CommandError{StdErr: "can't find session", ExitCode: 1}}
	c := &Client{runner: f}
	ok, err := c.HasSession(context.Background(), "x")
	if err != nil {
		t.Fatalf("has session: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false")
	}
}

func TestSessionHelpers(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{out: "%1\n%2\n"}
	c := &Client{runner: f}
	origAttach := attachSessionFn
	attachSessionFn = func(context.Context, string) error { return nil }
	defer func() { attachSessionFn = origAttach }()
	if err := c.NewSessionDetached(context.Background(), "s", "/tmp", "cmd"); err != nil {
		t.Fatalf("new session: %v", err)
	}
	if err := c.AttachSession(context.Background(), "s"); err != nil {
		t.Fatalf("attach session: %v", err)
	}
	panes, err := c.ListPanes(context.Background(), "s")
	if err != nil {
		t.Fatalf("list panes: %v", err)
	}
	if len(panes) == 0 {
		t.Fatalf("expected panes")
	}
}

func TestBufferAndCommandHelpers(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{out: "codex\n"}
	c := &Client{runner: f}
	if err := c.SetBuffer(context.Background(), "b", "hello"); err != nil {
		t.Fatalf("set buffer: %v", err)
	}
	if err := c.PasteBuffer(context.Background(), "b", "%2"); err != nil {
		t.Fatalf("paste buffer: %v", err)
	}
	if err := c.DeleteBuffer(context.Background(), "b"); err != nil {
		t.Fatalf("delete buffer: %v", err)
	}
	cmd, err := c.GetPaneCurrentCommand(context.Background(), "%2")
	if err != nil {
		t.Fatalf("get command: %v", err)
	}
	if cmd != "codex" {
		t.Fatalf("cmd = %q", cmd)
	}
}

func TestSplitPaneTmuxMissingMessage(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "1" }
	defer func() { osEnv = orig }()

	origPath := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = origPath }()

	c := &Client{runner: &fakeRunner{}}
	_, err := c.SplitPane(context.Background(), "right", "/tmp")
	if err == nil || err.Error() != "tmux not found in PATH; `n` requires tmux" {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestSplitPaneRunnerError(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "1" }
	defer func() { osEnv = orig }()
	origPath := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	defer func() { lookPath = origPath }()

	f := &fakeRunner{err: errors.New("boom")}
	c := &Client{runner: f}
	_, err := c.SplitPane(context.Background(), "right", "/tmp")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
}

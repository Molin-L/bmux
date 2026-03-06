package tmux

import (
	"context"
	"errors"
	"testing"

	"github.com/Molin-L/bmux/internal/errorsx"
)

type fakeRunner struct {
	out     string
	outputs []string
	err     error
	calls   []runCall
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
	if len(f.outputs) > 0 {
		out := f.outputs[0]
		f.outputs = f.outputs[1:]
		return out, nil
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

func TestSplitPaneOnTargetWithCommandAppendsCommand(t *testing.T) {
	t.Parallel()
	orig := osEnv
	osEnv = func(string) string { return "1" }
	defer func() { osEnv = orig }()
	origPath := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	defer func() { lookPath = origPath }()

	f := &fakeRunner{out: "%4"}
	c := &Client{runner: f}
	_, err := c.SplitPaneOnTargetWithCommand(context.Background(), "right", "/tmp/repo", "%3", "cat")
	if err != nil {
		t.Fatalf("split pane: %v", err)
	}
	got := f.calls[0].args
	if got[len(got)-1] != "cat" {
		t.Fatalf("expected trailing command, got %#v", got)
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

func TestGetWindowAndTerminalDimensions(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{outputs: []string{"201 51\n", "220 60\n"}}
	c := &Client{runner: f}

	windowW, windowH, err := c.GetWindowDimensions(context.Background())
	if err != nil {
		t.Fatalf("window dimensions: %v", err)
	}
	if windowW != 201 || windowH != 51 {
		t.Fatalf("window dimensions = %dx%d", windowW, windowH)
	}

	terminalW, terminalH, err := c.GetTerminalDimensions(context.Background())
	if err != nil {
		t.Fatalf("terminal dimensions: %v", err)
	}
	if terminalW != 220 || terminalH != 60 {
		t.Fatalf("terminal dimensions = %dx%d", terminalW, terminalH)
	}
}

func TestSetWindowSizeManual(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{}
	c := &Client{runner: f}
	if err := c.SetWindowSizeManual(context.Background(), "", 180, 44); err != nil {
		t.Fatalf("set window size: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d", len(f.calls))
	}
	want0 := []string{"set-window-option", "window-size", "manual"}
	for i := range want0 {
		if f.calls[0].args[i] != want0[i] {
			t.Fatalf("set-window-option arg[%d]=%q want=%q", i, f.calls[0].args[i], want0[i])
		}
	}
	want1 := []string{"resize-window", "-x", "180", "-y", "44"}
	for i := range want1 {
		if f.calls[1].args[i] != want1[i] {
			t.Fatalf("resize-window arg[%d]=%q want=%q", i, f.calls[1].args[i], want1[i])
		}
	}
}

func TestSetPaneBorderStatus(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{}
	c := &Client{runner: f}
	if err := c.SetPaneBorderStatus(context.Background(), "sess", "off"); err != nil {
		t.Fatalf("set pane border status: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d", len(f.calls))
	}
	want := []string{"set-window-option", "-t", "sess", "pane-border-status", "off"}
	for i := range want {
		if f.calls[0].args[i] != want[i] {
			t.Fatalf("arg[%d]=%q want=%q", i, f.calls[0].args[i], want[i])
		}
	}
}

func TestSetWindowOptionsForSidebarUsesHiddenPaneBorderStatus(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{}
	c := &Client{runner: f}
	if err := c.SetWindowOptionsForSidebar(context.Background(), "sess", 48); err != nil {
		t.Fatalf("set window options: %v", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("calls = %d", len(f.calls))
	}
	want0 := []string{"set-window-option", "-t", "sess", "pane-border-status", "off"}
	for i := range want0 {
		if f.calls[0].args[i] != want0[i] {
			t.Fatalf("call0 arg[%d]=%q want=%q", i, f.calls[0].args[i], want0[i])
		}
	}
}

func TestPaneTitleAndLayoutHelpers(t *testing.T) {
	t.Parallel()
	f := &fakeRunner{outputs: []string{"", "bmux-spacer\n"}}
	c := &Client{runner: f}
	if err := c.SetPaneTitle(context.Background(), "%9", "bmux-spacer"); err != nil {
		t.Fatalf("set pane title: %v", err)
	}
	title, err := c.GetPaneTitle(context.Background(), "%9")
	if err != nil {
		t.Fatalf("get pane title: %v", err)
	}
	if title != "bmux-spacer" {
		t.Fatalf("title = %q", title)
	}
	if err := c.SelectLayout(context.Background(), "", "main-vertical"); err != nil {
		t.Fatalf("select layout: %v", err)
	}
	if err := c.KillPane(context.Background(), "%9"); err != nil {
		t.Fatalf("kill pane: %v", err)
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

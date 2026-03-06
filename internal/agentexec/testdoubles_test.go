package agentexec

import (
	"context"
	"errors"
	"strings"

	"github.com/Molin-L/bmux/internal/model"
)

type fakeBeads struct {
	issue        model.Issue
	deps         []model.Dependency
	depsByIssue  map[string][]model.Dependency
	showByID     map[string]model.Issue
	showErrByID  map[string]error
	showCalls    []string
	claimed      []string
	claimErrByID map[string]error
}

func (f *fakeBeads) Show(_ context.Context, issueID string) (model.Issue, error) {
	f.showCalls = append(f.showCalls, issueID)
	if err, ok := f.showErrByID[issueID]; ok {
		return model.Issue{}, err
	}
	if issue, ok := f.showByID[issueID]; ok {
		return issue, nil
	}
	if f.issue.ID == issueID {
		return f.issue, nil
	}
	if f.issue.ID == "" {
		out := f.issue
		out.ID = issueID
		return out, nil
	}
	return model.Issue{ID: issueID}, nil
}

func (f *fakeBeads) Dependencies(_ context.Context, issueID string) ([]model.Dependency, error) {
	if f.depsByIssue != nil {
		if deps, ok := f.depsByIssue[issueID]; ok {
			return deps, nil
		}
	}
	return f.deps, nil
}

func (f *fakeBeads) Claim(_ context.Context, issueID string) error {
	f.claimed = append(f.claimed, issueID)
	if err, ok := f.claimErrByID[issueID]; ok {
		return err
	}
	return nil
}

type fakeTmux struct {
	paneID          string
	currentPaneID   string
	captured        string
	splitErr        error
	sendErr         error
	captureErr      error
	bufferErr       error
	titleErr        error
	sent            []string
	splitDir        string
	splitCWD        string
	splitTarget     string
	splitCommand    string
	pasted          []string
	killed          []string
	panes           []string
	titles          map[string]string
	currentCommand  string
	currentByPane   map[string]string
	currentCmdErr   error
	terminalWidth   int
	terminalHeight  int
	windowWidth     int
	windowHeight    int
	layouts         []string
	layoutErr       error
	sidebarSetCalls int
	borderStatuses  []string
	windowSizeCalls int
}

func (t *fakeTmux) SplitPane(_ context.Context, direction, cwd string) (string, error) {
	t.splitDir = direction
	t.splitCWD = cwd
	if t.splitErr != nil {
		return "", t.splitErr
	}
	if t.paneID == "" {
		t.paneID = "%2"
	}
	return t.paneID, nil
}

func (t *fakeTmux) SplitPaneOnTarget(_ context.Context, direction, cwd, target string) (string, error) {
	t.splitTarget = target
	return t.SplitPane(context.Background(), direction, cwd)
}

func (t *fakeTmux) SplitPaneOnTargetWithCommand(_ context.Context, direction, cwd, target, command string) (string, error) {
	t.splitTarget = target
	t.splitCommand = command
	return t.SplitPane(context.Background(), direction, cwd)
}

func (t *fakeTmux) SendKeys(_ context.Context, _ string, text string, _ bool) error {
	if t.sendErr != nil {
		return t.sendErr
	}
	t.sent = append(t.sent, text)
	return nil
}

func (t *fakeTmux) CapturePane(context.Context, string, int) (string, error) {
	if t.captureErr != nil {
		return "", t.captureErr
	}
	return t.captured, nil
}

func (t *fakeTmux) CurrentPaneID(context.Context) (string, error) {
	if strings.TrimSpace(t.currentPaneID) != "" {
		return t.currentPaneID, nil
	}
	return "%1", nil
}
func (t *fakeTmux) ListPanes(context.Context, string) ([]string, error) {
	if len(t.panes) > 0 {
		out := make([]string, len(t.panes))
		copy(out, t.panes)
		return out, nil
	}
	return []string{"%1", "%2"}, nil
}
func (t *fakeTmux) SetWindowOptionsForSidebar(context.Context, string, int) error {
	t.sidebarSetCalls++
	return nil
}
func (t *fakeTmux) SetPaneBorderStatus(_ context.Context, _ string, status string) error {
	t.borderStatuses = append(t.borderStatuses, strings.TrimSpace(status))
	return nil
}
func (t *fakeTmux) SelectLayoutMainVertical(context.Context, string) error {
	t.layouts = append(t.layouts, "main-vertical")
	return nil
}
func (t *fakeTmux) SelectLayout(_ context.Context, _ string, layout string) error {
	t.layouts = append(t.layouts, layout)
	if t.layoutErr != nil {
		return t.layoutErr
	}
	return nil
}
func (t *fakeTmux) SetBuffer(_ context.Context, _ string, content string) error {
	if t.bufferErr != nil {
		return t.bufferErr
	}
	t.pasted = append(t.pasted, content)
	return nil
}
func (t *fakeTmux) PasteBuffer(context.Context, string, string) error { return nil }
func (t *fakeTmux) DeleteBuffer(context.Context, string) error        { return nil }
func (t *fakeTmux) GetPaneCurrentCommand(_ context.Context, paneID string) (string, error) {
	if t.currentCmdErr != nil {
		return "", t.currentCmdErr
	}
	if t.currentByPane != nil {
		if cmd, ok := t.currentByPane[strings.TrimSpace(paneID)]; ok {
			return cmd, nil
		}
		if cmd, ok := t.currentByPane["*"]; ok {
			return cmd, nil
		}
	}
	if strings.TrimSpace(t.currentCommand) != "" {
		return t.currentCommand, nil
	}
	return "codex", nil
}
func (t *fakeTmux) GetWindowDimensions(context.Context) (int, int, error) {
	if t.windowWidth > 0 && t.windowHeight > 0 {
		return t.windowWidth, t.windowHeight, nil
	}
	if t.terminalWidth > 0 && t.terminalHeight > 0 {
		return t.terminalWidth, t.terminalHeight, nil
	}
	return 180, 50, nil
}
func (t *fakeTmux) GetTerminalDimensions(context.Context) (int, int, error) {
	if t.terminalWidth > 0 && t.terminalHeight > 0 {
		return t.terminalWidth, t.terminalHeight, nil
	}
	if t.windowWidth > 0 && t.windowHeight > 0 {
		return t.windowWidth, t.windowHeight, nil
	}
	return 180, 50, nil
}
func (t *fakeTmux) SetWindowSizeManual(_ context.Context, _ string, width, height int) error {
	t.windowSizeCalls++
	t.windowWidth = width
	t.windowHeight = height
	return nil
}
func (t *fakeTmux) SetPaneTitle(_ context.Context, paneID, title string) error {
	if t.titleErr != nil {
		return t.titleErr
	}
	if t.titles == nil {
		t.titles = map[string]string{}
	}
	t.titles[paneID] = title
	return nil
}
func (t *fakeTmux) GetPaneTitle(_ context.Context, paneID string) (string, error) {
	if t.titles == nil {
		return "", nil
	}
	return t.titles[paneID], nil
}
func (t *fakeTmux) KillPane(_ context.Context, paneID string) error {
	t.killed = append(t.killed, paneID)
	trimmed := strings.TrimSpace(paneID)
	if trimmed == "" {
		return nil
	}
	next := make([]string, 0, len(t.panes))
	for _, pane := range t.panes {
		if strings.TrimSpace(pane) != trimmed {
			next = append(next, pane)
		}
	}
	t.panes = next
	delete(t.titles, trimmed)
	if strings.TrimSpace(t.currentPaneID) == trimmed {
		t.currentPaneID = ""
	}
	if t.currentByPane != nil {
		delete(t.currentByPane, trimmed)
	}
	return nil
}

func (t *fakeTmux) paneCommandError(err error) {
	if err == nil {
		err = errors.New("pane command error")
	}
	t.sendErr = err
}

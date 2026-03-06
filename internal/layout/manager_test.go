package layout

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeTmux struct {
	panes          []string
	titles         map[string]string
	terminalW      int
	terminalH      int
	windowW        int
	windowH        int
	layouts        []string
	resized        [][2]int
	splitCalls     int
	setTitleCalls  int
	killCalls      int
	borderStatuses []string
	splitErr       error
	layoutErr      error
	borderErr      error
}

func (f *fakeTmux) ListPanes(context.Context, string) ([]string, error) {
	out := make([]string, len(f.panes))
	copy(out, f.panes)
	return out, nil
}

func (f *fakeTmux) GetPaneTitle(_ context.Context, paneID string) (string, error) {
	return f.titles[paneID], nil
}

func (f *fakeTmux) GetTerminalDimensions(context.Context) (int, int, error) {
	return f.terminalW, f.terminalH, nil
}

func (f *fakeTmux) GetWindowDimensions(context.Context) (int, int, error) {
	return f.windowW, f.windowH, nil
}

func (f *fakeTmux) SetWindowSizeManual(_ context.Context, _ string, width, height int) error {
	f.resized = append(f.resized, [2]int{width, height})
	f.windowW = width
	f.windowH = height
	return nil
}

func (f *fakeTmux) SelectLayout(_ context.Context, _ string, layout string) error {
	f.layouts = append(f.layouts, layout)
	if f.layoutErr != nil {
		return f.layoutErr
	}
	return nil
}

func (f *fakeTmux) SetPaneBorderStatus(_ context.Context, _ string, status string) error {
	if f.borderErr != nil {
		return f.borderErr
	}
	f.borderStatuses = append(f.borderStatuses, strings.TrimSpace(status))
	return nil
}

func (f *fakeTmux) SplitPaneOnTargetWithCommand(_ context.Context, _, _, target, command string) (string, error) {
	f.splitCalls++
	if f.splitErr != nil {
		return "", f.splitErr
	}
	if strings.TrimSpace(target) == "" {
		return "", nil
	}
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	id := "%9"
	f.panes = append(f.panes, id)
	return id, nil
}

func (f *fakeTmux) SetPaneTitle(_ context.Context, paneID, title string) error {
	f.setTitleCalls++
	if f.titles == nil {
		f.titles = map[string]string{}
	}
	f.titles[paneID] = title
	return nil
}

func (f *fakeTmux) KillPane(_ context.Context, paneID string) error {
	f.killCalls++
	next := make([]string, 0, len(f.panes))
	for _, pane := range f.panes {
		if pane != paneID {
			next = append(next, pane)
		}
	}
	f.panes = next
	delete(f.titles, paneID)
	return nil
}

func TestManagerRecalculateAppliesCustomLayout(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2", "%3"},
		titles:    map[string]string{},
		terminalW: 220,
		terminalH: 50,
		windowW:   220,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})
	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if len(tmux.layouts) == 0 {
		t.Fatalf("expected layout to be applied")
	}
}

func TestManagerRecalculateCreatesSpacerWhenNeeded(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2", "%3", "%4"},
		titles:    map[string]string{},
		terminalW: 180,
		terminalH: 50,
		windowW:   180,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})
	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if tmux.splitCalls == 0 {
		t.Fatalf("expected spacer pane split")
	}
	if tmux.setTitleCalls == 0 {
		t.Fatalf("expected spacer pane title set")
	}
}

func TestManagerRecalculateReusesExistingSpacerWhenStillNeeded(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes: []string{"%1", "%2", "%3", "%4", "%9"},
		titles: map[string]string{
			"%9": SpacerPaneTitle,
		},
		terminalW: 180,
		terminalH: 50,
		windowW:   180,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if tmux.splitCalls != 0 {
		t.Fatalf("expected no spacer split when spacer already exists, got %d", tmux.splitCalls)
	}
	if tmux.killCalls != 0 {
		t.Fatalf("expected no spacer kill when spacer still needed, got %d", tmux.killCalls)
	}
}

func TestManagerRecalculateUsesSidebarOnlyWhenNoContentPanes(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1"},
		titles:    map[string]string{},
		terminalW: 200,
		terminalH: 50,
		windowW:   200,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if tmux.splitCalls != 0 {
		t.Fatalf("split calls = %d, want 0", tmux.splitCalls)
	}
	if len(tmux.layouts) != 0 {
		t.Fatalf("expected no layout application for sidebar-only mode, got %#v", tmux.layouts)
	}
	if len(tmux.resized) != 1 {
		t.Fatalf("expected one resize to sidebar width, got %#v", tmux.resized)
	}
	if got := tmux.resized[0]; got != [2]int{40, 50} {
		t.Fatalf("resize = %#v, want %#v", got, [2]int{40, 50})
	}
}

func TestManagerRecalculateRemovesIdlePaneWhenRealContentExists(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes: []string{"%1", "%2", "%9"},
		titles: map[string]string{
			"%9": IdlePaneTitle,
		},
		terminalW: 200,
		terminalH: 50,
		windowW:   200,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if tmux.killCalls != 1 {
		t.Fatalf("kill calls = %d, want 1", tmux.killCalls)
	}
	if tmux.splitCalls != 0 {
		t.Fatalf("split calls = %d, want 0", tmux.splitCalls)
	}
	if _, ok := tmux.titles["%9"]; ok {
		t.Fatalf("idle pane title should be removed after kill")
	}
	if len(tmux.layouts) == 0 {
		t.Fatalf("expected layout to be applied")
	}
}

func TestManagerRecalculateRestoresSidebarOnlyWidthDespiteCache(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1"},
		titles:    map[string]string{},
		terminalW: 200,
		terminalH: 50,
		windowW:   40,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("first recalculate: %v", err)
	}
	if len(tmux.resized) != 0 {
		t.Fatalf("expected no resize on matching sidebar-only width, got %#v", tmux.resized)
	}
	tmux.windowW = 200

	if err := mgr.Recalculate(context.Background(), "%1", false); err != nil {
		t.Fatalf("second recalculate: %v", err)
	}
	if len(tmux.resized) != 1 {
		t.Fatalf("expected one resize on drifted sidebar-only width, got %#v", tmux.resized)
	}
	if got := tmux.resized[0]; got != [2]int{40, 50} {
		t.Fatalf("resize = %#v, want %#v", got, [2]int{40, 50})
	}
}

func TestManagerRecalculateDoesNotCreateIdlePaneWhenNoContentPanes(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1"},
		titles:    map[string]string{},
		terminalW: 200,
		terminalH: 50,
		windowW:   200,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if tmux.splitCalls != 0 {
		t.Fatalf("expected no split for sidebar-only mode, got %d", tmux.splitCalls)
	}
}

func TestManagerRecalculateFailsWhenSelectLayoutFailsWithoutFallback(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2"},
		titles:    map[string]string{},
		terminalW: 200,
		terminalH: 50,
		windowW:   200,
		windowH:   50,
		layoutErr: errors.New("layout failed"),
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	err := mgr.Recalculate(context.Background(), "%1", true)
	if err == nil {
		t.Fatalf("expected recalculate error when select-layout fails")
	}
	if !strings.Contains(err.Error(), "apply layout") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tmux.layouts) != 1 {
		t.Fatalf("expected single select-layout attempt, got %d", len(tmux.layouts))
	}
}

func TestManagerRecalculateUsesCappedLayoutWidthForSidebarLayout(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2"},
		titles:    map[string]string{},
		terminalW: 240,
		terminalH: 50,
		windowW:   135,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       55,
		MinPaneWidth:       60,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if len(tmux.layouts) == 0 {
		t.Fatalf("expected layout to be applied")
	}
	if !strings.Contains(tmux.layouts[0], ",135x50,0,0{") {
		t.Fatalf("layout should use capped window width, got %q", tmux.layouts[0])
	}
}

func TestManagerRecalculateResizesToCappedLayoutWidthWhenWindowChanges(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2"},
		titles:    map[string]string{},
		terminalW: 240,
		terminalH: 50,
		windowW:   135,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       55,
		MinPaneWidth:       60,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("first recalculate: %v", err)
	}
	if len(tmux.resized) != 0 {
		t.Fatalf("expected no resize on matching window size, got %#v", tmux.resized)
	}

	tmux.windowW = 180
	tmux.windowH = 50
	if err := mgr.Recalculate(context.Background(), "%1", false); err != nil {
		t.Fatalf("second recalculate: %v", err)
	}
	if len(tmux.resized) != 1 {
		t.Fatalf("expected one resize after shrink, got %#v", tmux.resized)
	}
	if got := tmux.resized[0]; got != [2]int{135, 50} {
		t.Fatalf("resize = %#v, want %#v", got, [2]int{135, 50})
	}
}

func TestManagerRecalculateSetsPaneBorderStatusOffEachRun(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2"},
		titles:    map[string]string{},
		terminalW: 220,
		terminalH: 50,
		windowW:   220,
		windowH:   50,
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       55,
		MinPaneWidth:       60,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	if err := mgr.Recalculate(context.Background(), "%1", true); err != nil {
		t.Fatalf("first recalculate: %v", err)
	}
	if err := mgr.Recalculate(context.Background(), "%1", false); err != nil {
		t.Fatalf("second recalculate: %v", err)
	}
	if got := len(tmux.borderStatuses); got != 2 {
		t.Fatalf("border status calls = %d, want 2", got)
	}
	if tmux.borderStatuses[0] != "off" || tmux.borderStatuses[1] != "off" {
		t.Fatalf("unexpected border statuses: %#v", tmux.borderStatuses)
	}
}

func TestManagerRecalculateFailsWhenPaneBorderStatusSetFails(t *testing.T) {
	t.Parallel()

	tmux := &fakeTmux{
		panes:     []string{"%1", "%2"},
		titles:    map[string]string{},
		terminalW: 220,
		terminalH: 50,
		windowW:   220,
		windowH:   50,
		borderErr: errors.New("status failed"),
	}
	mgr := NewManager(tmux, Config{
		SidebarWidth:       55,
		MinPaneWidth:       60,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	})

	err := mgr.Recalculate(context.Background(), "%1", true)
	if err == nil {
		t.Fatalf("expected recalculate error")
	}
	if !strings.Contains(err.Error(), "set pane border status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

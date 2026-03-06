package layout

import (
	"context"
	"strings"
	"testing"
)

type fakeTmux struct {
	panes         []string
	titles        map[string]string
	terminalW     int
	terminalH     int
	windowW       int
	windowH       int
	layouts       []string
	resized       [][2]int
	splitCalls    int
	setTitleCalls int
	killCalls     int
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
	return nil
}

func (f *fakeTmux) SetWindowOptionsForSidebar(context.Context, string, int) error {
	return nil
}

func (f *fakeTmux) SplitPaneOnTargetWithCommand(_ context.Context, _, _, target, command string) (string, error) {
	f.splitCalls++
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

package layout

import (
	"context"
	"strings"
	"sync"
)

type TmuxClient interface {
	ListPanes(ctx context.Context, target string) ([]string, error)
	GetPaneTitle(ctx context.Context, paneID string) (string, error)
	GetTerminalDimensions(ctx context.Context) (int, int, error)
	GetWindowDimensions(ctx context.Context) (int, int, error)
	SetWindowSizeManual(ctx context.Context, target string, width, height int) error
	SelectLayout(ctx context.Context, target, layout string) error
	SetWindowOptionsForSidebar(ctx context.Context, target string, controlWidth int) error
	SplitPaneOnTargetWithCommand(ctx context.Context, direction, cwd, target, command string) (string, error)
	SetPaneTitle(ctx context.Context, paneID, title string) error
	KillPane(ctx context.Context, paneID string) error
}

type Manager struct {
	tmux TmuxClient
	cfg  Config

	mu   sync.Mutex
	last layoutCache
}

type layoutCache struct {
	valid       bool
	terminalW   int
	terminalH   int
	contentPane int
	minPaneW    int
	maxPaneW    int
}

func NewManager(tmux TmuxClient, cfg Config) *Manager {
	return &Manager{
		tmux: tmux,
		cfg:  cfg,
	}
}

func (m *Manager) Recalculate(ctx context.Context, controlPaneID string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	controlPaneID = strings.TrimSpace(controlPaneID)
	if controlPaneID == "" || m.tmux == nil {
		return nil
	}

	allPanes, err := m.tmux.ListPanes(ctx, "")
	if err != nil {
		return nil
	}
	if !containsPane(allPanes, controlPaneID) {
		return nil
	}

	existingSpacerID := ""
	for _, paneID := range allPanes {
		title, titleErr := m.tmux.GetPaneTitle(ctx, paneID)
		if titleErr == nil && title == SpacerPaneTitle {
			existingSpacerID = paneID
			break
		}
	}

	realContentPanes := make([]string, 0, len(allPanes))
	for _, paneID := range allPanes {
		if paneID == controlPaneID || paneID == existingSpacerID {
			continue
		}
		realContentPanes = append(realContentPanes, paneID)
	}

	terminalW, terminalH, err := m.tmux.GetTerminalDimensions(ctx)
	if err != nil || terminalW <= 0 || terminalH <= 0 {
		windowW, windowH, windowErr := m.tmux.GetWindowDimensions(ctx)
		if windowErr != nil || windowW <= 0 || windowH <= 0 {
			return nil
		}
		terminalW, terminalH = windowW, windowH
	}

	if !force && m.last.valid &&
		m.last.terminalW == terminalW &&
		m.last.terminalH == terminalH &&
		m.last.contentPane == len(realContentPanes) &&
		m.last.minPaneW == m.cfg.MinPaneWidth &&
		m.last.maxPaneW == m.cfg.MaxPaneWidth {
		return nil
	}

	baseLayout := CalculateOptimalLayout(len(realContentPanes), terminalW, terminalH, m.cfg)
	needsSpacer := NeedsSpacerPane(len(realContentPanes), baseLayout, m.cfg)

	if existingSpacerID != "" {
		_ = m.tmux.KillPane(ctx, existingSpacerID)
	}

	spacerID := ""
	if needsSpacer && len(realContentPanes) > 0 {
		lastContentPaneID := realContentPanes[len(realContentPanes)-1]
		newPaneID, splitErr := m.tmux.SplitPaneOnTargetWithCommand(ctx, "right", "", lastContentPaneID, "cat")
		if splitErr == nil && strings.TrimSpace(newPaneID) != "" {
			spacerID = strings.TrimSpace(newPaneID)
			_ = m.tmux.SetPaneTitle(ctx, spacerID, SpacerPaneTitle)
		}
	}

	finalContentPanes := make([]string, 0, len(realContentPanes)+1)
	finalContentPanes = append(finalContentPanes, realContentPanes...)
	if spacerID != "" {
		finalContentPanes = append(finalContentPanes, spacerID)
	}

	finalLayout := baseLayout
	if spacerID != "" {
		finalLayout = CalculateOptimalLayout(len(finalContentPanes), terminalW, terminalH, m.cfg)
	}

	if currentW, currentH, wErr := m.tmux.GetWindowDimensions(ctx); wErr == nil {
		if currentW != finalLayout.WindowWidth || currentH != terminalH {
			_ = m.tmux.SetWindowSizeManual(ctx, "", finalLayout.WindowWidth, terminalH)
		}
	}

	if len(finalContentPanes) == 0 {
		_ = m.applyMainVerticalFallback(ctx)
		m.updateCache(terminalW, terminalH, len(realContentPanes))
		return nil
	}

	layoutString := GenerateSidebarGridLayout(
		controlPaneID,
		finalContentPanes,
		m.cfg.SidebarWidth,
		finalLayout.WindowWidth,
		terminalH,
		finalLayout.Cols,
		m.cfg.MaxPaneWidth,
		func(paneID string) bool { return paneID == spacerID },
	)
	if strings.TrimSpace(layoutString) == "" {
		_ = m.applyMainVerticalFallback(ctx)
		m.updateCache(terminalW, terminalH, len(realContentPanes))
		return nil
	}
	if err := m.tmux.SelectLayout(ctx, "", layoutString); err != nil {
		_ = m.applyMainVerticalFallback(ctx)
	}

	m.updateCache(terminalW, terminalH, len(realContentPanes))
	return nil
}

func (m *Manager) applyMainVerticalFallback(ctx context.Context) error {
	if err := m.tmux.SetWindowOptionsForSidebar(ctx, "", m.cfg.SidebarWidth); err != nil {
		return err
	}
	return m.tmux.SelectLayout(ctx, "", "main-vertical")
}

func (m *Manager) updateCache(terminalW, terminalH, contentPane int) {
	m.last = layoutCache{
		valid:       true,
		terminalW:   terminalW,
		terminalH:   terminalH,
		contentPane: contentPane,
		minPaneW:    m.cfg.MinPaneWidth,
		maxPaneW:    m.cfg.MaxPaneWidth,
	}
}

func containsPane(panes []string, target string) bool {
	for _, paneID := range panes {
		if paneID == target {
			return true
		}
	}
	return false
}

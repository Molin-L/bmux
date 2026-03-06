package layout

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type TmuxClient interface {
	ListPanes(ctx context.Context, target string) ([]string, error)
	GetPaneTitle(ctx context.Context, paneID string) (string, error)
	GetTerminalDimensions(ctx context.Context) (int, int, error)
	GetWindowDimensions(ctx context.Context) (int, int, error)
	SetPaneBorderStatus(ctx context.Context, target, status string) error
	SetWindowSizeManual(ctx context.Context, target string, width, height int) error
	SelectLayout(ctx context.Context, target, layout string) error
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
	if err := m.tmux.SetPaneBorderStatus(ctx, controlPaneID, "off"); err != nil {
		return fmt.Errorf("enforce sidebar layout: set pane border status: %w", err)
	}

	existingSpacerID := ""
	existingIdleID := ""
	for _, paneID := range allPanes {
		title, titleErr := m.tmux.GetPaneTitle(ctx, paneID)
		if titleErr != nil {
			continue
		}
		switch strings.TrimSpace(title) {
		case SpacerPaneTitle:
			if existingSpacerID == "" {
				existingSpacerID = paneID
			}
		case IdlePaneTitle:
			if existingIdleID == "" {
				existingIdleID = paneID
			}
		}
	}

	realContentPanes := make([]string, 0, len(allPanes))
	for _, paneID := range allPanes {
		if paneID == controlPaneID || paneID == existingSpacerID || paneID == existingIdleID {
			continue
		}
		realContentPanes = append(realContentPanes, paneID)
	}
	idlePaneRemoved := false
	if strings.TrimSpace(existingIdleID) != "" {
		_ = m.tmux.KillPane(ctx, existingIdleID)
		existingIdleID = ""
		idlePaneRemoved = true
	}

	terminalW, terminalH, err := m.tmux.GetTerminalDimensions(ctx)
	if err != nil || terminalW <= 0 || terminalH <= 0 {
		windowW, windowH, windowErr := m.tmux.GetWindowDimensions(ctx)
		if windowErr != nil || windowW <= 0 || windowH <= 0 {
			return nil
		}
		terminalW, terminalH = windowW, windowH
	}

	baseLayout := CalculateOptimalLayout(len(realContentPanes), terminalW, terminalH, m.cfg)
	needsSpacer := NeedsSpacerPane(len(realContentPanes), baseLayout, m.cfg)
	spacerChanged := false

	if existingSpacerID != "" && !needsSpacer {
		_ = m.tmux.KillPane(ctx, existingSpacerID)
		existingSpacerID = ""
		spacerChanged = true
	}

	spacerID := ""
	if needsSpacer && len(realContentPanes) > 0 {
		if strings.TrimSpace(existingSpacerID) != "" {
			spacerID = strings.TrimSpace(existingSpacerID)
		} else {
			lastContentPaneID := realContentPanes[len(realContentPanes)-1]
			newPaneID, splitErr := m.tmux.SplitPaneOnTargetWithCommand(ctx, "right", "", lastContentPaneID, "cat")
			if splitErr == nil && strings.TrimSpace(newPaneID) != "" {
				spacerID = strings.TrimSpace(newPaneID)
				_ = m.tmux.SetPaneTitle(ctx, spacerID, SpacerPaneTitle)
				spacerChanged = true
			}
		}
	}

	finalContentPanes := make([]string, 0, len(realContentPanes)+1)
	finalContentPanes = append(finalContentPanes, realContentPanes...)
	if spacerID != "" {
		finalContentPanes = append(finalContentPanes, spacerID)
	}

	finalLayout := baseLayout
	if len(finalContentPanes) != len(realContentPanes) {
		finalLayout = CalculateOptimalLayout(len(finalContentPanes), terminalW, terminalH, m.cfg)
	}

	currentW, currentH, windowErr := m.tmux.GetWindowDimensions(ctx)
	targetWindowWidth := finalLayout.WindowWidth
	if len(finalContentPanes) == 0 {
		targetWindowWidth = m.cfg.SidebarWidth
	}
	windowSizeMismatch := windowErr == nil && currentW > 0 && currentH > 0 && (currentW != targetWindowWidth || currentH != terminalH)
	if !force && m.last.valid &&
		m.last.terminalW == terminalW &&
		m.last.terminalH == terminalH &&
		m.last.contentPane == len(realContentPanes) &&
		m.last.minPaneW == m.cfg.MinPaneWidth &&
		m.last.maxPaneW == m.cfg.MaxPaneWidth &&
		!idlePaneRemoved &&
		!spacerChanged &&
		!windowSizeMismatch {
		return nil
	}
	if windowSizeMismatch {
		_ = m.tmux.SetWindowSizeManual(ctx, "", targetWindowWidth, terminalH)
	}

	if len(finalContentPanes) == 0 {
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
		return errors.New("enforce sidebar layout: generated layout is empty")
	}
	if err := m.tmux.SelectLayout(ctx, "", layoutString); err != nil {
		return fmt.Errorf("enforce sidebar layout: apply layout: %w", err)
	}

	m.updateCache(terminalW, terminalH, len(realContentPanes))
	return nil
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

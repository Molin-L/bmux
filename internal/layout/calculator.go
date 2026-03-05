package layout

import "math"

func CalculateOptimalLayout(numContentPanes, terminalWidth, terminalHeight int, cfg Config) Layout {
	minFeasiblePaneWidth := max(1, min(cfg.MinPaneWidth, cfg.MaxPaneWidth))

	if numContentPanes == 0 {
		return Layout{
			Cols:             0,
			Rows:             0,
			WindowWidth:      terminalWidth,
			PaneDistribution: []int{},
			ActualPaneWidth:  0,
		}
	}

	best := Layout{}
	bestScore := -1.0
	found := false

	for cols := numContentPanes; cols >= 1; cols-- {
		rows := int(math.Ceil(float64(numContentPanes) / float64(cols)))
		columnBorders := cols - 1
		rowBorders := rows - 1

		minRequiredWidth := cfg.SidebarWidth + cols*minFeasiblePaneWidth + columnBorders
		minRequiredHeight := rows*cfg.MinPaneHeight + rowBorders
		if minRequiredWidth > terminalWidth || minRequiredHeight > terminalHeight {
			continue
		}

		idealMaxWidth := cfg.SidebarWidth + cols*cfg.MaxPaneWidth + columnBorders
		windowWidth := min(idealMaxWidth, terminalWidth)
		effectiveContentWidth := windowWidth - cfg.SidebarWidth - columnBorders
		actualPaneWidth := float64(effectiveContentWidth) / float64(cols)
		distribution := distributePanes(numContentPanes, cols)

		availableHeight := terminalHeight - rowBorders
		paneHeight := availableHeight / rows
		score := scoreLayout(numContentPanes, cols, actualPaneWidth, paneHeight, terminalHeight, cfg.MaxPaneWidth)

		if !found || score > bestScore || (score == bestScore && cols < best.Cols) {
			found = true
			bestScore = score
			best = Layout{
				Cols:             cols,
				Rows:             rows,
				WindowWidth:      windowWidth,
				PaneDistribution: distribution,
				ActualPaneWidth:  actualPaneWidth,
			}
		}
	}

	if found {
		return best
	}

	return Layout{
		Cols:             1,
		Rows:             numContentPanes,
		WindowWidth:      terminalWidth,
		PaneDistribution: []int{numContentPanes},
		ActualPaneWidth:  float64(terminalWidth - cfg.SidebarWidth),
	}
}

func NeedsSpacerPane(numContentPanes int, layout Layout, cfg Config) bool {
	if layout.Cols <= 1 || numContentPanes <= 0 {
		return false
	}

	panesInLastRow := numContentPanes % layout.Cols
	if panesInLastRow == 0 {
		panesInLastRow = layout.Cols
	}
	if panesInLastRow == layout.Cols {
		return false
	}

	contentWidth := layout.WindowWidth - cfg.SidebarWidth - 1
	bordersInLastRow := panesInLastRow - 1
	availableWidth := contentWidth - bordersInLastRow
	widthPerPane := float64(availableWidth) / float64(panesInLastRow)
	if widthPerPane <= float64(cfg.MaxPaneWidth) {
		return false
	}

	contentPaneWidth := cfg.MaxPaneWidth
	totalBorders := panesInLastRow
	totalContentWidth := panesInLastRow * contentPaneWidth
	spacerWidth := contentWidth - totalContentWidth - totalBorders
	return spacerWidth >= cfg.MinSpacerPaneWidth
}

func distributePanes(numPanes, cols int) []int {
	distribution := make([]int, 0, cols)
	basePerCol := numPanes / cols
	remainder := numPanes % cols
	for i := 0; i < cols; i++ {
		count := basePerCol
		if i < remainder {
			count++
		}
		distribution = append(distribution, count)
	}
	return distribution
}

func scoreLayout(numContentPanes, cols int, actualPaneWidth float64, paneHeight, terminalHeight, maxComfortableWidth int) float64 {
	panesInLastRow := numContentPanes % cols
	if panesInLastRow == 0 {
		panesInLastRow = cols
	}
	balanceScore := 1.0
	if panesInLastRow == 1 {
		balanceScore = 0.5
	}
	heightScore := float64(paneHeight) / float64(terminalHeight)
	widthScore := 1.0
	if actualPaneWidth > float64(maxComfortableWidth) {
		widthScore = 0.8
	}
	return balanceScore * heightScore * widthScore
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

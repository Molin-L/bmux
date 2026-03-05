package layout

type Config struct {
	SidebarWidth       int
	MinPaneWidth       int
	MaxPaneWidth       int
	MinPaneHeight      int
	MinSpacerPaneWidth int
}

type Layout struct {
	Cols             int
	Rows             int
	WindowWidth      int
	PaneDistribution []int
	ActualPaneWidth  float64
}

const SpacerPaneTitle = "bmux-spacer"

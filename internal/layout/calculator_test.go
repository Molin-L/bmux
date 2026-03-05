package layout

import "testing"

func TestCalculateOptimalLayoutPrefersWiderGridWhenItFits(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	}
	got := CalculateOptimalLayout(3, 300, 50, cfg)
	if got.Cols != 3 {
		t.Fatalf("cols = %d, want 3", got.Cols)
	}
	if got.Rows != 1 {
		t.Fatalf("rows = %d, want 1", got.Rows)
	}
}

func TestNeedsSpacerPaneForWideOrphanLastRow(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SidebarWidth:       40,
		MinPaneWidth:       50,
		MaxPaneWidth:       80,
		MinPaneHeight:      15,
		MinSpacerPaneWidth: 20,
	}
	layout := Layout{
		Cols:        2,
		Rows:        2,
		WindowWidth: 180,
	}

	if !NeedsSpacerPane(3, layout, cfg) {
		t.Fatalf("expected spacer pane to be required")
	}
}

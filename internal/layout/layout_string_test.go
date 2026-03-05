package layout

import (
	"strings"
	"testing"
)

func TestGenerateSidebarGridLayoutIncludesChecksum(t *testing.T) {
	t.Parallel()

	content := []string{"%2", "%3", "%4"}
	got := GenerateSidebarGridLayout("%1", content, 40, 180, 50, 2, 80, func(string) bool { return false })
	if got == "" {
		t.Fatalf("layout string is empty")
	}
	parts := strings.SplitN(got, ",", 2)
	if len(parts) != 2 {
		t.Fatalf("layout missing checksum prefix: %q", got)
	}
	if len(parts[0]) != 4 {
		t.Fatalf("checksum length = %d, want 4", len(parts[0]))
	}
	if !strings.Contains(parts[1], "{") {
		t.Fatalf("layout body missing tree structure: %q", parts[1])
	}
}

func TestCalculateLayoutChecksumStableLength(t *testing.T) {
	t.Parallel()

	got := CalculateLayoutChecksum("100x40,0,0{40x40,0,0,1,59x40,41,0,2}")
	if len(got) != 4 {
		t.Fatalf("checksum length = %d, want 4", len(got))
	}
}

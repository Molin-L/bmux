package tui

import (
	"strings"
	"testing"
)

func TestWrapWithPrefixesWrapsWithoutTruncation(t *testing.T) {
	t.Parallel()
	lines := wrapWithPrefixes("one two three four five", "m: ", "   ", 10)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %v", lines)
	}
	if strings.Contains(strings.Join(lines, "\n"), "…") {
		t.Fatalf("unexpected truncation: %v", lines)
	}
}

func TestWrapTextHardWrapLongToken(t *testing.T) {
	t.Parallel()
	token := "abcdefghijklmnopqrstuvwxyz0123456789"
	lines := wrapText(token, 7, 7)
	if len(lines) < 2 {
		t.Fatalf("expected hard wrapped token, got %v", lines)
	}
	if got := strings.Join(lines, ""); got != token {
		t.Fatalf("hard wrap changed content: %q != %q", got, token)
	}
}

func TestWrapWithPrefixesContinuationAlignment(t *testing.T) {
	t.Parallel()
	lines := wrapWithPrefixes("alpha beta gamma delta epsilon", "  branch: ", "          ", 20)
	if len(lines) < 2 {
		t.Fatalf("expected continuation lines, got %v", lines)
	}
	if !strings.HasPrefix(lines[1], "          ") {
		t.Fatalf("expected continuation prefix, got %q", lines[1])
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/Molin-L/bmux/internal/waitpane"
)

func TestRunMainWaitBlockedDispatchesAndBypassesDefault(t *testing.T) {
	t.Parallel()
	calledWait := 0
	calledDefault := 0
	var got waitpane.Options

	err := runMain([]string{
		"--wait-blocked",
		"--issue-id", "bd-1",
		"--blocked-by", "bd-0",
		"--branch", "task/bd-1",
	}, mainDeps{
		runWaitPane: func(opts waitpane.Options) error {
			calledWait++
			got = opts
			return nil
		},
		runDefault: func([]string) error {
			calledDefault++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runMain: %v", err)
	}
	if calledWait != 1 {
		t.Fatalf("waitpane calls = %d, want 1", calledWait)
	}
	if calledDefault != 0 {
		t.Fatalf("default calls = %d, want 0", calledDefault)
	}
	if got.IssueID != "bd-1" || got.BlockedBy != "bd-0" || got.Branch != "task/bd-1" {
		t.Fatalf("unexpected waitpane options: %+v", got)
	}
}

func TestRunMainWaitBlockedMissingArgsReturnsError(t *testing.T) {
	t.Parallel()
	err := runMain([]string{"--wait-blocked", "--issue-id", "bd-1"}, mainDeps{
		runWaitPane: func(waitpane.Options) error { return nil },
		runDefault:  func([]string) error { return nil },
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "--blocked-by") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMainWaitBlockedAcceptsBlockedByAlias(t *testing.T) {
	t.Parallel()
	var got waitpane.Options
	err := runMain([]string{
		"--wait-blocked",
		"--issue-id", "bd-2",
		"--blockedby", "bd-1",
	}, mainDeps{
		runWaitPane: func(opts waitpane.Options) error {
			got = opts
			return nil
		},
		runDefault: func([]string) error { return nil },
	})
	if err != nil {
		t.Fatalf("runMain: %v", err)
	}
	if got.BlockedBy != "bd-1" {
		t.Fatalf("blockedBy = %q", got.BlockedBy)
	}
}

func TestRunMainNonWaitModeUsesDefaultPath(t *testing.T) {
	t.Parallel()
	calledWait := 0
	calledDefault := 0
	err := runMain([]string{"--some-flag", "x"}, mainDeps{
		runWaitPane: func(waitpane.Options) error {
			calledWait++
			return nil
		},
		runDefault: func(args []string) error {
			calledDefault++
			if len(args) != 2 || args[0] != "--some-flag" || args[1] != "x" {
				t.Fatalf("unexpected args: %#v", args)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runMain: %v", err)
	}
	if calledWait != 0 {
		t.Fatalf("waitpane calls = %d, want 0", calledWait)
	}
	if calledDefault != 1 {
		t.Fatalf("default calls = %d, want 1", calledDefault)
	}
}

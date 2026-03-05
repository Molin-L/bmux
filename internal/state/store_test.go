package state_test

import (
	"testing"
	"time"

	"github.com/Molin-L/bmux/internal/model"
	"github.com/Molin-L/bmux/internal/state"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := state.New(root)

	meta := model.TaskBranchMeta{
		IssueID:      "bd-a1",
		Branch:       "task/bd-a1-auth",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
		BaseCommit:   "abc123",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		Status:       "active",
	}
	if err := store.Upsert(meta); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, ok, err := store.ByIssueID("bd-a1")
	if err != nil {
		t.Fatalf("by issue id: %v", err)
	}
	if !ok {
		t.Fatal("expected issue metadata to exist")
	}
	if got.Branch != meta.Branch {
		t.Fatalf("branch = %q, want %q", got.Branch, meta.Branch)
	}
}

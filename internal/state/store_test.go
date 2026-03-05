package state_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestStoreRunRoundTripAndChaosState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := state.New(root)
	now := time.Now().UTC().Truncate(time.Second)
	run := model.TaskRunMeta{
		IssueID:         "bd-a1",
		Mode:            model.RunModeSelfRun,
		Agent:           "codex",
		PaneID:          "%7",
		StartedAt:       now,
		UpdatedAt:       now,
		ExpectedProcess: "codex",
	}
	if err := store.RunLockUpsert(run); err != nil {
		t.Fatalf("run upsert: %v", err)
	}
	got, ok, err := store.RunLockByIssueID("bd-a1")
	if err != nil {
		t.Fatalf("run by issue id: %v", err)
	}
	if !ok {
		t.Fatal("expected run metadata to exist")
	}
	if got.Mode != model.RunModeSelfRun || got.PaneID != "%7" {
		t.Fatalf("unexpected run: %+v", got)
	}

	chaos := model.ChaosState{
		SessionID:       "chaos-1",
		Active:          true,
		PendingIssueIDs: []string{"bd-a1"},
		Blockers:        map[string][]string{"bd-a1": []string{"bd-a0"}},
		StartedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.SetChaosState(&chaos); err != nil {
		t.Fatalf("set chaos state: %v", err)
	}
	gotChaos, ok, err := store.ChaosState()
	if err != nil {
		t.Fatalf("chaos state: %v", err)
	}
	if !ok {
		t.Fatal("expected chaos state")
	}
	if gotChaos.SessionID != "chaos-1" || !gotChaos.Active {
		t.Fatalf("unexpected chaos state: %+v", gotChaos)
	}
}

func TestStoreBackCompatWithoutRuns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stateDir := filepath.Join(root, ".bmux")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	legacy := `{"tasks":{"bd-x":{"issue_id":"bd-x","branch":"task/bd-x","status":"active"}}}`
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	store := state.New(root)
	if _, ok, err := store.ByIssueID("bd-x"); err != nil || !ok {
		t.Fatalf("task lookup failed: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.RunLockByIssueID("bd-x"); err != nil {
		t.Fatalf("run lookup err: %v", err)
	} else if ok {
		t.Fatalf("unexpected run metadata in legacy state")
	}
}

func TestStoreRunBackCompatLegacyStatusFieldsDroppedOnWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stateDir := filepath.Join(root, ".bmux")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	legacy := `{
  "tasks": {},
  "runs": {
    "bd-r1": {
      "issue_id": "bd-r1",
      "mode": "self_run",
      "agent": "codex",
      "pane_id": "%9",
      "status": "completed",
      "completed_at": "2026-03-05T12:00:00Z",
      "error": "legacy field"
    }
  }
}`
	statePath := filepath.Join(stateDir, "state.json")
	if err := os.WriteFile(statePath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	store := state.New(root)
	run, ok, err := store.RunLockByIssueID("bd-r1")
	if err != nil {
		t.Fatalf("run lookup err: %v", err)
	}
	if !ok {
		t.Fatalf("expected legacy run to load")
	}
	if run.Mode != model.RunModeSelfRun || run.PaneID != "%9" {
		t.Fatalf("unexpected run loaded from legacy state: %+v", run)
	}

	run.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := store.RunLockUpsert(run); err != nil {
		t.Fatalf("run upsert: %v", err)
	}
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read rewritten state: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, `"completed_at"`) || strings.Contains(content, `"error"`) {
		t.Fatalf("legacy run status fields should be dropped, got:\n%s", content)
	}
}

package beads

import (
	"context"
	"os/exec"
	"testing"

	"github.com/Molin-L/bmux/internal/errorsx"
)

func TestResolveActorPrefersBDActor(t *testing.T) {
	repo := t.TempDir()
	runOK(t, "git", "-C", repo, "init")
	runOK(t, "git", "-C", repo, "config", "user.name", "Git Actor")
	t.Setenv("BD_ACTOR", "BD Actor")
	t.Setenv("USER", "Shell User")

	got, err := ResolveActor(context.Background(), repo)
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if got != "BD Actor" {
		t.Fatalf("actor = %q", got)
	}
}

func TestResolveActorFallsBackToGitUserName(t *testing.T) {
	repo := t.TempDir()
	runOK(t, "git", "-C", repo, "init")
	runOK(t, "git", "-C", repo, "config", "user.name", "Git Actor")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("USER", "Shell User")

	got, err := ResolveActor(context.Background(), repo)
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if got != "Git Actor" {
		t.Fatalf("actor = %q", got)
	}
}

func TestResolveActorFallsBackToUserEnv(t *testing.T) {
	repo := t.TempDir() + "/missing-repo-root"
	t.Setenv("BD_ACTOR", "")
	t.Setenv("USER", "Shell User")

	got, err := ResolveActor(context.Background(), repo)
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if got != "Shell User" {
		t.Fatalf("actor = %q", got)
	}
}

func TestClaimedByFromErrorParsesCommandStderr(t *testing.T) {
	t.Parallel()
	err := &errorsx.CommandError{
		Command:  "bd",
		Args:     []string{"update", "bd-1", "--claim", "--json"},
		Dir:      "/tmp/repo",
		StdErr:   "Error claiming bd-1: issue already claimed by Molin Liu",
		ExitCode: 1,
	}

	got, ok := ClaimedByFromError(err)
	if !ok {
		t.Fatal("expected claimed-by match")
	}
	if got != "Molin Liu" {
		t.Fatalf("claimed by = %q", got)
	}
}

func runOK(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v (%s)", name, args, err, out)
	}
}

package beads

import (
	"context"
	"testing"

	"github.com/Molin-L/bmux/internal/model"
)

type fakeRunner struct {
	out   string
	err   error
	calls [][]string
}

func (f *fakeRunner) Run(_ context.Context, _ string, _ string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

func TestCreateIssueBuildsArgsWithParent(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{out: `{"id":"bd-1","title":"x"}`}
	c := &Client{repoRoot: t.TempDir(), runner: r}

	got, err := c.CreateIssue(context.Background(), model.CreateIssueRequest{
		Title: "Task", Description: "Desc", Type: "task", Priority: 2, ParentID: "bd-epic",
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if got.ID != "bd-1" {
		t.Fatalf("id = %q", got.ID)
	}
	args := r.calls[0]
	mustContain(t, args, "--parent")
	mustContain(t, args, "bd-epic")
}

func TestCreateIssueParsesArrayJSON(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{out: `[{"id":"bd-2","title":"x"}]`}
	c := &Client{repoRoot: t.TempDir(), runner: r}

	got, err := c.CreateIssue(context.Background(), model.CreateIssueRequest{
		Title: "Epic", Description: "Desc", Type: "epic", Priority: 1,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if got.ID != "bd-2" {
		t.Fatalf("id = %q", got.ID)
	}
}

func TestParseDependencyDirectionOutgoing(t *testing.T) {
	t.Parallel()
	dep := parseDependency("bd-1", map[string]any{
		"type":    "blocks",
		"from_id": "bd-1",
		"to_id":   "bd-2",
	})
	if dep.Direction != "outgoing" {
		t.Fatalf("direction = %q, want outgoing", dep.Direction)
	}
	if dep.IssueID != "bd-2" {
		t.Fatalf("issue id = %q, want bd-2", dep.IssueID)
	}
}

func TestParseDependencyDirectionIncoming(t *testing.T) {
	t.Parallel()
	dep := parseDependency("bd-2", map[string]any{
		"type":    "blocks",
		"from_id": "bd-1",
		"to_id":   "bd-2",
	})
	if dep.Direction != "incoming" {
		t.Fatalf("direction = %q, want incoming", dep.Direction)
	}
	if dep.IssueID != "bd-1" {
		t.Fatalf("issue id = %q, want bd-1", dep.IssueID)
	}
}

func TestClaimBuildsArgs(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{out: `{}`}
	c := &Client{repoRoot: t.TempDir(), runner: r}

	if err := c.Claim(context.Background(), "bd-9"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls = %d", len(r.calls))
	}
	args := r.calls[0]
	want := []string{"update", "bd-9", "--claim", "--json"}
	for _, token := range want {
		mustContain(t, args, token)
	}
}

func TestAddDependencyBuildsArgs(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{out: `{}`}
	c := &Client{repoRoot: t.TempDir(), runner: r}

	if err := c.AddDependency(context.Background(), "bd-42", "bd-41", "blocks"); err != nil {
		t.Fatalf("add dependency: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls = %d", len(r.calls))
	}
	args := r.calls[0]
	want := []string{"dep", "add", "bd-42", "bd-41", "--type", "blocks", "--json"}
	for _, token := range want {
		mustContain(t, args, token)
	}
}

func TestAddDependencyRequiresIDs(t *testing.T) {
	t.Parallel()
	c := &Client{repoRoot: t.TempDir(), runner: &fakeRunner{}}
	if err := c.AddDependency(context.Background(), "", "bd-1", "blocks"); err == nil {
		t.Fatalf("expected validation error")
	}
}

func mustContain(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Fatalf("args %#v do not contain %q", args, want)
}

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

func mustContain(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Fatalf("args %#v do not contain %q", args, want)
}

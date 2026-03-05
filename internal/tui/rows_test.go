package tui

import (
	"testing"

	"github.com/Molin-L/bmux/internal/model"
)

func TestBuildIssueRowsOrdersParentsBeforeChildrenWithinEpic(t *testing.T) {
	t.Parallel()
	rows := buildIssueRows([]model.Issue{
		{ID: "epic.1.2", Title: "child-2", EpicID: "epic", EpicTitle: "Epic", ParentID: "epic.1", HierarchyDepth: 2},
		{ID: "epic.1.1", Title: "child-1", EpicID: "epic", EpicTitle: "Epic", ParentID: "epic.1", HierarchyDepth: 2},
		{ID: "epic.1", Title: "task", EpicID: "epic", EpicTitle: "Epic", ParentID: "epic", HierarchyDepth: 1},
		{ID: "epic", Title: "epic", EpicID: "epic", EpicTitle: "Epic", HierarchyDepth: 0},
	})

	got := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.kind != issueRowIssue {
			continue
		}
		got = append(got, row.issue.ID)
	}

	want := []string{"epic", "epic.1", "epic.1.2", "epic.1.1"}
	if len(got) != len(want) {
		t.Fatalf("got len %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order[%d] = %q, want %q (full=%#v)", i, got[i], want[i], got)
		}
	}
}

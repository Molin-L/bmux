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

func TestBuildIssueRowsGroupsByStatusBucketThenEpicHierarchy(t *testing.T) {
	t.Parallel()
	rows := buildIssueRows([]model.Issue{
		{ID: "bd-closed", Title: "Closed task", Status: "closed", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-open", Title: "Open task", Status: "open", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
		{ID: "bd-inprog", Title: "In progress task", Status: "in_progress", EpicID: "bd-epic", EpicTitle: "Epic", HierarchyDepth: 1},
	})

	gotKinds := make([]issueRowKind, 0, len(rows))
	gotIssueIDs := make([]string, 0, len(rows))
	gotStatusHeaders := make([]string, 0, len(rows))
	for _, row := range rows {
		gotKinds = append(gotKinds, row.kind)
		switch row.kind {
		case issueRowStatusHeader:
			gotStatusHeaders = append(gotStatusHeaders, row.statusLabel)
		case issueRowIssue:
			gotIssueIDs = append(gotIssueIDs, row.issue.ID)
		}
	}

	wantStatusHeaders := []string{"in_progress", "open", "closed"}
	if len(gotStatusHeaders) != len(wantStatusHeaders) {
		t.Fatalf("status header len=%d, want=%d (%#v)", len(gotStatusHeaders), len(wantStatusHeaders), gotStatusHeaders)
	}
	for i := range wantStatusHeaders {
		if gotStatusHeaders[i] != wantStatusHeaders[i] {
			t.Fatalf("status header[%d]=%q, want %q (%#v)", i, gotStatusHeaders[i], wantStatusHeaders[i], gotStatusHeaders)
		}
	}

	wantIssueIDs := []string{"bd-inprog", "bd-open", "bd-closed"}
	if len(gotIssueIDs) != len(wantIssueIDs) {
		t.Fatalf("issue id len=%d, want=%d (%#v)", len(gotIssueIDs), len(wantIssueIDs), gotIssueIDs)
	}
	for i := range wantIssueIDs {
		if gotIssueIDs[i] != wantIssueIDs[i] {
			t.Fatalf("issue order[%d]=%q, want %q (%#v)", i, gotIssueIDs[i], wantIssueIDs[i], gotIssueIDs)
		}
	}
}

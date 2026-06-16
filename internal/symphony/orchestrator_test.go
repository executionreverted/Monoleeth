package symphony

import (
	"testing"
	"time"
)

func TestSortIssuesForDispatch(t *testing.T) {
	high := 1
	low := 3
	now := time.Now()
	older := now.Add(-time.Hour)
	issues := []Issue{
		{Identifier: "GP-3", Priority: &low, UpdatedAt: &older},
		{Identifier: "GP-2", Priority: &high, UpdatedAt: &now},
		{Identifier: "GP-1", Priority: &high, UpdatedAt: &older},
	}
	got := SortIssuesForDispatch(issues)
	if got[0].Identifier != "GP-1" || got[1].Identifier != "GP-2" || got[2].Identifier != "GP-3" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestRequiredLabels(t *testing.T) {
	issue := Issue{Labels: []string{"symphony", "backend"}}
	if !hasRequiredLabels(issue, []string{"Symphony"}) {
		t.Fatalf("expected case-insensitive label match")
	}
	if hasRequiredLabels(issue, []string{"frontend"}) {
		t.Fatalf("did not expect missing label to match")
	}
}

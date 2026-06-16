package symphony

import "testing"

func TestNormalizeLinearIssue(t *testing.T) {
	raw := map[string]any{
		"id":          "issue-id",
		"identifier":  "GP-7",
		"title":       "Fix navigation",
		"description": "Details",
		"priority":    float64(1),
		"state":       map[string]any{"name": "Todo"},
		"branchName":  "caner/gp-7",
		"url":         "https://linear.app/acme/issue/GP-7",
		"assignee":    map[string]any{"id": "user-1"},
		"labels": map[string]any{"nodes": []any{
			map[string]any{"name": "Symphony"},
		}},
		"inverseRelations": map[string]any{"nodes": []any{
			map[string]any{"type": "blocks", "issue": map[string]any{
				"id": "blocker", "identifier": "GP-1", "state": map[string]any{"name": "In Progress"},
			}},
		}},
	}
	issue := normalizeLinearIssue(raw, &assigneeFilter{Values: map[string]bool{"user-1": true}})
	if issue == nil {
		t.Fatal("expected issue")
	}
	if issue.Identifier != "GP-7" || issue.State != "Todo" || issue.Priority == nil || *issue.Priority != 1 {
		t.Fatalf("unexpected issue: %#v", issue)
	}
	if len(issue.Labels) != 1 || issue.Labels[0] != "symphony" {
		t.Fatalf("labels not normalized: %#v", issue.Labels)
	}
	if len(issue.BlockedBy) != 1 || issue.BlockedBy[0].Identifier != "GP-1" {
		t.Fatalf("blockers not extracted: %#v", issue.BlockedBy)
	}
	if !issue.AssignedToWorker {
		t.Fatalf("assignee filter should route issue")
	}
}

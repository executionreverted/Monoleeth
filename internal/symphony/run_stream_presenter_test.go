package symphony

import "testing"

func TestDisplayRunEventsFoldsAgentMessageDeltas(t *testing.T) {
	now := nowUTC()
	entries := []RunLogEntry{
		{
			Timestamp:  now,
			IssueID:    "local-1",
			Identifier: "TASK-1",
			Event:      "notification",
			Details: map[string]any{"payload": map[string]any{
				"method": "item/agentMessage/delta",
				"params": map[string]any{"itemId": "msg-1", "delta": "Mer"},
			}},
		},
		{
			Timestamp:  now,
			IssueID:    "local-1",
			Identifier: "TASK-1",
			Event:      "notification",
			Details: map[string]any{"payload": map[string]any{
				"method": "item/agentMessage/delta",
				"params": map[string]any{"itemId": "msg-1", "delta": "haba"},
			}},
		},
	}

	display := DisplayRunEvents(entries)
	if len(display) != 1 {
		t.Fatalf("expected one folded message, got %#v", display)
	}
	if display[0].Kind != "assistant" || display[0].Title != "Agent" || display[0].Body != "Merhaba" {
		t.Fatalf("unexpected display event: %#v", display[0])
	}
}

func TestDisplayRunEventsExtractsCommandOutput(t *testing.T) {
	entry := RunLogEntry{
		Timestamp:  nowUTC(),
		IssueID:    "local-1",
		Identifier: "TASK-1",
		Event:      "notification",
		Details: map[string]any{"payload": map[string]any{
			"method": "item/completed",
			"params": map[string]any{"item": map[string]any{
				"command":          "git log -1",
				"aggregatedOutput": "abc123 test commit",
			}},
		}},
	}

	display := DisplayRunEvents([]RunLogEntry{entry})
	if len(display) != 1 {
		t.Fatalf("expected command display, got %#v", display)
	}
	if display[0].Kind != "tool" || display[0].Title != "Command completed" {
		t.Fatalf("unexpected command display: %#v", display[0])
	}
	if display[0].Body != "git log -1\n\nabc123 test commit" {
		t.Fatalf("unexpected command body: %q", display[0].Body)
	}
}

func TestDisplayRunEventsKeepsLifecycleEvents(t *testing.T) {
	entry := RunLogEntry{Timestamp: nowUTC(), IssueID: "local-1", Identifier: "TASK-1", Event: "task_completed", Summary: "Task completed", TurnCount: 1}
	display := DisplayRunEvents([]RunLogEntry{entry})
	if len(display) != 1 || display[0].Kind != "success" || display[0].Title != "Task completed" {
		t.Fatalf("unexpected lifecycle display: %#v", display)
	}
}

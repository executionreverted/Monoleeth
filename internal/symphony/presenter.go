package symphony

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func StatePayload(snapshot Snapshot) map[string]any {
	running := runningPayloads(snapshot.Running)
	retrying := retryPayloads(snapshot.Retries)
	blocked := blockedPayloads(snapshot.Blocked)
	return map[string]any{
		"generated_at": iso8601(nowUTC()),
		"counts": map[string]any{
			"running":  len(running),
			"retrying": len(retrying),
			"blocked":  len(blocked),
		},
		"running":  running,
		"retrying": retrying,
		"blocked":  blocked,
		"codex_totals": map[string]any{
			"input_tokens":    nil,
			"output_tokens":   nil,
			"total_tokens":    nil,
			"seconds_running": nil,
		},
		"rate_limits": nil,
	}
}

func IssuePayload(snapshot Snapshot, issueIdentifier string) (map[string]any, bool) {
	running := findRunningByIdentifier(snapshot.Running, issueIdentifier)
	retry := findRetryByIdentifier(snapshot.Retries, issueIdentifier)
	blocked := findBlockedByIdentifier(snapshot.Blocked, issueIdentifier)
	if running == nil && retry == nil && blocked == nil {
		return nil, false
	}
	issueID := ""
	if running != nil {
		issueID = running.IssueID
	} else if retry != nil {
		issueID = retry.IssueID
	} else {
		issueID = blocked.IssueID
	}
	workspacePath := fallbackWorkspacePath(snapshot.WorkspaceRoot, issueIdentifier)
	if running != nil && running.WorkspacePath != "" {
		workspacePath = running.WorkspacePath
	} else if blocked != nil && blocked.WorkspacePath != "" {
		workspacePath = blocked.WorkspacePath
	}
	return map[string]any{
		"issue_identifier": issueIdentifier,
		"issue_id":         issueID,
		"status":           issueStatus(running, retry, blocked),
		"workspace": map[string]any{
			"path": workspacePath,
			"host": nil,
		},
		"attempts": map[string]any{
			"restart_count":         restartCount(retry),
			"current_retry_attempt": retryAttempt(retry),
		},
		"running":       runningIssuePayload(running),
		"retry":         retryIssuePayload(retry),
		"blocked":       blockedIssuePayload(blocked),
		"logs":          map[string]any{"codex_session_logs": []any{}},
		"recent_events": recentEventsPayload(running, blocked),
		"last_error":    lastError(retry, blocked),
		"tracked":       map[string]any{},
	}, true
}

func runningPayloads(entries map[string]RunningEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range sortedRunning(entries) {
		out = append(out, runningEntryPayload(entry))
	}
	return out
}

func retryPayloads(entries map[string]RetryEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range sortedRetries(entries) {
		out = append(out, retryEntryPayload(entry))
	}
	return out
}

func blockedPayloads(entries map[string]BlockedEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range sortedBlocked(entries) {
		out = append(out, blockedEntryPayload(entry))
	}
	return out
}

func runningEntryPayload(entry RunningEntry) map[string]any {
	return map[string]any{
		"issue_id":         entry.IssueID,
		"issue_identifier": entry.Identifier,
		"issue_url":        stringOrNil(entry.Issue.URL),
		"state":            entry.Issue.State,
		"worker_host":      nil,
		"workspace_path":   stringOrNil(entry.WorkspacePath),
		"session_id":       stringOrNil(entry.SessionID),
		"turn_count":       entry.TurnCount,
		"last_event":       stringOrNil(entry.LastCodexEvent),
		"last_message":     summarizeCodexMessage(entry.LastCodexMessage),
		"started_at":       iso8601(entry.StartedAt),
		"last_event_at":    iso8601Ptr(entry.LastCodexTimestamp),
		"tokens":           unavailableTokens(),
	}
}

func retryEntryPayload(entry RetryEntry) map[string]any {
	return map[string]any{
		"issue_id":         entry.IssueID,
		"issue_identifier": entry.Identifier,
		"issue_url":        nil,
		"attempt":          entry.Attempt,
		"due_at":           iso8601(entry.DueAt),
		"error":            stringOrNil(entry.Error),
		"worker_host":      nil,
		"workspace_path":   nil,
	}
}

func blockedEntryPayload(entry BlockedEntry) map[string]any {
	return map[string]any{
		"issue_id":         entry.IssueID,
		"issue_identifier": entry.Identifier,
		"issue_url":        stringOrNil(entry.Issue.URL),
		"state":            entry.Issue.State,
		"error":            stringOrNil(entry.Error),
		"worker_host":      nil,
		"workspace_path":   stringOrNil(entry.WorkspacePath),
		"session_id":       stringOrNil(entry.SessionID),
		"blocked_at":       iso8601(entry.BlockedAt),
		"last_event":       stringOrNil(entry.LastEvent),
		"last_message":     summarizeCodexMessage(entry.LastMessage),
		"last_event_at":    iso8601Ptr(entry.LastTimestamp),
	}
}

func runningIssuePayload(entry *RunningEntry) any {
	if entry == nil {
		return nil
	}
	return map[string]any{
		"worker_host":    nil,
		"workspace_path": stringOrNil(entry.WorkspacePath),
		"session_id":     stringOrNil(entry.SessionID),
		"turn_count":     entry.TurnCount,
		"state":          entry.Issue.State,
		"started_at":     iso8601(entry.StartedAt),
		"last_event":     stringOrNil(entry.LastCodexEvent),
		"last_message":   summarizeCodexMessage(entry.LastCodexMessage),
		"last_event_at":  iso8601Ptr(entry.LastCodexTimestamp),
		"tokens":         unavailableTokens(),
	}
}

func retryIssuePayload(entry *RetryEntry) any {
	if entry == nil {
		return nil
	}
	return map[string]any{
		"attempt":        entry.Attempt,
		"due_at":         iso8601(entry.DueAt),
		"error":          stringOrNil(entry.Error),
		"worker_host":    nil,
		"workspace_path": nil,
	}
}

func blockedIssuePayload(entry *BlockedEntry) any {
	if entry == nil {
		return nil
	}
	return map[string]any{
		"worker_host":    nil,
		"workspace_path": stringOrNil(entry.WorkspacePath),
		"session_id":     stringOrNil(entry.SessionID),
		"state":          entry.Issue.State,
		"error":          stringOrNil(entry.Error),
		"blocked_at":     iso8601(entry.BlockedAt),
		"last_event":     stringOrNil(entry.LastEvent),
		"last_message":   summarizeCodexMessage(entry.LastMessage),
		"last_event_at":  iso8601Ptr(entry.LastTimestamp),
	}
}

func unavailableTokens() map[string]any {
	return map[string]any{"input_tokens": nil, "output_tokens": nil, "total_tokens": nil}
}

func recentEventsPayload(running *RunningEntry, blocked *BlockedEntry) []map[string]any {
	if running != nil && running.LastCodexTimestamp != nil {
		return []map[string]any{{
			"at":      iso8601Ptr(running.LastCodexTimestamp),
			"event":   stringOrNil(running.LastCodexEvent),
			"message": summarizeCodexMessage(running.LastCodexMessage),
		}}
	}
	if blocked != nil && blocked.LastTimestamp != nil {
		return []map[string]any{{
			"at":      iso8601Ptr(blocked.LastTimestamp),
			"event":   stringOrNil(blocked.LastEvent),
			"message": summarizeCodexMessage(blocked.LastMessage),
		}}
	}
	return []map[string]any{}
}

func summarizeCodexMessage(message map[string]any) any {
	if len(message) == 0 {
		return nil
	}
	for _, key := range []string{"message", "text", "summary"} {
		if value, _ := message[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if payload, ok := message["payload"].(map[string]any); ok {
		if params, ok := payload["params"].(map[string]any); ok {
			for _, key := range []string{"message", "text"} {
				if value, _ := params[key].(string); strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	encoded, err := json.Marshal(message)
	if err != nil || len(encoded) == 0 {
		return nil
	}
	text := string(encoded)
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
}

func findRunningByIdentifier(entries map[string]RunningEntry, identifier string) *RunningEntry {
	for _, entry := range entries {
		if entry.Identifier == identifier {
			copy := entry
			return &copy
		}
	}
	return nil
}

func findRetryByIdentifier(entries map[string]RetryEntry, identifier string) *RetryEntry {
	for _, entry := range entries {
		if entry.Identifier == identifier {
			copy := entry
			return &copy
		}
	}
	return nil
}

func findBlockedByIdentifier(entries map[string]BlockedEntry, identifier string) *BlockedEntry {
	for _, entry := range entries {
		if entry.Identifier == identifier {
			copy := entry
			return &copy
		}
	}
	return nil
}

func sortedRunning(entries map[string]RunningEntry) []RunningEntry {
	out := make([]RunningEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Identifier < out[j].Identifier })
	return out
}

func sortedRetries(entries map[string]RetryEntry) []RetryEntry {
	out := make([]RetryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Identifier < out[j].Identifier })
	return out
}

func sortedBlocked(entries map[string]BlockedEntry) []BlockedEntry {
	out := make([]BlockedEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Identifier < out[j].Identifier })
	return out
}

func issueStatus(running *RunningEntry, retry *RetryEntry, _ *BlockedEntry) string {
	if running != nil {
		return "running"
	}
	if retry != nil {
		return "retrying"
	}
	return "blocked"
}

func retryAttempt(retry *RetryEntry) int {
	if retry == nil {
		return 0
	}
	return retry.Attempt
}

func restartCount(retry *RetryEntry) int {
	attempt := retryAttempt(retry)
	if attempt <= 0 {
		return 0
	}
	return attempt - 1
}

func lastError(retry *RetryEntry, blocked *BlockedEntry) any {
	if blocked != nil && blocked.Error != "" {
		return blocked.Error
	}
	if retry != nil && retry.Error != "" {
		return retry.Error
	}
	return nil
}

func fallbackWorkspacePath(root, issueIdentifier string) string {
	if root == "" {
		return issueIdentifier
	}
	return filepath.Join(root, issueIdentifier)
}

func stringOrNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func iso8601Ptr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return iso8601(*value)
}

func iso8601(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Truncate(time.Second).Format(time.RFC3339)
}

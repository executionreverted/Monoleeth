package symphony

import (
	"strings"
	"time"
)

type DisplayRunEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	IssueID    string    `json:"issue_id"`
	Identifier string    `json:"identifier"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Body       string    `json:"body,omitempty"`
	Attempt    int       `json:"attempt,omitempty"`
	TurnCount  int       `json:"turn_count,omitempty"`
}

func DisplayRunEvents(entries []RunLogEntry) []DisplayRunEvent {
	out := make([]DisplayRunEvent, 0, len(entries))
	assistantByItem := map[string]int{}
	for _, entry := range entries {
		payload, _ := entry.Details["payload"].(map[string]any)
		method, _ := payload["method"].(string)
		params, _ := payload["params"].(map[string]any)
		switch method {
		case "item/agentMessage/delta":
			itemID, _ := params["itemId"].(string)
			delta, _ := params["delta"].(string)
			if strings.TrimSpace(itemID) == "" || delta == "" {
				continue
			}
			if index, ok := assistantByItem[itemID]; ok {
				out[index].Body += delta
				out[index].Timestamp = entry.Timestamp
				continue
			}
			assistantByItem[itemID] = len(out)
			out = append(out, displayFromEntry(entry, "assistant", "Agent", delta))
		case "item/started":
			if display, ok := commandStartedDisplay(entry, params); ok {
				out = append(out, display)
			}
		case "item/completed":
			if display, ok := commandCompletedDisplay(entry, params); ok {
				out = append(out, display)
			}
		case "turn/completed":
			out = append(out, displayFromEntry(entry, "status", "Turn completed", ""))
		default:
			if display, ok := lifecycleDisplay(entry); ok {
				out = append(out, display)
			}
		}
	}
	return out
}

func displayFromEntry(entry RunLogEntry, kind, title, body string) DisplayRunEvent {
	return DisplayRunEvent{
		Timestamp:  entry.Timestamp,
		IssueID:    entry.IssueID,
		Identifier: entry.Identifier,
		Kind:       kind,
		Title:      title,
		Body:       trimDisplayBody(body),
		Attempt:    entry.Attempt,
		TurnCount:  entry.TurnCount,
	}
}

func commandStartedDisplay(entry RunLogEntry, params map[string]any) (DisplayRunEvent, bool) {
	item, _ := params["item"].(map[string]any)
	command, _ := item["command"].(string)
	if strings.TrimSpace(command) == "" {
		return DisplayRunEvent{}, false
	}
	return displayFromEntry(entry, "tool", "Command started", command), true
}

func commandCompletedDisplay(entry RunLogEntry, params map[string]any) (DisplayRunEvent, bool) {
	item, _ := params["item"].(map[string]any)
	command, _ := item["command"].(string)
	output, _ := item["aggregatedOutput"].(string)
	if strings.TrimSpace(command) == "" && strings.TrimSpace(output) == "" {
		return DisplayRunEvent{}, false
	}
	body := strings.TrimSpace(output)
	if body == "" {
		body = strings.TrimSpace(command)
	} else if command != "" {
		body = strings.TrimSpace(command) + "\n\n" + body
	}
	return displayFromEntry(entry, "tool", "Command completed", body), true
}

func lifecycleDisplay(entry RunLogEntry) (DisplayRunEvent, bool) {
	switch entry.Event {
	case "dispatch_started":
		return displayFromEntry(entry, "status", "Dispatched", nonEmpty(entry.Summary, "Dispatched to agent")), true
	case "task_completed":
		return displayFromEntry(entry, "success", "Task completed", nonEmpty(entry.Summary, "Task completed")), true
	case "turn_error", "task_failed_retry_scheduled":
		return displayFromEntry(entry, "error", "Run failed", nonEmpty(entry.Summary, entry.Event)), true
	case "retry_scheduled":
		return displayFromEntry(entry, "warning", "Retry scheduled", nonEmpty(entry.Summary, "Retry scheduled")), true
	case "task_blocked":
		return displayFromEntry(entry, "warning", "Blocked", nonEmpty(entry.Summary, "Task blocked")), true
	case "max_turns_reached":
		return displayFromEntry(entry, "warning", "Max turns reached", nonEmpty(entry.Summary, "Max turns reached")), true
	}
	if entry.Event != "notification" && strings.TrimSpace(entry.Summary) != "" {
		return displayFromEntry(entry, "status", humanizeEvent(entry.Event), entry.Summary), true
	}
	return DisplayRunEvent{}, false
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func trimDisplayBody(body string) string {
	body = strings.TrimSpace(body)
	const maxDisplayBodyRunes = 2400
	runes := []rune(body)
	if len(runes) <= maxDisplayBodyRunes {
		return body
	}
	return string(runes[:maxDisplayBodyRunes]) + "\n...<truncated; raw stream keeps the full event>"
}

func humanizeEvent(event string) string {
	event = strings.TrimSpace(strings.ReplaceAll(event, "_", " "))
	if event == "" {
		return "Event"
	}
	return strings.ToUpper(event[:1]) + event[1:]
}

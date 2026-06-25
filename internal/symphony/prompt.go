package symphony

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	ifTagPattern  = regexp.MustCompile(`(?s){%\s*if\s+([A-Za-z0-9_.$-]+)\s*%}(.*?)(?:{%\s*else\s*%}(.*?))?{%\s*endif\s*%}`)
	varTagPattern = regexp.MustCompile(`{{\s*([A-Za-z0-9_.$-]+)\s*}}`)
)

func BuildPrompt(template string, issue Issue, attempt int) (string, error) {
	ctx := map[string]any{
		"attempt": attemptValue(attempt),
		"issue":   issueToMap(issue),
	}
	rendered := ifTagPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := ifTagPattern.FindStringSubmatch(match)
		if len(parts) == 0 {
			return match
		}
		value, ok := lookupPath(ctx, parts[1])
		if truthy(value) && ok {
			return parts[2]
		}
		if len(parts) > 3 {
			return parts[3]
		}
		return ""
	})
	var missing []string
	rendered = varTagPattern.ReplaceAllStringFunc(rendered, func(match string) string {
		parts := varTagPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		value, ok := lookupPath(ctx, parts[1])
		if !ok {
			missing = append(missing, parts[1])
			return ""
		}
		return stringifyTemplateValue(value)
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("template missing variables: %s", strings.Join(missing, ", "))
	}
	return rendered, nil
}

func attemptValue(attempt int) any {
	if attempt <= 0 {
		return nil
	}
	return attempt
}

func issueToMap(issue Issue) map[string]any {
	blockedBy := make([]map[string]any, 0, len(issue.BlockedBy))
	for _, blocker := range issue.BlockedBy {
		blockedBy = append(blockedBy, map[string]any{
			"id":         blocker.ID,
			"identifier": blocker.Identifier,
			"state":      blocker.State,
		})
	}
	return map[string]any{
		"id":                 issue.ID,
		"identifier":         issue.Identifier,
		"title":              issue.Title,
		"description":        issue.Description,
		"priority":           issue.Priority,
		"state":              issue.State,
		"branch_name":        issue.BranchName,
		"url":                issue.URL,
		"labels":             issue.Labels,
		"blocked_by":         blockedBy,
		"assignee_id":        issue.AssigneeID,
		"assigned_to_worker": issue.AssignedToWorker,
		"agent_backend":      issue.AgentBackend,
		"agent_model":        issue.AgentModel,
		"agent_endpoint":     issue.AgentEndpoint,
		"created_at":         timePtrString(issue.CreatedAt),
		"updated_at":         timePtrString(issue.UpdatedAt),
	}
}

func lookupPath(ctx map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}
	var cur any = ctx
	for _, part := range parts {
		switch typed := cur.(type) {
		case map[string]any:
			value, ok := typed[part]
			if !ok {
				return nil, false
			}
			cur = value
		default:
			return nil, false
		}
	}
	return cur, true
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.TrimSpace(v) != ""
	case int:
		return v != 0
	case *int:
		return v != nil
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Map:
			return rv.Len() > 0
		}
		return true
	}
}

func stringifyTemplateValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, ", ")
	case *int:
		if v == nil {
			return ""
		}
		return fmt.Sprint(*v)
	default:
		return fmt.Sprint(v)
	}
}

func timePtrString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

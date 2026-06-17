package symphony

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const linearIssuePageSize = 50

const linearPollQuery = `
query SymphonyLinearPoll($projectSlug: String!, $stateNames: [String!]!, $first: Int!, $relationFirst: Int!, $after: String) {
  issues(filter: {project: {slugId: {eq: $projectSlug}}, state: {name: {in: $stateNames}}}, first: $first, after: $after) {
    nodes {
      id
      identifier
      title
      description
      priority
      state { name }
      branchName
      url
      assignee { id }
      labels { nodes { name } }
      inverseRelations(first: $relationFirst) {
        nodes {
          type
          issue { id identifier state { name } }
        }
      }
      createdAt
      updatedAt
    }
    pageInfo { hasNextPage endCursor }
  }
}`

const linearIssuesByIDQuery = `
query SymphonyLinearIssuesById($ids: [ID!]!, $first: Int!, $relationFirst: Int!) {
  issues(filter: {id: {in: $ids}}, first: $first) {
    nodes {
      id
      identifier
      title
      description
      priority
      state { name }
      branchName
      url
      assignee { id }
      labels { nodes { name } }
      inverseRelations(first: $relationFirst) {
        nodes {
          type
          issue { id identifier state { name } }
        }
      }
      createdAt
      updatedAt
    }
  }
}`

const linearViewerQuery = `
query SymphonyLinearViewer {
  viewer { id }
}`

type Tracker interface {
	FetchCandidateIssues(context.Context) ([]Issue, error)
	FetchIssueStatesByIDs(context.Context, []string) ([]Issue, error)
	FetchIssuesByStates(context.Context, []string) ([]Issue, error)
	GraphQL(context.Context, string, map[string]any, string) (map[string]any, error)
}

type IssueStateUpdater interface {
	UpdateIssueState(context.Context, string, string) error
}

type LocalTaskCreator interface {
	CreateTask(context.Context, CreateTaskInput) (Issue, error)
	ListTasks(context.Context) ([]Issue, error)
}

type LinearClient struct {
	config Config
	client *http.Client
	logger *Logger
}

func NewLinearClient(config Config, logger *Logger) *LinearClient {
	return &LinearClient{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

func (c *LinearClient) FetchCandidateIssues(ctx context.Context) ([]Issue, error) {
	if c.config.Tracker.APIKey == "" {
		return nil, errors.New("missing Linear API token")
	}
	if c.config.Tracker.ProjectSlug == "" {
		return nil, errors.New("missing Linear project slug")
	}
	assignee, err := c.assigneeFilter(ctx)
	if err != nil {
		return nil, err
	}
	return c.fetchByStates(ctx, c.config.Tracker.ActiveStates, assignee)
}

func (c *LinearClient) FetchIssuesByStates(ctx context.Context, states []string) ([]Issue, error) {
	if len(states) == 0 {
		return nil, nil
	}
	return c.fetchByStates(ctx, uniqueStrings(states), nil)
}

func (c *LinearClient) FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]Issue, error) {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	assignee, err := c.assigneeFilter(ctx)
	if err != nil {
		return nil, err
	}
	var out []Issue
	order := map[string]int{}
	for i, id := range ids {
		order[id] = i
	}
	for start := 0; start < len(ids); start += linearIssuePageSize {
		end := start + linearIssuePageSize
		if end > len(ids) {
			end = len(ids)
		}
		body, err := c.GraphQL(ctx, linearIssuesByIDQuery, map[string]any{
			"ids":           ids[start:end],
			"first":         end - start,
			"relationFirst": linearIssuePageSize,
		}, "")
		if err != nil {
			return nil, err
		}
		nodes, err := issueNodes(body)
		if err != nil {
			return nil, err
		}
		for _, node := range nodes {
			if issue := normalizeLinearIssue(node, assignee); issue != nil {
				out = append(out, *issue)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return order[out[i].ID] < order[out[j].ID]
	})
	return out, nil
}

func (c *LinearClient) fetchByStates(ctx context.Context, states []string, assignee *assigneeFilter) ([]Issue, error) {
	var out []Issue
	var after any
	for {
		body, err := c.GraphQL(ctx, linearPollQuery, map[string]any{
			"projectSlug":   c.config.Tracker.ProjectSlug,
			"stateNames":    states,
			"first":         linearIssuePageSize,
			"relationFirst": linearIssuePageSize,
			"after":         after,
		}, "")
		if err != nil {
			return nil, err
		}
		nodes, pageInfo, err := issuePage(body)
		if err != nil {
			return nil, err
		}
		for _, node := range nodes {
			if issue := normalizeLinearIssue(node, assignee); issue != nil {
				out = append(out, *issue)
			}
		}
		hasNext, _ := pageInfo["hasNextPage"].(bool)
		cursor, _ := pageInfo["endCursor"].(string)
		if !hasNext {
			break
		}
		if cursor == "" {
			return nil, errors.New("linear missing endCursor")
		}
		after = cursor
	}
	return out, nil
}

func (c *LinearClient) GraphQL(ctx context.Context, query string, variables map[string]any, operationName string) (map[string]any, error) {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	if strings.TrimSpace(operationName) != "" {
		payload["operationName"] = strings.TrimSpace(operationName)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Tracker.Endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.config.Tracker.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linear API status %d: %s", resp.StatusCode, truncateForLog(string(raw), 1000))
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	if errorsPayload, ok := body["errors"]; ok {
		return nil, fmt.Errorf("linear graphql errors: %v", errorsPayload)
	}
	return body, nil
}

type assigneeFilter struct {
	Configured string
	Values     map[string]bool
}

func (c *LinearClient) assigneeFilter(ctx context.Context) (*assigneeFilter, error) {
	assignee := strings.TrimSpace(c.config.Tracker.Assignee)
	if assignee == "" {
		return nil, nil
	}
	if strings.EqualFold(assignee, "me") {
		body, err := c.GraphQL(ctx, linearViewerQuery, map[string]any{}, "")
		if err != nil {
			return nil, err
		}
		data, _ := getMap(body, "data")
		viewer, _ := getMap(data, "viewer")
		id, _ := viewer["id"].(string)
		if id == "" {
			return nil, errors.New("missing Linear viewer identity")
		}
		return &assigneeFilter{Configured: "me", Values: map[string]bool{id: true}}, nil
	}
	return &assigneeFilter{Configured: assignee, Values: map[string]bool{assignee: true}}, nil
}

func issuePage(body map[string]any) ([]map[string]any, map[string]any, error) {
	data, _ := getMap(body, "data")
	issues, ok := getMap(data, "issues")
	if !ok {
		return nil, nil, errors.New("linear unknown payload")
	}
	nodes, err := asMapSlice(issues["nodes"])
	if err != nil {
		return nil, nil, err
	}
	pageInfo, _ := getMap(issues, "pageInfo")
	return nodes, pageInfo, nil
}

func issueNodes(body map[string]any) ([]map[string]any, error) {
	data, _ := getMap(body, "data")
	issues, ok := getMap(data, "issues")
	if !ok {
		return nil, errors.New("linear unknown payload")
	}
	return asMapSlice(issues["nodes"])
}

func normalizeLinearIssue(raw map[string]any, assignee *assigneeFilter) *Issue {
	id, _ := raw["id"].(string)
	identifier, _ := raw["identifier"].(string)
	title, _ := raw["title"].(string)
	stateMap, _ := getMap(raw, "state")
	state, _ := stateMap["name"].(string)
	assigneeMap, _ := getMap(raw, "assignee")
	assigneeID, _ := assigneeMap["id"].(string)
	priority := parseOptionalInt(raw["priority"])
	issue := &Issue{
		ID:               id,
		Identifier:       identifier,
		Title:            title,
		State:            state,
		Priority:         priority,
		Labels:           extractLinearLabels(raw),
		BlockedBy:        extractLinearBlockers(raw),
		AssigneeID:       assigneeID,
		AssignedToWorker: true,
		CreatedAt:        parseLinearTime(raw["createdAt"]),
		UpdatedAt:        parseLinearTime(raw["updatedAt"]),
	}
	if description, _ := raw["description"].(string); description != "" {
		issue.Description = description
	}
	if branchName, _ := raw["branchName"].(string); branchName != "" {
		issue.BranchName = branchName
	}
	if url, _ := raw["url"].(string); url != "" {
		issue.URL = url
	}
	if assignee != nil {
		issue.AssignedToWorker = assigneeID != "" && assignee.Values[assigneeID]
	}
	return issue
}

func extractLinearLabels(raw map[string]any) []string {
	labelsMap, ok := getMap(raw, "labels")
	if !ok {
		return nil
	}
	nodes, err := asMapSlice(labelsMap["nodes"])
	if err != nil {
		return nil
	}
	var labels []string
	for _, node := range nodes {
		if name, _ := node["name"].(string); strings.TrimSpace(name) != "" {
			labels = append(labels, strings.ToLower(strings.TrimSpace(name)))
		}
	}
	return labels
}

func extractLinearBlockers(raw map[string]any) []BlockerRef {
	relationsMap, ok := getMap(raw, "inverseRelations")
	if !ok {
		return nil
	}
	nodes, err := asMapSlice(relationsMap["nodes"])
	if err != nil {
		return nil
	}
	var blockers []BlockerRef
	for _, node := range nodes {
		relationType, _ := node["type"].(string)
		if normalizeState(relationType) != "blocks" {
			continue
		}
		issueMap, ok := getMap(node, "issue")
		if !ok {
			continue
		}
		stateMap, _ := getMap(issueMap, "state")
		blocker := BlockerRef{}
		blocker.ID, _ = issueMap["id"].(string)
		blocker.Identifier, _ = issueMap["identifier"].(string)
		blocker.State, _ = stateMap["name"].(string)
		blockers = append(blockers, blocker)
	}
	return blockers
}

func getMap(parent map[string]any, key string) (map[string]any, bool) {
	value, ok := parent[key]
	if !ok || value == nil {
		return nil, false
	}
	typed, ok := value.(map[string]any)
	return typed, ok
}

func asMapSlice(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("expected list")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func parseOptionalInt(value any) *int {
	switch v := value.(type) {
	case float64:
		out := int(v)
		return &out
	case int:
		out := v
		return &out
	default:
		return nil
	}
}

func parseLinearTime(value any) *time.Time {
	raw, ok := value.(string)
	if !ok || raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	return &t
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

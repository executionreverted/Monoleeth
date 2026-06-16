package symphony

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Tracker       TrackerConfig       `yaml:"tracker" json:"tracker"`
	Polling       PollingConfig       `yaml:"polling" json:"polling"`
	Workspace     WorkspaceConfig     `yaml:"workspace" json:"workspace"`
	Hooks         HooksConfig         `yaml:"hooks" json:"hooks"`
	Agent         AgentConfig         `yaml:"agent" json:"agent"`
	Codex         CodexConfig         `yaml:"codex" json:"codex"`
	Worker        WorkerConfig        `yaml:"worker" json:"worker"`
	Server        ServerConfig        `yaml:"server" json:"server"`
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`
}

type TrackerConfig struct {
	Kind           string   `yaml:"kind" json:"kind"`
	Endpoint       string   `yaml:"endpoint" json:"endpoint"`
	APIKey         string   `yaml:"api_key" json:"-"`
	ProjectSlug    string   `yaml:"project_slug" json:"project_slug"`
	Assignee       string   `yaml:"assignee" json:"assignee,omitempty"`
	RequiredLabels []string `yaml:"required_labels" json:"required_labels"`
	ActiveStates   []string `yaml:"active_states" json:"active_states"`
	TerminalStates []string `yaml:"terminal_states" json:"terminal_states"`
}

type PollingConfig struct {
	IntervalMS int `yaml:"interval_ms" json:"interval_ms"`
}

type WorkspaceConfig struct {
	Root string `yaml:"root" json:"root"`
}

type HooksConfig struct {
	AfterCreate  string `yaml:"after_create" json:"after_create,omitempty"`
	BeforeRun    string `yaml:"before_run" json:"before_run,omitempty"`
	AfterRun     string `yaml:"after_run" json:"after_run,omitempty"`
	BeforeRemove string `yaml:"before_remove" json:"before_remove,omitempty"`
	TimeoutMS    int    `yaml:"timeout_ms" json:"timeout_ms"`
}

type AgentConfig struct {
	MaxConcurrentAgents        int            `yaml:"max_concurrent_agents" json:"max_concurrent_agents"`
	MaxTurns                   int            `yaml:"max_turns" json:"max_turns"`
	MaxRetryBackoffMS          int            `yaml:"max_retry_backoff_ms" json:"max_retry_backoff_ms"`
	MaxConcurrentAgentsByState map[string]int `yaml:"max_concurrent_agents_by_state" json:"max_concurrent_agents_by_state"`
}

type CodexConfig struct {
	Command           string         `yaml:"command" json:"command"`
	ApprovalPolicy    any            `yaml:"approval_policy" json:"approval_policy"`
	ThreadSandbox     string         `yaml:"thread_sandbox" json:"thread_sandbox"`
	TurnSandboxPolicy map[string]any `yaml:"turn_sandbox_policy" json:"turn_sandbox_policy,omitempty"`
	TurnTimeoutMS     int            `yaml:"turn_timeout_ms" json:"turn_timeout_ms"`
	ReadTimeoutMS     int            `yaml:"read_timeout_ms" json:"read_timeout_ms"`
	StallTimeoutMS    int            `yaml:"stall_timeout_ms" json:"stall_timeout_ms"`
}

type WorkerConfig struct {
	SSHHosts                   []string `yaml:"ssh_hosts" json:"ssh_hosts"`
	MaxConcurrentAgentsPerHost int      `yaml:"max_concurrent_agents_per_host" json:"max_concurrent_agents_per_host,omitempty"`
}

type ServerConfig struct {
	Port int    `yaml:"port" json:"port,omitempty"`
	Host string `yaml:"host" json:"host"`
}

type ObservabilityConfig struct {
	DashboardEnabled bool `yaml:"dashboard_enabled" json:"dashboard_enabled"`
	RefreshMS        int  `yaml:"refresh_ms" json:"refresh_ms"`
	RenderIntervalMS int  `yaml:"render_interval_ms" json:"render_interval_ms"`
}

func defaultConfig() Config {
	return Config{
		Tracker: TrackerConfig{
			Kind:           "linear",
			Endpoint:       "https://api.linear.app/graphql",
			ActiveStates:   []string{"Todo", "In Progress"},
			TerminalStates: []string{"Closed", "Cancelled", "Canceled", "Duplicate", "Done"},
			RequiredLabels: []string{},
		},
		Polling:   PollingConfig{IntervalMS: 30000},
		Workspace: WorkspaceConfig{Root: filepath.Join(os.TempDir(), "symphony_workspaces")},
		Hooks:     HooksConfig{TimeoutMS: 60000},
		Agent: AgentConfig{
			MaxConcurrentAgents:        10,
			MaxTurns:                   20,
			MaxRetryBackoffMS:          300000,
			MaxConcurrentAgentsByState: map[string]int{},
		},
		Codex: CodexConfig{
			Command: "codex app-server",
			ApprovalPolicy: map[string]any{"reject": map[string]any{
				"sandbox_approval": true,
				"rules":            true,
				"mcp_elicitations": true,
			}},
			ThreadSandbox:  "workspace-write",
			TurnTimeoutMS:  3600000,
			ReadTimeoutMS:  5000,
			StallTimeoutMS: 300000,
		},
		Server:        ServerConfig{Host: "127.0.0.1"},
		Observability: ObservabilityConfig{DashboardEnabled: true, RefreshMS: 1000, RenderIntervalMS: 16},
	}
}

func finalizeConfig(cfg Config, workflowDir string) (Config, error) {
	cfg.Tracker.APIKey = resolveSecretSetting(cfg.Tracker.APIKey, os.Getenv("LINEAR_API_KEY"))
	cfg.Tracker.Assignee = resolveSecretSetting(cfg.Tracker.Assignee, os.Getenv("LINEAR_ASSIGNEE"))
	cfg.Tracker.RequiredLabels = normalizeLabels(cfg.Tracker.RequiredLabels)
	cfg.Agent.MaxConcurrentAgentsByState = normalizeStateLimits(cfg.Agent.MaxConcurrentAgentsByState)
	cfg.Workspace.Root = resolvePathValue(cfg.Workspace.Root, workflowDir, filepath.Join(os.TempDir(), "symphony_workspaces"))

	if cfg.Tracker.Kind == "" {
		cfg.Tracker.Kind = "linear"
	}
	if cfg.Tracker.Endpoint == "" {
		cfg.Tracker.Endpoint = "https://api.linear.app/graphql"
	}
	if cfg.Polling.IntervalMS <= 0 {
		return cfg, errors.New("polling.interval_ms must be greater than 0")
	}
	if cfg.Agent.MaxConcurrentAgents <= 0 {
		return cfg, errors.New("agent.max_concurrent_agents must be greater than 0")
	}
	if cfg.Agent.MaxTurns <= 0 {
		return cfg, errors.New("agent.max_turns must be greater than 0")
	}
	if cfg.Agent.MaxRetryBackoffMS <= 0 {
		return cfg, errors.New("agent.max_retry_backoff_ms must be greater than 0")
	}
	if cfg.Codex.Command == "" {
		cfg.Codex.Command = "codex app-server"
	}
	if cfg.Codex.ThreadSandbox == "" {
		cfg.Codex.ThreadSandbox = "workspace-write"
	}
	if cfg.Codex.TurnTimeoutMS <= 0 || cfg.Codex.ReadTimeoutMS <= 0 || cfg.Codex.StallTimeoutMS < 0 {
		return cfg, errors.New("codex timeout values are invalid")
	}
	if cfg.Hooks.TimeoutMS <= 0 {
		return cfg, errors.New("hooks.timeout_ms must be greater than 0")
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	return cfg, nil
}

func (c Config) ValidateDispatch() error {
	switch {
	case c.Tracker.Kind == "":
		return errors.New("missing tracker kind")
	case c.Tracker.Kind != "linear" && c.Tracker.Kind != "memory":
		return fmt.Errorf("unsupported tracker kind: %s", c.Tracker.Kind)
	case c.Tracker.Kind == "linear" && c.Tracker.APIKey == "":
		return errors.New("missing Linear API token")
	case c.Tracker.Kind == "linear" && c.Tracker.ProjectSlug == "":
		return errors.New("missing Linear project slug")
	default:
		return nil
	}
}

func (c Config) MaxConcurrentAgentsForState(state string) int {
	key := normalizeState(state)
	if v, ok := c.Agent.MaxConcurrentAgentsByState[key]; ok && v > 0 {
		return v
	}
	return c.Agent.MaxConcurrentAgents
}

func (c Config) TurnSandboxPolicy(workspace string) map[string]any {
	if c.Codex.TurnSandboxPolicy != nil {
		return c.Codex.TurnSandboxPolicy
	}
	root := workspace
	if root == "" {
		root = c.Workspace.Root
	}
	return map[string]any{
		"type":                "workspaceWrite",
		"writableRoots":       []string{root},
		"readOnlyAccess":      map[string]any{"type": "fullAccess"},
		"networkAccess":       false,
		"excludeTmpdirEnvVar": false,
		"excludeSlashTmp":     false,
	}
}

func resolveSecretSetting(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	if name, ok := envReferenceName(value); ok {
		return strings.TrimSpace(os.Getenv(name))
	}
	return value
}

func resolvePathValue(value, workflowDir, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if name, ok := envReferenceName(value); ok {
		value = strings.TrimSpace(os.Getenv(name))
		if value == "" {
			value = fallback
		}
	}
	if value == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			value = home
		}
	} else if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(workflowDir, value)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return value
	}
	return abs
}

var envRefPattern = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)$`)

func envReferenceName(value string) (string, bool) {
	matches := envRefPattern.FindStringSubmatch(value)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func normalizeLabels(labels []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}

func normalizeState(state string) string {
	return strings.ToLower(strings.TrimSpace(state))
}

func normalizeStateLimits(in map[string]int) map[string]int {
	out := map[string]int{}
	for state, limit := range in {
		state = normalizeState(state)
		if state != "" && limit > 0 {
			out[state] = limit
		}
	}
	return out
}

func durationFromMS(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

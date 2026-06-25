package symphony

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type HTTPServer struct {
	server       *http.Server
	orchestrator *Orchestrator
	logger       *Logger
}

const (
	defaultTaskWaitTimeout = 5 * time.Minute
	maxTaskWaitTimeout     = 30 * time.Minute
)

func NewHTTPServer(host string, port int, orchestrator *Orchestrator, logger *Logger) *HTTPServer {
	mux := http.NewServeMux()
	s := &HTTPServer{orchestrator: orchestrator, logger: logger}
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/tasks", s.handleTasksPage)
	mux.HandleFunc("/dashboard.css", s.handleDashboardCSS)
	mux.HandleFunc("/api/v1/state", s.handleState)
	mux.HandleFunc("/api/v1/raw-state", s.handleRawState)
	mux.HandleFunc("/api/v1/refresh", s.handleRefresh)
	mux.HandleFunc("/api/v1/autorun", s.handleAutoRun)
	mux.HandleFunc("/api/v1/tasks", s.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", s.handleTaskAction)
	mux.HandleFunc("/api/v1/", s.handleIssue)
	s.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *HTTPServer) Start() error {
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	go func() {
		s.logger.Info("starting Symphony HTTP API", "addr", s.server.Addr)
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP API stopped with error", "error", err)
		}
	}()
	return nil
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

func (s *HTTPServer) handleTasksPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(tasksHTML))
}

func (s *HTTPServer) handleDashboardCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(upstreamDashboardCSS))
}

func (s *HTTPServer) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.orchestrator.StatePayload())
}

func (s *HTTPServer) handleRawState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.orchestrator.Snapshot())
}

func (s *HTTPServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.orchestrator.ForcePoll()
	writeJSON(w, map[string]any{"ok": true, "requested_at": iso8601(nowUTC())})
}

func (s *HTTPServer) handleAutoRun(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"auto_run": !s.orchestrator.AutoRunPaused()})
	case http.MethodPatch:
		var input struct {
			AutoRun *bool `json:"auto_run"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		if input.AutoRun == nil {
			writeError(w, fmt.Errorf("auto_run is required"), http.StatusBadRequest)
			return
		}
		s.orchestrator.SetAutoRunPaused(!*input.AutoRun)
		writeJSON(w, map[string]any{"auto_run": *input.AutoRun})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks, err := s.orchestrator.ListTasks(r.Context())
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		if tasks == nil {
			tasks = []Issue{}
		}
		writeJSON(w, map[string]any{"tasks": tasks})
	case http.MethodPost:
		var input CreateTaskInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		task, err := s.orchestrator.CreateTask(r.Context(), input)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, task)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	issueID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case r.Method == http.MethodGet && action == "stream":
		events := s.orchestrator.TaskRunLog(issueID)
		writeJSON(w, map[string]any{"events": events, "display_events": DisplayRunEvents(events)})
	case r.Method == http.MethodGet && action == "workspace-diff":
		packet, err := s.orchestrator.WorkspaceDiff(r.Context(), issueID)
		if errors.Is(err, ErrTaskNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, ErrWorkspaceNotFound) {
			writeError(w, err, http.StatusConflict)
			return
		}
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, packet)
	case r.Method == http.MethodGet && action == "wait":
		timeout, err := parseTaskWaitTimeout(r)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := s.orchestrator.WaitForTask(ctx, issueID)
		if errors.Is(err, ErrTaskNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	case r.Method == http.MethodPost && action == "run":
		if err := s.orchestrator.RunTask(r.Context(), issueID); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case r.Method == http.MethodPatch && action == "state":
		var input struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		if err := s.orchestrator.UpdateTaskState(r.Context(), issueID, input.State); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func parseTaskWaitTimeout(r *http.Request) (time.Duration, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("timeout_ms"))
	if raw == "" {
		return defaultTaskWaitTimeout, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("timeout_ms must be an integer")
	}
	if value < 0 {
		return 0, fmt.Errorf("timeout_ms must be non-negative")
	}
	timeout := time.Duration(value) * time.Millisecond
	if timeout == 0 {
		return 0, nil
	}
	if timeout > maxTaskWaitTimeout {
		return maxTaskWaitTimeout, nil
	}
	return timeout, nil
}

func (s *HTTPServer) handleIssue(w http.ResponseWriter, r *http.Request) {
	identifier := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if identifier == "" {
		http.NotFound(w, r)
		return
	}
	payload, ok := s.orchestrator.IssuePayload(identifier)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, payload)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

//go:embed dashboard.css
var upstreamDashboardCSS string

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Symphony Go</title>
  <link rel="stylesheet" href="/dashboard.css" />
</head>
<body>
  <main class="app-shell">
    <nav class="top-nav" aria-label="Primary navigation">
      <a class="top-nav-brand" href="/">
        <span class="top-nav-mark">S</span>
        <span>Symphony</span>
      </a>
      <div class="top-nav-links">
        <a class="top-nav-link top-nav-link-active" href="/">Observability</a>
        <a class="top-nav-link" href="/tasks">Tasks</a>
      </div>
    </nav>
    <section class="dashboard-shell phx-connected" data-phx-main>
      <header class="hero-card">
        <div class="hero-grid">
          <div>
            <p class="eyebrow">Symphony Control</p>
            <h1 class="hero-title">Runtime Control Center</h1>
            <p class="hero-copy">Live orchestration health, retry pressure, token burn, and active worker telemetry for the current Symphony runtime.</p>
          </div>

          <div class="status-stack">
            <span class="status-badge status-badge-live">
              <span class="status-badge-dot"></span>
              Live
            </span>
            <span class="status-badge status-badge-offline">
              <span class="status-badge-dot"></span>
              Offline
            </span>
          </div>
        </div>
      </header>

      <section class="error-card" id="errorCard" hidden>
        <h2 class="error-title">Snapshot unavailable</h2>
        <p class="error-copy" id="errorCopy"></p>
      </section>

      <section id="dashboardContent">
        <section class="metric-grid" id="metricGrid"></section>

        <section class="section-card">
          <div class="section-header">
            <div>
              <h2 class="section-title">Rate limits</h2>
              <p class="section-copy">Latest upstream rate-limit snapshot, when available.</p>
            </div>
          </div>
          <pre class="code-panel" id="rateLimits">Loading...</pre>
        </section>

        <section class="section-card">
          <div class="section-header">
            <div>
              <h2 class="section-title">Running sessions</h2>
              <p class="section-copy">Active issues, last known agent activity, and token usage.</p>
            </div>
          </div>
          <div id="runningSessions"></div>
        </section>

        <section class="section-card">
          <div class="section-header">
            <div>
              <h2 class="section-title">Blocked sessions</h2>
              <p class="section-copy">Issues paused because Codex requested operator input or approval.</p>
            </div>
          </div>
          <div id="blockedSessions"></div>
        </section>

        <section class="section-card">
          <div class="section-header">
            <div>
              <h2 class="section-title">Retry queue</h2>
              <p class="section-copy">Issues waiting for the next retry window.</p>
            </div>
          </div>
          <div id="retryQueue"></div>
        </section>
      </section>
    </section>
  </main>
  <script>
    let latestPayload = null;

    async function api(path, options = {}) {
      const response = await fetch(path, {
        headers: { "Content-Type": "application/json", ...(options.headers || {}) },
        ...options,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || response.statusText);
      }
      return response.json();
    }

    function escapeHTML(value) {
      return String(value ?? "").replace(/[&<>"']/g, char => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" }[char]));
    }

    function formatInt(value) {
      return Number.isInteger(value) ? value.toLocaleString("en-US") : "n/a";
    }

    function prettyValue(value) {
      return value == null ? "n/a" : JSON.stringify(value, null, 2);
    }

    function stateBadgeClass(state) {
      const base = "state-badge";
      const normalized = String(state || "").toLowerCase();
      if (["progress", "running", "active"].some(part => normalized.includes(part))) return base + " state-badge-active";
      if (["blocked", "error", "failed"].some(part => normalized.includes(part))) return base + " state-badge-danger";
      if (["todo", "queued", "pending", "retry"].some(part => normalized.includes(part))) return base + " state-badge-warning";
      return base;
    }

    function runtimeSecondsFrom(startedAt) {
      if (!startedAt) return 0;
      const parsed = Date.parse(startedAt);
      if (Number.isNaN(parsed)) return 0;
      return Math.max(Math.floor((Date.now() - parsed) / 1000), 0);
    }

    function formatRuntimeSeconds(seconds) {
      const whole = Math.max(Math.trunc(seconds || 0), 0);
      return Math.floor(whole / 60) + "m " + (whole % 60) + "s";
    }

    function totalRuntimeSeconds(payload) {
      const completed = Number(payload?.codex_totals?.seconds_running) || 0;
      return completed + (payload?.running || []).reduce((total, entry) => total + runtimeSecondsFrom(entry.started_at), 0);
    }

    function formatRuntimeAndTurns(startedAt, turnCount) {
      const runtime = formatRuntimeSeconds(runtimeSecondsFrom(startedAt));
      return Number.isInteger(turnCount) && turnCount > 0 ? runtime + " / " + turnCount : runtime;
    }

    function issueIdentifier(entry) {
      const identifier = escapeHTML(entry.issue_identifier || entry.identifier || "n/a");
      const url = typeof entry.issue_url === "string" ? entry.issue_url.trim() : "";
      if (url.startsWith("http://") || url.startsWith("https://")) {
        return '<a class="issue-id issue-id-link" href="' + escapeHTML(url) + '" target="_blank" rel="noopener noreferrer" aria-label="Open ' + identifier + ' in the issue tracker">' + identifier + '</a>';
      }
      return '<span class="issue-id">' + identifier + '</span>';
    }

    function issueStack(entry) {
      const identifier = escapeHTML(entry.issue_identifier || entry.identifier || "");
      return '<div class="issue-stack">' +
        issueIdentifier(entry) +
        '<a class="issue-link" href="/api/v1/' + identifier + '">JSON details</a>' +
      '</div>';
    }

    function copyButton(sessionID) {
      if (!sessionID) return '<span class="muted">n/a</span>';
      return '<button type="button" class="subtle-button" data-label="Copy ID" data-copy="' + escapeHTML(sessionID) + '">Copy ID</button>';
    }

    function detailStack(entry) {
      const message = entry.last_message || entry.last_event || "n/a";
      const event = entry.last_event || "n/a";
      const at = entry.last_event_at ? ' · <span class="mono numeric">' + escapeHTML(entry.last_event_at) + '</span>' : "";
      return '<div class="detail-stack">' +
        '<span class="event-text" title="' + escapeHTML(message) + '">' + escapeHTML(message) + '</span>' +
        '<span class="muted event-meta">' + escapeHTML(event) + at + '</span>' +
      '</div>';
    }

    function tokenStack(tokens) {
      tokens = tokens || {};
      return '<div class="token-stack numeric">' +
        '<span>Total: ' + formatInt(tokens.total_tokens) + '</span>' +
        '<span class="muted">In ' + formatInt(tokens.input_tokens) + ' / Out ' + formatInt(tokens.output_tokens) + '</span>' +
      '</div>';
    }

    function renderMetrics(payload) {
      const totals = payload.codex_totals || {};
      const metrics = [
        ["Running", payload.counts?.running || 0, "Active issue sessions in the current runtime."],
        ["Retrying", payload.counts?.retrying || 0, "Issues waiting for the next retry window."],
        ["Blocked", payload.counts?.blocked || 0, "Issues paused for operator input or approval."],
        ["Total tokens", formatInt(totals.total_tokens), "In " + formatInt(totals.input_tokens) + " / Out " + formatInt(totals.output_tokens)],
        ["Runtime", formatRuntimeSeconds(totalRuntimeSeconds(payload)), "Total Codex runtime across completed and active sessions."],
      ];
      document.getElementById("metricGrid").innerHTML = metrics.map(metric =>
        '<article class="metric-card"><p class="metric-label">' + escapeHTML(metric[0]) + '</p><p class="metric-value numeric">' + escapeHTML(metric[1]) + '</p><p class="metric-detail">' + escapeHTML(metric[2]) + '</p></article>'
      ).join("");
    }

    function renderRunning(payload) {
      const running = payload.running || [];
      if (!running.length) {
        document.getElementById("runningSessions").innerHTML = '<p class="empty-state">No active sessions.</p>';
        return;
      }
      document.getElementById("runningSessions").innerHTML =
        '<div class="table-wrap"><table class="data-table data-table-running">' +
        '<colgroup><col style="width: 12rem;" /><col style="width: 8rem;" /><col style="width: 7.5rem;" /><col style="width: 8.5rem;" /><col /><col style="width: 10rem;" /></colgroup>' +
        '<thead><tr><th>Issue</th><th>State</th><th>Session</th><th>Runtime / turns</th><th>Codex update</th><th>Tokens</th></tr></thead><tbody>' +
        running.map(entry => '<tr>' +
          '<td>' + issueStack(entry) + '</td>' +
          '<td><span class="' + stateBadgeClass(entry.state) + '">' + escapeHTML(entry.state || "n/a") + '</span></td>' +
          '<td><div class="session-stack">' + copyButton(entry.session_id) + '</div></td>' +
          '<td class="numeric">' + escapeHTML(formatRuntimeAndTurns(entry.started_at, entry.turn_count)) + '</td>' +
          '<td>' + detailStack(entry) + '</td>' +
          '<td>' + tokenStack(entry.tokens) + '</td>' +
        '</tr>').join("") +
        '</tbody></table></div>';
    }

    function renderBlocked(payload) {
      const blocked = payload.blocked || [];
      if (!blocked.length) {
        document.getElementById("blockedSessions").innerHTML = '<p class="empty-state">No blocked sessions.</p>';
        return;
      }
      document.getElementById("blockedSessions").innerHTML =
        '<div class="table-wrap"><table class="data-table" style="min-width: 760px;"><thead><tr><th>Issue</th><th>State</th><th>Session</th><th>Blocked at</th><th>Last update</th><th>Error</th></tr></thead><tbody>' +
        blocked.map(entry => '<tr>' +
          '<td>' + issueStack(entry) + '</td>' +
          '<td><span class="' + stateBadgeClass(entry.state || "Blocked") + '">' + escapeHTML(entry.state || "Blocked") + '</span></td>' +
          '<td>' + copyButton(entry.session_id) + '</td>' +
          '<td class="mono">' + escapeHTML(entry.blocked_at || "n/a") + '</td>' +
          '<td>' + detailStack(entry) + '</td>' +
          '<td>' + escapeHTML(entry.error || "n/a") + '</td>' +
        '</tr>').join("") +
        '</tbody></table></div>';
    }

    function renderRetrying(payload) {
      const retrying = payload.retrying || [];
      if (!retrying.length) {
        document.getElementById("retryQueue").innerHTML = '<p class="empty-state">No issues are currently backing off.</p>';
        return;
      }
      document.getElementById("retryQueue").innerHTML =
        '<div class="table-wrap"><table class="data-table" style="min-width: 680px;"><thead><tr><th>Issue</th><th>Attempt</th><th>Due at</th><th>Error</th></tr></thead><tbody>' +
        retrying.map(entry => '<tr>' +
          '<td>' + issueStack(entry) + '</td>' +
          '<td>' + escapeHTML(entry.attempt || 0) + '</td>' +
          '<td class="mono">' + escapeHTML(entry.due_at || "n/a") + '</td>' +
          '<td>' + escapeHTML(entry.error || "n/a") + '</td>' +
        '</tr>').join("") +
        '</tbody></table></div>';
    }

    function renderDashboard(payload) {
      latestPayload = payload;
      const errorCard = document.getElementById("errorCard");
      const content = document.getElementById("dashboardContent");
      if (payload.error) {
        errorCard.hidden = false;
        content.hidden = true;
        document.getElementById("errorCopy").innerHTML = '<strong>' + escapeHTML(payload.error.code) + ':</strong> ' + escapeHTML(payload.error.message);
        return;
      }
      errorCard.hidden = true;
      content.hidden = false;
      renderMetrics(payload);
      document.getElementById("rateLimits").textContent = prettyValue(payload.rate_limits);
      renderRunning(payload);
      renderBlocked(payload);
      renderRetrying(payload);
    }

    async function refresh() {
      renderDashboard(await api("/api/v1/state"));
    }

    document.addEventListener("click", event => {
      const button = event.target.closest("[data-copy]");
      if (!button) return;
      navigator.clipboard.writeText(button.dataset.copy);
      button.textContent = "Copied";
      clearTimeout(button._copyTimer);
      button._copyTimer = setTimeout(() => { button.textContent = button.dataset.label; }, 1200);
    });

    refresh().catch(error => {
      document.getElementById("errorCard").hidden = false;
      document.getElementById("dashboardContent").hidden = true;
      document.getElementById("errorCopy").textContent = error.message;
    });
    setInterval(() => refresh().catch(() => {}), 2500);
    setInterval(() => { if (latestPayload) renderMetrics(latestPayload); }, 1000);
  </script>
</body>
</html>`

const tasksHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Symphony Tasks</title>
  <link rel="stylesheet" href="/dashboard.css" />
  <style>
    .error-card { display: none; }
    .section-header { margin-bottom: 1rem; }
    button.ghost { min-height: 2rem; background: rgba(5, 9, 8, 0.72); color: var(--muted-strong); border-color: var(--line-strong); box-shadow: none; padding: 0.32rem 0.56rem; font-size: 0.78rem; }
    button.ghost:hover { background: var(--accent-soft); color: var(--accent-hot); box-shadow: none; }
    .form-grid { display: grid; grid-template-columns: 1fr; gap: 0.75rem; align-items: end; }
    label { display: grid; gap: 0.32rem; color: var(--muted); font-size: 0.78rem; font-weight: 900; letter-spacing: 0.04em; text-transform: uppercase; }
    input, textarea { width: 100%; border: 1px solid var(--line-strong); border-radius: var(--radius-md); background: rgba(2, 5, 4, 0.82); color: var(--ink); padding: 0.72rem 0.8rem; outline: none; box-shadow: inset 0 0 1rem rgba(0, 0, 0, 0.22); font: inherit; caret-color: var(--accent); transition: border-color 140ms ease, box-shadow 140ms ease, background 140ms ease; }
    input::placeholder, textarea::placeholder { color: rgba(181, 212, 200, 0.48); }
    input:focus, textarea:focus { background: rgba(5, 11, 9, 0.95); border-color: var(--accent); box-shadow: inset 0 0 1rem rgba(0, 0, 0, 0.28), var(--glow); }
    textarea { min-height: 5.5rem; resize: vertical; }
    .board-grid { display: grid; grid-template-columns: 1fr; gap: 0.85rem; align-items: start; }
    .board-column { min-height: 18rem; max-height: 34rem; border: 1px solid var(--line); border-radius: var(--radius-lg); background: rgba(3, 8, 6, 0.62); padding: 0.7rem; overflow: hidden; box-shadow: inset 0 0 1.5rem rgba(0, 0, 0, 0.18); }
    .column-title { position: sticky; top: 0; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; margin: 0 0 0.7rem; padding-bottom: 0.55rem; border-bottom: 1px solid var(--line); color: var(--accent-hot); font-size: 0.72rem; font-weight: 900; text-transform: uppercase; letter-spacing: 0.06em; background: rgba(3, 8, 6, 0.86); }
    .column-count { min-width: 1.7rem; border: 1px solid var(--line-strong); border-radius: var(--radius-md); padding: 0.18rem 0.38rem; color: var(--ink); text-align: center; }
    .task-list { display: grid; gap: 0.62rem; max-height: 29.5rem; overflow: auto; padding-right: 0.18rem; }
    .task-card { position: relative; background: linear-gradient(180deg, rgba(20, 34, 29, 0.92), rgba(8, 15, 13, 0.94)); border: 1px solid var(--line); box-shadow: var(--shadow-sm); border-radius: var(--radius-lg); padding: 0.78rem; display: grid; gap: 0.58rem; animation: boot-sequence 360ms ease both; }
    .task-card::before { position: absolute; inset: 0; pointer-events: none; content: ""; border-left: 2px solid var(--accent); opacity: 0.56; }
    .task-card:hover, .task-card-selected { border-color: var(--accent); box-shadow: var(--glow); }
    .task-top { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.55rem; min-width: 0; }
    .task-heading { min-width: 0; }
    .task-id { color: var(--accent-hot); font-size: 0.78rem; font-weight: 900; letter-spacing: 0; }
    .task-title { margin: 0.12rem 0 0; color: var(--ink); font-weight: 900; line-height: 1.28; overflow-wrap: anywhere; }
    .task-desc { margin: 0; color: var(--muted-strong); font-size: 0.84rem; max-height: 5rem; overflow: auto; padding-right: 0.15rem; }
    .task-meta { display: flex; flex-wrap: wrap; gap: 0.35rem; color: var(--muted); font-size: 0.72rem; }
    .task-chip { display: inline-flex; align-items: center; min-height: 1.45rem; border: 1px solid var(--line); border-radius: var(--radius-md); background: rgba(66, 242, 178, 0.07); padding: 0.16rem 0.4rem; color: var(--muted-strong); }
    .task-actions { display: flex; flex-wrap: wrap; gap: 0.38rem; }
    .queue-controls { display: flex; align-items: center; flex-wrap: wrap; gap: 0.5rem; }
    .stream-list { display: grid; gap: 0.55rem; max-height: 31rem; overflow: auto; padding-right: 0.2rem; }
    .stream-entry { border: 1px solid var(--line); border-radius: var(--radius-md); background: rgba(4, 9, 8, 0.82); padding: 0.65rem 0.72rem; display: grid; gap: 0.38rem; animation: boot-sequence 260ms ease both; }
    .stream-entry:hover { border-color: var(--line-strong); background: rgba(13, 24, 20, 0.9); }
    .stream-entry-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.75rem; color: var(--muted); font-size: 0.74rem; }
    .stream-entry-title { display: flex; align-items: center; flex-wrap: wrap; gap: 0.38rem; min-width: 0; }
    .stream-event { color: var(--accent-hot); font-weight: 900; }
    .stream-summary { margin: 0; color: var(--muted-strong); font-size: 0.86rem; line-height: 1.45; word-break: break-word; white-space: pre-wrap; }
    .patch-grid { display: grid; grid-template-columns: 1fr; gap: 0.75rem; align-items: start; }
    .patch-list { display: grid; gap: 0.55rem; color: var(--muted); font-size: 0.84rem; max-height: 28rem; overflow: auto; }
    .patch-meta-card { border: 1px solid var(--line); border-radius: var(--radius-md); background: rgba(4, 9, 8, 0.72); padding: 0.65rem; }
    .patch-meta-card strong { color: var(--accent-hot); }
    .patch-panel { max-height: 36rem; white-space: pre; }
    .agent-island { position: fixed; left: 50%; bottom: 1rem; z-index: 40; width: min(calc(100vw - 1rem), 50rem); transform: translateX(-50%); border: 1px solid var(--line-strong); border-radius: var(--radius-lg); background: rgba(4, 9, 8, 0.96); box-shadow: var(--shadow-lg); backdrop-filter: blur(18px); overflow: hidden; }
    .agent-island-top { display: grid; grid-template-columns: minmax(0, 1fr); gap: 0.6rem; padding: 0.68rem; border-bottom: 1px solid var(--line); background: linear-gradient(90deg, rgba(66, 242, 178, 0.12), transparent); }
    .agent-island-title { display: flex; align-items: center; gap: 0.5rem; min-width: 0; font-weight: 900; }
    .agent-island-dot { width: 0.55rem; height: 0.55rem; border-radius: 999px; background: var(--muted); flex: 0 0 auto; }
    .agent-island-dot-active { background: var(--accent); box-shadow: 0 0 1rem var(--accent); animation: pulse-dot 1500ms ease-in-out infinite; }
    .agent-island-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .agent-island-meta { color: var(--muted); font-size: 0.76rem; white-space: nowrap; }
    .agent-island-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; }
    .agent-chip-list { display: flex; flex-wrap: wrap; gap: 0.32rem; min-width: 0; }
    .agent-chip { min-height: 2rem; padding: 0.3rem 0.5rem; border-radius: var(--radius-md); border-color: var(--line-strong); background: rgba(5, 10, 8, 0.82); color: var(--muted-strong); font-size: 0.76rem; box-shadow: none; }
    .agent-chip-active { color: var(--accent-hot); border-color: var(--accent); background: var(--accent-soft); }
    .agent-island-button { min-height: 2rem; padding: 0.3rem 0.5rem; font-size: 0.76rem; }
    .agent-island-body { padding: 0.7rem 0.72rem 0.8rem; display: grid; gap: 0.45rem; max-height: 15rem; overflow: auto; }
    .agent-island-line { display: grid; gap: 0.22rem; padding: 0.48rem 0.55rem; border: 1px solid var(--line); border-radius: var(--radius-md); background: rgba(12, 22, 19, 0.9); }
    .agent-island-line-head { display: flex; justify-content: space-between; gap: 0.6rem; color: var(--muted); font-size: 0.72rem; }
    .agent-island-line-text { margin: 0; font-size: 0.84rem; color: var(--muted-strong); line-height: 1.4; word-break: break-word; white-space: pre-wrap; }
    .agent-island[data-collapsed="true"] .agent-island-body { display: none; }
    .agent-island[data-collapsed="true"] .agent-island-top { border-bottom: 0; }
    .code-panel { overflow: auto; }
    @keyframes pulse-dot { 0%, 100% { opacity: 0.72; transform: scale(0.92); } 50% { opacity: 1; transform: scale(1.08); } }
    @media (min-width: 768px) { .form-grid { grid-template-columns: minmax(14rem, 0.8fr) minmax(20rem, 1.6fr) auto; } .board-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .agent-island-top { grid-template-columns: minmax(0, 1fr) auto; align-items: center; } .agent-island-actions { justify-content: flex-end; } }
    @media (min-width: 1180px) { .board-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); } .patch-grid { grid-template-columns: minmax(0, 0.62fr) minmax(0, 1.38fr); } }
    @media (max-width: 560px) { .task-actions button { flex: 1 1 6.5rem; min-height: 2.75rem; } .stream-entry-head, .agent-island-line-head { flex-direction: column; } }
  </style>
</head>
<body>
  <main class="app-shell">
    <nav class="top-nav" aria-label="Primary navigation">
      <a class="top-nav-brand" href="/">
        <span class="top-nav-mark">S</span>
        <span>Symphony</span>
      </a>
      <div class="top-nav-links">
        <a class="top-nav-link" href="/">Observability</a>
        <a class="top-nav-link top-nav-link-active" href="/tasks">Tasks</a>
      </div>
    </nav>
    <section class="dashboard-shell">
      <header class="hero-card">
        <div class="hero-grid">
          <div>
            <p class="eyebrow">Symphony Command</p>
            <h1 class="hero-title">Task Control Deck</h1>
            <p class="hero-copy">Create, dispatch, inspect, and recover local Codex work from one dense console surface.</p>
          </div>
          <div class="status-stack">
            <span class="status-badge status-badge-live" style="display: inline-flex;">
              <span class="status-badge-dot"></span>
              Live
            </span>
            <span class="status-badge" id="trackerKind">Tracker</span>
          </div>
        </div>
      </header>

      <section class="error-card" id="errorCard"></section>

      <section class="metric-grid">
        <article class="metric-card"><p class="metric-label">Tasks</p><p class="metric-value numeric" id="metricTasks">0</p><p class="metric-detail">Local tasks in this Symphony workspace.</p></article>
        <article class="metric-card"><p class="metric-label">Running</p><p class="metric-value numeric" id="metricRunning">0</p><p class="metric-detail">Active Codex sessions.</p></article>
        <article class="metric-card"><p class="metric-label">Retrying</p><p class="metric-value numeric" id="metricRetrying">0</p><p class="metric-detail">Tasks waiting for retry.</p></article>
        <article class="metric-card"><p class="metric-label">Blocked</p><p class="metric-value numeric" id="metricBlocked">0</p><p class="metric-detail">Tasks paused for input or approval.</p></article>
      </section>

      <section class="section-card">
        <div class="section-header">
          <div>
            <h2 class="section-title">New directive</h2>
            <p class="section-copy">Write the work here. Symphony stores it locally and can dispatch it to Codex.</p>
          </div>
        </div>
        <form id="taskForm" class="form-grid">
          <label>Title<input id="taskTitle" name="title" required placeholder="Tighten combat audit logging" /></label>
          <label>Description<textarea id="taskDescription" name="description" placeholder="Acceptance criteria, constraints, notes..."></textarea></label>
          <label>Agent<input id="taskBackend" name="agent_backend" placeholder="codex or crush" /></label>
          <label>Model<input id="taskModel" name="agent_model" placeholder="zai/glm-5.2 or openrouter/sakana/fugu-ultra" /></label>
          <label>Endpoint<input id="taskEndpoint" name="agent_endpoint" placeholder="optional API base URL" /></label>
          <button type="submit">Create & run</button>
        </form>
      </section>

      <section class="section-card">
        <div class="section-header">
          <div>
            <h2 class="section-title">Task board</h2>
            <p class="section-copy">Lane view for local tracker state, active runs, and follow-up review.</p>
          </div>
          <div class="queue-controls">
            <span class="state-badge" id="autoRunState">Auto-run</span>
            <button class="secondary" id="autoRunButton" type="button">Pause auto-run</button>
            <button class="secondary" id="refreshButton" type="button">Refresh</button>
          </div>
        </div>
        <div class="board-grid" id="taskBoard"></div>
      </section>

      <section class="section-card" id="streamSection">
        <div class="section-header">
          <div>
            <h2 class="section-title">Run stream</h2>
            <p class="section-copy" id="streamSubtitle">Select a task to inspect its latest agent events.</p>
          </div>
        </div>
        <div class="stream-list" id="streamList"><p class="empty-state">No task selected.</p></div>
      </section>

      <section class="section-card" id="patchSection">
        <div class="section-header">
          <div>
            <h2 class="section-title">Patch review</h2>
            <p class="section-copy" id="patchSubtitle">Select a task to inspect its workspace diff.</p>
          </div>
        </div>
        <div class="patch-grid">
          <div class="patch-list" id="patchMeta"><p class="empty-state">No task selected.</p></div>
          <pre class="code-panel patch-panel" id="patchDiff">No patch loaded.</pre>
        </div>
      </section>

      <section class="section-card">
        <div class="section-header"><div><h2 class="section-title">Raw state</h2><p class="section-copy">Debug payload from /api/v1/raw-state.</p></div></div>
        <pre class="code-panel" id="rawState">Loading...</pre>
      </section>
    </section>
  </main>
  <aside class="agent-island" id="agentIsland" data-collapsed="false" aria-live="polite">
    <div class="agent-island-top">
      <div class="agent-island-title">
        <span class="agent-island-dot" id="agentIslandDot"></span>
        <span class="agent-island-label" id="agentIslandTitle">Agent stream</span>
        <span class="agent-island-meta" id="agentIslandMeta">No active agent</span>
      </div>
      <div class="agent-island-actions">
        <div class="agent-chip-list" id="agentIslandChips"></div>
        <button class="secondary agent-island-button" id="agentIslandFollow" type="button">Follow latest</button>
        <button class="secondary agent-island-button" id="agentIslandToggle" type="button">Collapse</button>
      </div>
    </div>
    <div class="agent-island-body" id="agentIslandBody">
      <p class="empty-state">No active agent.</p>
    </div>
  </aside>
  <script>
    const columns = ["Todo", "In Progress", "Blocked", "Done"];
    const board = document.getElementById("taskBoard");
    const rawState = document.getElementById("rawState");
    const errorCard = document.getElementById("errorCard");
    let selectedTaskID = "";
    let selectedTaskIdentifier = "";
    let selectedPatchID = "";
    let selectedPatchIdentifier = "";
    let islandTaskID = "";
    let islandPinned = false;
    let islandCollapsed = false;
    let autoRun = true;

    async function api(path, options = {}) {
      const response = await fetch(path, {
        headers: { "Content-Type": "application/json", ...(options.headers || {}) },
        ...options,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || response.statusText);
      }
      return response.json();
    }

    function escapeHTML(value) {
      return String(value ?? "").replace(/[&<>"']/g, char => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" }[char]));
    }

    function badgeClass(state) {
      const value = String(state || "").toLowerCase();
      if (["progress", "running", "active"].some(part => value.includes(part))) return "state-badge state-badge-active";
      if (["blocked", "error", "failed"].some(part => value.includes(part))) return "state-badge state-badge-danger";
      if (["todo", "queued", "pending", "retry"].some(part => value.includes(part))) return "state-badge state-badge-warning";
      return "state-badge";
    }

    function compactTime(value) {
      if (!value) return "no timestamp";
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return "bad timestamp";
      return date.toLocaleString("en-US", { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit" });
    }

    function taskMeta(task) {
      const labels = Array.isArray(task.labels) ? task.labels : [];
      const chips = labels.length
        ? labels.slice(0, 3).map(label => '<span class="task-chip">#' + escapeHTML(label) + '</span>').join("")
        : '<span class="task-chip">no labels</span>';
      const overflow = labels.length > 3 ? '<span class="task-chip">+' + escapeHTML(labels.length - 3) + '</span>' : "";
      const priority = Number.isInteger(task.priority) ? '<span class="task-chip">P' + escapeHTML(task.priority) + '</span>' : "";
      const backend = task.agent_backend ? '<span class="task-chip">' + escapeHTML(task.agent_backend) + '</span>' : "";
      const model = task.agent_model ? '<span class="task-chip">' + escapeHTML(task.agent_model) + '</span>' : "";
      const endpoint = task.agent_endpoint ? '<span class="task-chip">endpoint</span>' : "";
      return '<div class="task-meta">' +
        '<span class="task-chip">updated ' + escapeHTML(compactTime(task.updated_at || task.created_at)) + '</span>' +
        priority +
        backend +
        model +
        endpoint +
        chips +
        overflow +
      '</div>';
    }

    function taskCard(task) {
      const desc = task.description ? '<p class="task-desc">' + escapeHTML(task.description) + '</p>' : '<p class="task-desc">No description.</p>';
      const selected = task.id === selectedTaskID || task.id === selectedPatchID ? " task-card-selected" : "";
      return '<article class="task-card' + selected + '">' +
        '<div class="task-top">' +
          '<div class="task-heading"><div class="task-id">' + escapeHTML(task.identifier) + '</div><div class="task-title">' + escapeHTML(task.title) + '</div></div>' +
          '<span class="' + badgeClass(task.state) + '">' + escapeHTML(task.state || "Todo") + '</span>' +
        '</div>' +
        desc +
        taskMeta(task) +
        '<div class="task-actions">' +
          '<button class="ghost" type="button" data-run="' + escapeHTML(task.id) + '">Run</button>' +
          '<button class="ghost" type="button" data-stream="' + escapeHTML(task.id) + '" data-identifier="' + escapeHTML(task.identifier) + '">Stream</button>' +
          '<button class="ghost" type="button" data-patch="' + escapeHTML(task.id) + '" data-identifier="' + escapeHTML(task.identifier) + '">Patch</button>' +
          '<button class="ghost" type="button" data-state="' + escapeHTML(task.id) + '" data-next="Todo">Todo</button>' +
          '<button class="ghost" type="button" data-state="' + escapeHTML(task.id) + '" data-next="Done">Done</button>' +
        '</div>' +
      '</article>';
    }

    function renderBoard(tasks) {
      board.innerHTML = columns.map(column => {
        const items = tasks.filter(task => (task.state || "Todo") === column);
        return '<section class="board-column">' +
          '<h3 class="column-title"><span>' + escapeHTML(column) + '</span><span class="column-count">' + items.length + '</span></h3>' +
          '<div class="task-list">' + (items.length ? items.map(taskCard).join("") : '<p class="empty-state">No tasks.</p>') + '</div>' +
        '</section>';
      }).join("");
    }

    function renderAutoRun(raw) {
      autoRun = !raw.auto_run_paused;
      const state = document.getElementById("autoRunState");
      const button = document.getElementById("autoRunButton");
      state.textContent = autoRun ? "Auto-run: on" : "Auto-run: paused";
      state.className = autoRun ? "state-badge state-badge-active" : "state-badge state-badge-warning";
      button.textContent = autoRun ? "Pause auto-run" : "Resume auto-run";
    }

    function renderStream(events) {
      const list = document.getElementById("streamList");
      if (!selectedTaskID) {
        document.getElementById("streamSubtitle").textContent = "Select a task to inspect its latest agent events.";
        list.innerHTML = '<p class="empty-state">No task selected.</p>';
        return;
      }
      document.getElementById("streamSubtitle").textContent = "Latest events for " + selectedTaskIdentifier + ".";
      if (!events.length) {
        list.innerHTML = '<p class="empty-state">No stream events yet.</p>';
        return;
      }
      list.innerHTML = events.slice().reverse().map(event =>
        '<article class="stream-entry">' +
          '<div class="stream-entry-head">' +
            '<span class="stream-entry-title"><span class="stream-event">' + escapeHTML(event.title || event.event || "Event") + '</span><span>' + escapeHTML(event.kind || "raw") + '</span><span>turn ' + escapeHTML(event.turn_count || 0) + '</span></span>' +
            '<span class="mono numeric">' + escapeHTML(event.timestamp || "n/a") + '</span>' +
          '</div>' +
          '<p class="stream-summary">' + escapeHTML(event.body || event.summary || JSON.stringify(event.details || {})) + '</p>' +
        '</article>'
      ).join("");
    }

    async function refreshSelectedStream() {
      if (!selectedTaskID) {
        renderStream([]);
        return;
      }
      const payload = await api("/api/v1/tasks/" + encodeURIComponent(selectedTaskID) + "/stream");
      renderStream(payload.display_events || payload.events || []);
    }

    function renderPatch(packet) {
      const meta = document.getElementById("patchMeta");
      const diff = document.getElementById("patchDiff");
      if (!selectedPatchID) {
        document.getElementById("patchSubtitle").textContent = "Select a task to inspect its workspace diff.";
        meta.innerHTML = '<p class="empty-state">No task selected.</p>';
        diff.textContent = "No patch loaded.";
        return;
      }
      document.getElementById("patchSubtitle").textContent = "Workspace diff for " + selectedPatchIdentifier + ".";
      meta.innerHTML =
        '<div class="patch-meta-card"><strong>State</strong><br>' + escapeHTML(packet.state || "n/a") + '</div>' +
        '<div class="patch-meta-card"><strong>Workspace</strong><br><span class="mono">' + escapeHTML(packet.workspace_path || "n/a") + '</span></div>' +
        '<div class="patch-meta-card"><strong>Status</strong><pre class="code-panel">' + escapeHTML(packet.git_status || "clean") + '</pre></div>' +
        '<div class="patch-meta-card"><strong>Untracked</strong><br>' + escapeHTML((packet.untracked_files || []).join(", ") || "none") + '</div>' +
        '<div class="patch-meta-card"><strong>Stat</strong><pre class="code-panel">' + escapeHTML(packet.diff_stat || "No tracked diff.") + '</pre></div>';
      diff.textContent = packet.patch || packet.tracked_diff || "No patch changes.";
    }

    async function refreshSelectedPatch() {
      if (!selectedPatchID) {
        renderPatch(null);
        return;
      }
      const packet = await api("/api/v1/tasks/" + encodeURIComponent(selectedPatchID) + "/workspace-diff");
      renderPatch(packet);
    }

    function sortedRunningEntries(raw) {
      const running = Object.values(raw.running || {});
      return running.sort((left, right) => {
        const leftAt = Date.parse(left.last_codex_timestamp || left.started_at || "") || 0;
        const rightAt = Date.parse(right.last_codex_timestamp || right.started_at || "") || 0;
        return rightAt - leftAt;
      });
    }

    function renderAgentIslandChips(entries) {
      const chips = document.getElementById("agentIslandChips");
      chips.innerHTML = entries.map(entry =>
        '<button class="agent-chip ' + (entry.issue_id === islandTaskID ? 'agent-chip-active' : '') + '" type="button" data-island-task="' + escapeHTML(entry.issue_id) + '" data-island-identifier="' + escapeHTML(entry.identifier || entry.issue?.identifier || entry.issue_id) + '">' +
          escapeHTML(entry.identifier || entry.issue?.identifier || entry.issue_id) +
        '</button>'
      ).join("");
    }

    function renderAgentIsland(raw, events) {
      const entries = sortedRunningEntries(raw);
      const current = entries.find(entry => entry.issue_id === islandTaskID);
      const dot = document.getElementById("agentIslandDot");
      const title = document.getElementById("agentIslandTitle");
      const meta = document.getElementById("agentIslandMeta");
      const body = document.getElementById("agentIslandBody");
      const follow = document.getElementById("agentIslandFollow");
      renderAgentIslandChips(entries);
      follow.hidden = !islandPinned;
      if (!current) {
        dot.className = "agent-island-dot";
        title.textContent = "Agent stream";
        meta.textContent = "No active agent";
        body.innerHTML = '<p class="empty-state">No active agent.</p>';
        return;
      }
      const identifier = current.identifier || current.issue?.identifier || current.issue_id;
      dot.className = "agent-island-dot agent-island-dot-active";
      title.textContent = identifier;
      meta.textContent = (current.last_codex_event || "running") + " · turn " + (current.turn_count || 0);
      if (!events.length) {
        body.innerHTML = '<p class="empty-state">Agent is running; no stream events yet.</p>';
        return;
      }
      body.innerHTML = events.slice(-8).reverse().map(event =>
        '<article class="agent-island-line">' +
          '<div class="agent-island-line-head"><span>' + escapeHTML(event.title || event.event || "Event") + ' · ' + escapeHTML(event.kind || "raw") + '</span><span class="mono numeric">' + escapeHTML(event.timestamp || "n/a") + '</span></div>' +
          '<p class="agent-island-line-text">' + escapeHTML(event.body || event.summary || JSON.stringify(event.details || {})) + '</p>' +
        '</article>'
      ).join("");
    }

    async function refreshAgentIsland(raw) {
      const entries = sortedRunningEntries(raw);
      if (!islandPinned) {
        islandTaskID = entries[0]?.issue_id || "";
      } else if (!entries.some(entry => entry.issue_id === islandTaskID)) {
        islandPinned = false;
        islandTaskID = entries[0]?.issue_id || "";
      }
      if (!islandTaskID) {
        renderAgentIsland(raw, []);
        return;
      }
      const payload = await api("/api/v1/tasks/" + encodeURIComponent(islandTaskID) + "/stream");
      renderAgentIsland(raw, payload.display_events || payload.events || []);
    }

    async function refresh() {
      try {
        const [state, tasksPayload, raw] = await Promise.all([api("/api/v1/state"), api("/api/v1/tasks"), api("/api/v1/raw-state")]);
        const tasks = tasksPayload.tasks || [];
        errorCard.style.display = "none";
        document.getElementById("trackerKind").textContent = "tracker: " + (raw.tracker_kind || "local");
        renderAutoRun(raw);
        document.getElementById("metricTasks").textContent = tasks.length;
        document.getElementById("metricRunning").textContent = state.counts?.running || 0;
        document.getElementById("metricRetrying").textContent = state.counts?.retrying || 0;
        document.getElementById("metricBlocked").textContent = state.counts?.blocked || 0;
        renderBoard(tasks);
        await refreshSelectedStream();
        await refreshSelectedPatch();
        await refreshAgentIsland(raw);
        rawState.textContent = JSON.stringify(raw, null, 2);
      } catch (error) {
        errorCard.style.display = "block";
        errorCard.textContent = error.message;
      }
    }

    document.getElementById("taskForm").addEventListener("submit", async event => {
      event.preventDefault();
      const title = document.getElementById("taskTitle").value;
      const description = document.getElementById("taskDescription").value;
      const agent_backend = document.getElementById("taskBackend").value.trim();
      const agent_model = document.getElementById("taskModel").value.trim();
      const agent_endpoint = document.getElementById("taskEndpoint").value.trim();
      await api("/api/v1/tasks", { method: "POST", body: JSON.stringify({ title, description, agent_backend, agent_model, agent_endpoint }) });
      event.target.reset();
      await refresh();
    });

    document.getElementById("refreshButton").addEventListener("click", async () => {
      await api("/api/v1/refresh", { method: "POST", body: "{}" }).catch(() => {});
      await refresh();
    });

    document.getElementById("autoRunButton").addEventListener("click", async () => {
      await api("/api/v1/autorun", { method: "PATCH", body: JSON.stringify({ auto_run: !autoRun }) });
      await refresh();
    });

    document.getElementById("agentIslandToggle").addEventListener("click", () => {
      islandCollapsed = !islandCollapsed;
      document.getElementById("agentIsland").dataset.collapsed = islandCollapsed ? "true" : "false";
      document.getElementById("agentIslandToggle").textContent = islandCollapsed ? "Expand" : "Collapse";
    });

    document.getElementById("agentIslandFollow").addEventListener("click", async () => {
      islandPinned = false;
      await refresh();
    });

    document.getElementById("agentIslandChips").addEventListener("click", async event => {
      const chip = event.target.closest("[data-island-task]");
      if (!chip) return;
      islandPinned = true;
      islandTaskID = chip.dataset.islandTask;
      selectedTaskID = chip.dataset.islandTask;
      selectedTaskIdentifier = chip.dataset.islandIdentifier || chip.dataset.islandTask;
      await refreshSelectedStream();
      await refresh();
    });

    board.addEventListener("click", async event => {
      const run = event.target.closest("[data-run]");
      const stream = event.target.closest("[data-stream]");
      const patch = event.target.closest("[data-patch]");
      const state = event.target.closest("[data-state]");
      if (run) {
        await api("/api/v1/tasks/" + run.dataset.run + "/run", { method: "POST", body: "{}" });
        await refresh();
      } else if (stream) {
        selectedTaskID = stream.dataset.stream;
        selectedTaskIdentifier = stream.dataset.identifier || selectedTaskID;
        await refreshSelectedStream();
      } else if (patch) {
        selectedPatchID = patch.dataset.patch;
        selectedPatchIdentifier = patch.dataset.identifier || selectedPatchID;
        await refreshSelectedPatch();
      } else if (state) {
        await api("/api/v1/tasks/" + state.dataset.state + "/state", { method: "PATCH", body: JSON.stringify({ state: state.dataset.next }) });
        await refresh();
      }
    });

    refresh();
    setInterval(refresh, 2500);
  </script>
</body>
</html>`

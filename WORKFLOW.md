---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: "REPLACE_WITH_LINEAR_PROJECT_SLUG"
  required_labels:
    - symphony
  active_states:
    - Todo
    - In Progress
  terminal_states:
    - Done
    - Closed
    - Cancelled
    - Canceled
    - Duplicate

polling:
  interval_ms: 30000

workspace:
  root: .symphony/workspaces

hooks:
  after_create: |
    set -euo pipefail
    : "${SOURCE_REPO_URL:?Set SOURCE_REPO_URL before starting Symphony.}"
    git clone "$SOURCE_REPO_URL" .

    if [ -f package.json ]; then
      if [ -f package-lock.json ]; then
        npm ci
      else
        npm install
      fi
    fi

    if [ -f requirements.txt ]; then
      python3 -m pip install -r requirements.txt
    fi
  before_run: |
    git status --short
  after_run: |
    echo "Symphony run finished for $(pwd)"
  timeout_ms: 120000

agent:
  max_concurrent_agents: 2
  max_turns: 12
  max_retry_backoff_ms: 300000
  max_concurrent_agents_by_state:
    "in progress": 1

codex:
  command: "${CODEX_BIN:-codex} --config shell_environment_policy.inherit=all app-server"
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    networkAccess: true
  turn_timeout_ms: 3600000
  stall_timeout_ms: 300000
---

You are working inside a Symphony-managed workspace for Linear issue `{{ issue.identifier }}`.

Issue context:
- Title: {{ issue.title }}
- Status: {{ issue.state }}
- URL: {{ issue.url }}
- Labels: {{ issue.labels }}

Description:
{% if issue.description %}
{{ issue.description }}
{% else %}
No description provided.
{% endif %}

{% if attempt %}
This is retry/continuation attempt #{{ attempt }}. Continue from the current workspace state and avoid repeating completed work unless validation requires it.
{% endif %}

Operating rules:
1. Work only inside the current repository workspace.
2. Read local repo instructions first, especially any `AGENTS.md`, `README`, and project docs.
3. Keep the implementation scoped to this issue. File follow-up work separately instead of expanding scope.
4. Before changing code, identify the current behavior or missing behavior and write down a concise plan.
5. After changing code, run the narrowest useful validation for the touched area.
6. Do not commit secrets, generated dependency folders, local logs, or `.symphony` workspaces.
7. If you are blocked by missing credentials, missing issue details, unavailable tooling, or repository access, stop with a concise blocker report that names the exact missing item.

Completion bar:
- The issue request is implemented or explicitly blocked.
- Relevant validation has run and the outcome is reported.
- Any created branch, commit, PR, or follow-up issue is referenced in the final report when available.

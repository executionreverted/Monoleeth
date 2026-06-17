---
tracker:
  kind: local
  local_root: .symphony/tasks
  required_labels:
    - local
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
    git clone file:///Users/canersevince/gameproject .

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
  max_concurrent_agents: 4
  max_turns: 12
  max_retry_backoff_ms: 300000
  max_concurrent_agents_by_state:
    "in progress": 4

codex:
  command: "${CODEX_BIN:-codex} --config shell_environment_policy.inherit=all app-server"
  thread_sandbox: workspace-write
  turn_sandbox_policy:
    type: workspaceWrite
    networkAccess: true
  turn_timeout_ms: 3600000
  stall_timeout_ms: 300000

server:
  host: 0.0.0.0
  port: 4000
---

You are working inside a Symphony-managed workspace for local task `{{ issue.identifier }}`.

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
2. Do not read or follow `AGENTS.md` or `docs/symphony-operating-model.md`; those are for the main Codex project-manager session.
3. Read `docs/symphony-worker-rules.md` before changing files.
4. Do not spawn subagents, create Symphony tasks, dispatch agents, or manage the Symphony queue.
5. For gameplay, economy, world, client, infrastructure, or observability work, read `docs/roadmap/00-index.md` and the matching phase file under `docs/roadmap/` before implementing.
6. Use the roadmap phase file as the working checklist: follow its TODO order where applicable, use its tests and abuse/safety checks, and check its done criteria before claiming completion.
7. Read the relevant module spec under `docs/plans/modules/` for the module being touched.
8. Keep the implementation scoped to this issue, module, and roadmap phase. File follow-up work separately instead of expanding scope.
9. Before changing code, identify the current behavior or missing behavior and write down a concise plan.
10. After changing code, run the narrowest useful validation for the touched area.
11. Update the relevant roadmap phase file for tasks actually completed. Check off only TODOs that were implemented and verified; leave unfinished TODOs unchecked.
12. Do not commit secrets, generated dependency folders, local logs, or `.symphony` workspaces.
13. If you are blocked by missing credentials, missing issue details, unavailable tooling, or repository access, stop with a concise blocker report that names the exact missing item.

Completion bar:
- The issue request is implemented or explicitly blocked.
- The relevant roadmap phase file was checked and updated when implementation progress changed.
- Relevant validation has run and the outcome is reported.
- The final report names the roadmap phase or phases touched.
- Any created branch, commit, PR, or follow-up issue is referenced in the final report when available.

# Go Symphony Port Design

Goal: implement a Go reference version of OpenAI Symphony for this repo, based on `openai/symphony` `SPEC.md` and the Elixir prototype behavior.

Scope:

- Preserve the same repo-owned `WORKFLOW.md` contract.
- Keep the same guardrails acknowledgement flag.
- Poll Linear via GraphQL, normalize issues, filter required labels, and dispatch active issues.
- Create one workspace per issue, run lifecycle hooks, and launch `codex app-server` over stdio.
- Speak the Codex app-server JSONL protocol: initialize, start thread, start turn, process completion/failure/approval/tool notifications.
- Expose structured logs plus `/api/v1/state`, `/api/v1/{issue_identifier}`, and `/api/v1/refresh`.

Implementation choices:

- Go package: `internal/symphony`.
- CLI entrypoint: `cmd/symphony`.
- YAML: `go.yaml.in/yaml/v4`.
- Liquid rendering: small in-repo renderer for the subset used by Symphony workflows: `{{ path }}`, `{% if path %}`, `{% else %}`, `{% endif %}`.
- SSH workers and rich dashboard are documented as later extensions; local worker behavior is implemented first.

Validation:

- Unit tests for workflow/config parsing, prompt rendering, workspace safety/hooks, Linear normalization, and orchestrator selection helpers.
- `go test ./...`.

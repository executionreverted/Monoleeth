# Symphony setup

Symphony is not a normal app dependency for this repo. It is an orchestrator that reads this repo's `WORKFLOW.md`, polls Linear, creates one isolated workspace per issue, and launches `codex app-server` inside that workspace.

This repo includes a Go implementation under `cmd/symphony`, based on OpenAI Symphony's language-agnostic `SPEC.md` and Elixir prototype behavior.

Sources:

- OpenAI announcement: https://openai.com/index/open-source-codex-orchestration-symphony/
- Symphony repo: https://github.com/openai/symphony
- Elixir reference implementation used as the behavior reference: https://github.com/openai/symphony/blob/main/elixir/README.md

## What is already in this repo

- `WORKFLOW.md`: the repo-owned Symphony contract.
- `.env.example`: local environment variables to export before running.
- `.gitignore`: ignores local Symphony workspaces, logs, and env files.

## Prerequisites

1. Codex CLI must be installed and authenticated.
   - This machine currently has Codex available as `codex`.
2. Linear must have a project for this repo.
3. Create a Linear personal API token:
   - Linear -> Settings -> Security & access -> Personal API keys.
4. Go must be installed. This repo was verified with Go 1.25.1.

## Configure this repo

1. Copy the example env file or export the values directly:

```bash
cp .env.example .env
```

2. Fill in at least:

```bash
LINEAR_API_KEY=lin_api_...
SOURCE_REPO_URL=file:///Users/canersevince/gameproject
```

Use the local `file://` URL for smoke tests before the GitHub remote exists. After the repo is on GitHub, prefer the SSH remote:

```bash
SOURCE_REPO_URL=git@github.com:your-org/gameproject.git
```

3. Edit `WORKFLOW.md` and replace:

```yaml
project_slug: "REPLACE_WITH_LINEAR_PROJECT_SLUG"
```

To find the slug, open the Linear project, copy its URL, and use the slug segment from that URL.

4. Add the `symphony` label to Linear issues you want Symphony to run. This repo's workflow requires that label so random issues are not picked up.

## Build and run Symphony

```bash
cd /Users/canersevince/gameproject
mkdir -p .symphony/bin
go build -o .symphony/bin/symphony ./cmd/symphony
```

Load the env values, then start the orchestrator against this repo's workflow:

```bash
cd /Users/canersevince/gameproject
set -a
source .env
set +a

.symphony/bin/symphony \
  --i-understand-that-this-will-be-running-without-the-usual-guardrails \
  --logs-root /Users/canersevince/gameproject/.symphony/logs \
  --port 4000 \
  /Users/canersevince/gameproject/WORKFLOW.md
```

The dashboard/API will be available on:

```text
http://localhost:4000
```

## First smoke test

1. Create a small Linear issue in the configured project.
2. Add the `symphony` label.
3. Put it in `Todo`.
4. Start Symphony.
5. Confirm a workspace appears under `.symphony/workspaces`.
6. Confirm Symphony logs show the issue being dispatched.

## Notes for later

- When the actual game stack is chosen, update `hooks.after_create` in `WORKFLOW.md` to install that stack's dependencies.
- Keep `agent.max_concurrent_agents` low until CI, tests, and GitHub PR flow are reliable.
- If package installs need network access inside Codex turns, keep `codex.turn_sandbox_policy.networkAccess: true`.
- SSH worker pools and the rich LiveView dashboard from the Elixir prototype are not implemented in the Go port yet. The Go port does implement the repo workflow contract, Linear polling, local workspaces, hooks, Codex app-server JSONL protocol, retries, and JSON status API.

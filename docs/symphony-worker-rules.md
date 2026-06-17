# Symphony Worker Rules

Date: 2026-06-17

This document is for Symphony-managed worker tasks only.

## Hard Boundaries

- Do not read or follow `AGENTS.md`.
- Do not read or follow `docs/symphony-operating-model.md`.
- Do not spawn subagents.
- Do not create, edit, dispatch, or manage Symphony tasks.
- Do not manage the Symphony queue.
- Do not commit.

`AGENTS.md` and `docs/symphony-operating-model.md` are for the main Codex
project-manager session. A Symphony worker is only responsible for its assigned
task in its current workspace.

## Required Reading

Before changing files:

1. Read the assigned task title and description.
2. Read `docs/roadmap/00-index.md`.
3. Read the matching roadmap phase file under `docs/roadmap/`.
4. Read the relevant module spec under `docs/plans/modules/`.
5. Read any additional docs explicitly named by the task.

For gameplay, economy, world, client, infrastructure, or observability work,
respect phase dependencies unless the task explicitly says otherwise.

## Work Rules

- Keep the implementation scoped to the assigned task.
- Prefer the smallest vertical slice that satisfies the task.
- Do not expand into adjacent roadmap items.
- Update roadmap checkboxes only for work that was implemented and verified.
- Leave unfinished TODOs unchecked.
- Do not touch unrelated files.
- Do not include secrets, local logs, generated dependency folders, or
  `.symphony` workspaces in the result.

## Validation

Run the narrowest useful tests while developing. Before final report, run the
validation requested by the task. If no task-specific validation is named, run:

```bash
go test ./...
git diff --check
```

If validation cannot run, report the exact blocker.

## Final Report

Finish with a concise report that names:

- changed files
- roadmap phase touched
- validation commands and outcomes
- any blockers or follow-up work


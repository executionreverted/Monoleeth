# Symphony Operating Model

Date: 2026-06-17

This document defines how Codex should manage Symphony in this repo.

The goal is not to use Symphony as a single-task runner. The goal is to act as
the conductor: split work, keep compatible agents busy, review their output,
merge minimal commits, and keep roadmap state current.

## Default Role

Codex is the Symphony project manager.

When managing project work through Symphony:

- Do not open one task and idle unless the work is truly indivisible.
- Prefer a wave of 2-5 independent tasks.
- Keep each task scoped to one module, one roadmap slice, and one small commit.
- Keep tasks conflict-free by assigning non-overlapping files or clear ownership.
- Use long `/wait` calls instead of frequent stream polling.
- Review every completed workspace diff before applying it.
- Apply, verify, and commit one task at a time.
- Keep the roadmap and docs updated in the same commit as the code.

## Wave Planning

Before creating Symphony tasks:

1. Read `AGENTS.md`.
2. Read `docs/roadmap/00-index.md`.
3. Read the active phase file under `docs/roadmap/`.
4. Read the referenced module specs under `docs/plans/modules/`.
5. Identify the next unchecked TODOs.
6. Split them into independent task cards.

A good wave has:

- one task per service method, primitive, or narrowly testable behavior
- explicit files or package ownership
- exact docs to read
- exact tests to add
- exact validation commands
- "Do not commit" in every task description

Avoid parallel tasks that touch the same production file unless the change is
purely additive and easy to merge. If two tasks both need the same core file,
run them sequentially or split a prerequisite task first.

## Conflict Rules

Safe parallel examples:

- `CreditWallet` and `RemoveItem` if wallet and inventory service files are separate.
- UI read-only patch review and economy service work.
- Model validation tests and documentation audit.

Unsafe parallel examples:

- Two tasks editing `InventoryService` internals at the same time.
- A reservation task and a generic move/remove task if both alter escrow rules.
- A roadmap audit task running while implementation tasks update the same phase file.

When unsure, prefer smaller waves over merge-heavy waves.

## Agent Lifecycle

For each task:

1. Create the task through `/api/v1/tasks`.
2. Let auto-run dispatch it, or call `/run` if needed.
3. Wait with `/api/v1/tasks/{id}/wait?timeout_ms=...`.
4. Do not burn context by repeatedly reading streams.
5. If blocked, report the exact blocker and stop that branch.
6. If done, fetch `/api/v1/tasks/{id}/workspace-diff`.
7. Review the diff packet before applying anything.
8. Apply the patch to the main repo only after review.
9. Run the narrowest relevant tests.
10. Run `go test ./...` and `git diff --check`.
11. Commit with one clear reason.

If multiple tasks finish around the same time, process them in dependency order:

1. shared primitives
2. service methods
3. integration behavior
4. roadmap or audit updates

## Commit Discipline

Commits should stay minimal:

- `symphony:` for orchestration tooling
- `game:` for gameplay/economy implementation
- `docs:` for documentation-only changes
- `test:` for test-only changes

Do not mix unrelated Symphony tooling and gameplay changes in the same commit
unless one is required to execute or verify the other.

Every applied Symphony task should end with:

- clean `git status --short`
- passing `go test ./...`
- passing `git diff --check`
- roadmap checkboxes updated only for verified work

## Tracking Memory

At the start of each wave, note:

- active roadmap phase
- completed task identifiers
- pending unchecked TODOs
- known blocked or deferred work
- commits created in the wave

At the end of each wave, update the relevant roadmap phase resume notes if the
next step would not be obvious from the checklist.

The operating style is simple: keep the agents busy, keep the commits small,
keep the docs truthful, and keep the user out of mechanical queue management.

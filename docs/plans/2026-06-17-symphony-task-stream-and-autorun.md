# Symphony Task Stream And Autorun Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add per-task run streams and queue controls to the local Go Symphony UI.

**Architecture:** The orchestrator remains the source of truth for running state. It will record bounded per-task runtime events in memory, persist them as JSONL under the local tracker root, expose them through HTTP APIs, and allow auto-dispatch to be paused or resumed without breaking manual task runs.

**Tech Stack:** Go, `net/http`, embedded HTML/CSS/JS, JSONL task logs, existing local tracker files.

---

### Task 1: Event Log Model And Persistence

**Files:**
- Modify: `internal/symphony/domain.go`
- Modify: `internal/symphony/orchestrator.go`
- Test: `internal/symphony/orchestrator_test.go`

**Steps:**
1. Add a `RunLogEntry` model with timestamp, issue id, identifier, event, attempt, turn count, summary, and raw details.
2. Add `runLogs map[string][]RunLogEntry` to `Orchestrator`.
3. Add `recordRunEvent`, `TaskRunLog`, and `appendRunLogFile` helpers.
4. Record lifecycle events for dispatch, Codex events, retry scheduling, blocked, completed, failed, and stopped.
5. Test that Codex events are retained and bounded.

### Task 2: Queue Control

**Files:**
- Modify: `internal/symphony/domain.go`
- Modify: `internal/symphony/orchestrator.go`
- Test: `internal/symphony/orchestrator_test.go`

**Steps:**
1. Add `autoRunPaused` to the snapshot.
2. Add `SetAutoRunPaused` and `AutoRunPaused`.
3. Make `dispatchCandidates` skip automatic dispatch while paused.
4. Keep manual `RunTask` available while paused.
5. Test that paused auto-run does not dispatch candidates.

### Task 3: HTTP APIs

**Files:**
- Modify: `internal/symphony/http.go`
- Test: `internal/symphony/http_test.go`

**Steps:**
1. Add `GET /api/v1/tasks/{id}/stream`.
2. Add `PATCH /api/v1/autorun`.
3. Return stream entries as `{"events":[]}`.
4. Return autorun state as `{"auto_run":true|false}`.
5. Test both APIs.

### Task 4: UI

**Files:**
- Modify: `internal/symphony/http.go`
- Modify: `internal/symphony/dashboard.css`
- Test: `internal/symphony/http_test.go`

**Steps:**
1. Add auto-run pause/resume controls to `/tasks`.
2. Add a task stream panel that opens from a task card.
3. Poll the selected task stream with the existing refresh loop.
4. Style the stream panel with the current Twenty-inspired UI language.
5. Test that `/tasks` contains the new controls.

### Task 5: Verification

**Commands:**
- `go test ./...`
- `go test -race ./internal/symphony`
- `git diff --check`

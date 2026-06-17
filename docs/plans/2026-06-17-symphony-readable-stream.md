# Symphony Readable Stream Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Render task streams as readable agent messages and command summaries instead of raw app-server JSON.

**Architecture:** Keep raw `events` in `/api/v1/tasks/{id}/stream` for debugging and add a derived `display_events` array produced by Go. The presenter folds `item/agentMessage/delta` chunks by item id, extracts command started/completed events, and keeps Symphony lifecycle events readable.

**Tech Stack:** Go stream presenter, existing HTTP endpoint, vanilla JavaScript UI.

---

### Task 1: Display Event Presenter

**Files:**
- Create: `internal/symphony/run_stream_presenter.go`
- Test: `internal/symphony/run_stream_presenter_test.go`

**Steps:**
1. Add `DisplayRunEvent` with timestamp, kind, title, body, identifier, attempt, and turn count.
2. Fold `item/agentMessage/delta` events into one assistant message.
3. Extract command text and aggregated output from `item/started` and `item/completed`.
4. Preserve `task_completed`, `turn_error`, and retry events as lifecycle display entries.

### Task 2: API And UI

**Files:**
- Modify: `internal/symphony/http.go`
- Test: `internal/symphony/http_test.go`

**Steps:**
1. Return both `events` and `display_events` from `/api/v1/tasks/{id}/stream`.
2. Update run stream and agent island renderers to prefer `display_events`.
3. Keep fallback to raw events.

### Task 3: Verification

**Commands:**
- `go test ./...`
- `go test -race ./internal/symphony`
- `git diff --check`

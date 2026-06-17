# Symphony Agent Stream Island Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show the currently active agent/task stream in a fixed bottom-center island on the local tasks page.

**Architecture:** Reuse the existing `/api/v1/raw-state` running-session data and `/api/v1/tasks/{id}/stream` event API. The island is an embedded HTML/CSS/JS component in the existing Go tasks page, auto-following the latest active task while allowing manual task selection.

**Tech Stack:** Go embedded HTML, vanilla JavaScript polling, responsive CSS.

---

### Task 1: Island Markup And Styles

**Files:**
- Modify: `internal/symphony/http.go`
- Test: `internal/symphony/http_test.go`

**Steps:**
1. Add fixed bottom-center island markup to `tasksHTML`.
2. Add mobile-first CSS for compact and expanded states.
3. Add test assertions that `/tasks` renders the island controls.

### Task 2: Island Data Flow

**Files:**
- Modify: `internal/symphony/http.go`

**Steps:**
1. Select the most recently active running task from `/api/v1/raw-state`.
2. Fetch that task's stream with `/api/v1/tasks/{id}/stream`.
3. Render task chips when multiple agents are running.
4. Allow manual chip selection and return to follow-latest mode.

### Task 3: Verification

**Commands:**
- `go test ./...`
- `go test -race ./internal/symphony`
- `git diff --check`
- Playwright desktop/mobile screenshots for `/tasks`.

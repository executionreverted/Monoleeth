# Quest Board Replacing Galaxy Menu Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the misleading Galaxy menu with a real Quest Board that lists
offers, active quests, claimable quests, completed quests, objectives, rewards,
and quest actions.

**Architecture:** Quest board, accepted quests, progress, completion, rewards,
and rerolls are server-owned. Client selection/filtering is local.

**Tech Stack:** Go quest runtime handlers, TypeScript HUD/reducer, CSS board
layout, browser smoke.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/ui-implementation/09-quests-admin-observability-release.md
output/mockups/final-mockup.png
```

## Current Behavior

- The nav label is `Galaxy`, but the window title/body is Quest Board.
- `questBlock` focuses one quest or offer.
- Existing contracts include `quest.board`, `quest.accept`,
  `quest.progress`, `quest.claim_reward`, and `quest.reroll`.

## Target UX

- Left nav label is `Quests`.
- Quest Board window has:
  - tabs or segmented control: Offers, Active, Claimable, Completed
  - list of quests/offers
  - selected quest detail pane
  - objective progress list
  - reward list
  - actions: Accept, Claim, Reroll
- Counts are real server counts.
- If no quests/offers exist, show empty state rather than one fake quest.

## Implementation Tasks

1. Rename nav label.
   - In `baseWindowDefinitions`, change `Galaxy` to `Quests`.
   - Keep icon only if it still fits the mockup; otherwise use a quest-like
     existing HUD SVG or add a vetted asset.

2. Build board layout.
   - Replace single `questBlock` focus with list/detail.
   - Keep selected quest/offer local UI state.
   - Default selection priority:
     - first claimable quest
     - first active quest
     - first offer
     - empty state

3. Wire actions.
   - Accept offer calls `quest.accept`.
   - Claim calls `quest.claim_reward`.
   - Reroll calls `quest.reroll` only when wallet has enough server-reported
     currency.
   - Progress is display-only from server events/snapshots.

4. Tests.
   - Client render test for multiple offers/active quests.
   - Browser smoke accepts an offer, refreshes progress, claims if claimable,
     and rerolls when seeded wallet permits.
   - Go tests only if handler behavior changes.

## Files Likely Touched

```text
client/src/ui/hud.ts
client/src/styles.css
client/src/state/types.ts
client/src/state/reducer.ts
client/tests/browser-smoke.mjs
internal/game/server/quest_admin_observability_handlers.go
internal/game/server/server_test.go
```

## Acceptance Checklist

- [x] Navigation says `Quests`, not `Galaxy`.
- [x] Quest Board shows multiple offers/active/claimable/completed entries.
- [x] Selecting a quest changes detail pane.
- [x] Objective progress and rewards are visible.
- [x] Accept/Claim/Reroll are wired to real contracts.
- [x] Empty board states are honest and not fake.
- [x] Browser smoke covers quest list selection and at least one action.

## Implementation Notes

- `client/src/ui/hud.ts` now renders the quest window as a category board with
  Offers, Active, Claimable, and Completed sections, local row selection,
  objective progress, rewards, and server-contract actions.
- `client/tests/browser-smoke.mjs` opens the real authenticated Quest Board,
  verifies the nav label, changes selected detail, accepts a real offer, and
  captures `output/screenshots/ui-patch-3/quests-{viewport}.png`.
- The movement smoke assertion now samples in-flight interpolation before
  full-page screenshot capture; the previous order could consume the remaining
  ETA and produce a false jump failure.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/quests ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```

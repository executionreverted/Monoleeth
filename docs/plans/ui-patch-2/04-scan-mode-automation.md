# Scan Mode Automation And Visual Indicator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Turn scan into a toggled mode that periodically sends real `scan.pulse` intents and visibly shows scanning around the ship.

**Status:** Completed in Phase 04.

**Architecture:** Scan mode is client-local control state. The server still
owns whether each pulse starts, resolves, consumes energy, discovers a planet,
grants XP, or rejects due to cooldown/rate-limit/movement/energy. The client
loop backs off based on server response timing and safe public errors.

**Tech Stack:** TypeScript timers, existing realtime protocol, PixiJS renderer,
CSS action button animation, browser smoke.

---

## Files

- Modify: `client/src/state/types.ts`
- Modify: `client/src/state/reducer.ts`
- Modify: `client/src/state/reducer.test.ts`
- Modify: `client/src/app/client-app.ts`
- Modify: `client/src/render/world-view.ts`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/src/ui/hud.ts`
- Modify: `client/src/styles.css`
- Modify: `client/tests/browser-smoke.mjs`
- Reference: `docs/plans/modules/14-world-aoi-fog-security.md`

## Steps

1. Add client UI state:
   - `scanMode.enabled`
   - `scanMode.nextPulseAt`
   - `scanMode.lastRejectedAt`
   - `scanMode.lastError`
   This is presentation/control state, not scan truth.
2. Add actions:
   - `scanModeToggled`
   - `scanPulseScheduled`
   - `scanPulseRejected`
   - `scanPulseAccepted`
3. In `ClientApp`, add a scan timer loop:
   - if scan mode disabled, no timer
   - if disconnected or ship disabled, show disabled and pause
   - if pending `scan.pulse`, wait
   - when due, send `scan.pulse`
4. On server messages:
   - `scan.pulse_started` sets next expected resolution from `resolve_after`
   - `scan.pulse_resolved` / `scan.planet_discovered` schedules the next pulse
   - cooldown/rate-limit/energy/moving errors back off and keep a visible
     reason
5. Add renderer scan state:
   - `WorldViewState.scanMode`
   - draw 2-3 expanding rings around self while active
   - rings are visual only and never affect visibility
6. Update action button:
   - inactive label: `Scan`
   - active label: `Scanning`
   - animated border/pulse, cooldown/progress detail
   - click toggles mode
7. Tests:
   - reducer toggles mode without altering gameplay truth
   - scan loop sends repeated `scan.pulse` only after server timing/backoff
   - scan mode stops when logout/auth expiry clears state
   - browser smoke verifies action button active class and ring pixels exist
8. Screenshot:
   - save scan mode active under `output/screenshots/ui-patch-2/04/`

## Acceptance

- [x] Scan button toggles mode.
- [x] Client automatically sends real `scan.pulse` when allowed by timing.
- [x] Server rejection never mutates fake scan results.
- [x] Active scan has ship-centered visual waves and animated action button.
- [x] Mode clears on logout/auth expiry and pauses while disconnected.

## Implementation Notes

- `scanMode` is client-local control state only; scan results, planet discovery,
  XP, cooldown, energy, and rejection truth still come from server responses and
  events.
- `ClientApp` pauses the loop while disconnected/disabled, waits on pending
  `scan.pulse`, waits while `lastScan.status === "started"`, and backs off on
  server-safe errors.
- Pixi scan rings render around the interpolated self ship position and expose
  `worldView.scanWaves` only for smoke/debug verification.
- Browser smoke captures active scan screenshots under
  `output/screenshots/ui-patch-2/04/` and verifies fixture auto-loop command
  count plus real-server ring/button state.

## Commit

```bash
git add client/src/state client/src/app client/src/render client/src/ui client/src/styles.css client/tests/browser-smoke.mjs output/screenshots/ui-patch-2/04
git commit -m "client: add scanner mode"
```

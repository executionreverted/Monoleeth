# Phase 08 Scanner Gates Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Finish the remaining Phase 08 scanner discovery TODOs for capacitor/energy validation and stationary scan gating without exposing hidden procedural truth or trusting client movement/energy claims.

**Architecture:** Extend `ScannerService` with two server-owned prerequisite gates before cooldown and pulse creation: one provider validates current scan energy/capacitor availability, and the existing zone position provider carries authoritative movement state so scans require a stationary ship. The Phase 08 domain service remains in-memory and provider-based; concrete durable energy consumption or world-worker slow-state application stays a runtime follow-up unless an existing authoritative store already exposes it.

**Tech Stack:** Go, `internal/game/discovery`, `internal/game/world`, `internal/game/stats`, roadmap/module Markdown docs, Go tests.

---

### Task 1: Add Scanner Energy And Movement Contracts

**Files:**
- Modify: `internal/game/discovery/scanner_types.go`
- Modify: `internal/game/discovery/scanner_helpers.go`
- Test: `internal/game/discovery/scanner_test.go`

**Step 1: Write failing tests for scanner prerequisites**

Add tests near the existing start-pulse tests:

```go
func TestStartScanPulseEnergyUnavailableFailsBeforeCooldownAndMutation(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	cooldowns := &recordingScannerCooldownProvider{accepted: true}
	energy := &recordingScannerEnergyProvider{accepted: false}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:      seed,
		store:     NewInMemoryStore(),
		position:  candidate.Position(),
		snapshot:  scannerSnapshot(candidate, 10_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:  true,
		cooldowns: cooldowns,
		energy:    energy,
		xp:        &recordingScanXPProvider{},
	})

	_, err := service.StartScanPulse(scannerStartInput("pulse_no_energy"))
	if !errors.Is(err, ErrScannerEnergyUnavailable) {
		t.Fatalf("StartScanPulse() error = %v, want ErrScannerEnergyUnavailable", err)
	}
	if cooldowns.calls != 0 || len(service.Events()) != 0 {
		t.Fatalf("scan mutated after energy rejection")
	}
}

func TestStartScanPulseMovingShipFailsBeforeCooldownAndMutation(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	cooldowns := &recordingScannerCooldownProvider{accepted: true}
	position := ScannerPosition{
		WorldID: scannerTestWorldID,
		ZoneID: scannerTestZoneID,
		Position: candidate.Position(),
		Movement: world.MovementState{Moving: true, Target: world.Vec2{X: 1, Y: 1}},
	}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:            seed,
		store:           NewInMemoryStore(),
		scannerPosition: position,
		snapshot:        scannerSnapshot(candidate, 10_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:        true,
		cooldowns:       cooldowns,
		xp:              &recordingScanXPProvider{},
	})

	_, err := service.StartScanPulse(scannerStartInput("pulse_moving"))
	if !errors.Is(err, ErrScanMovementRestricted) {
		t.Fatalf("StartScanPulse() error = %v, want ErrScanMovementRestricted", err)
	}
	if cooldowns.calls != 0 || len(service.Events()) != 0 {
		t.Fatalf("scan mutated after movement rejection")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/game/discovery -run 'TestStartScanPulse(EnergyUnavailable|MovingShip)' -count=1
```

Expected: fail because the new errors/provider fields do not exist.

**Step 3: Add contract types**

In `scanner_types.go`:

```go
var (
	ErrScannerEnergyUnavailable = errors.New("scanner energy unavailable")
	ErrScanMovementRestricted   = errors.New("scan movement restricted")
)

type ScannerEnergyInput struct {
	PlayerID       foundation.PlayerID
	ShipID         foundation.ShipID
	WorldID        foundation.WorldID
	ZoneID         foundation.ZoneID
	PulseReference ScanPulseReference
	CheckedAt      time.Time
	Stats          stats.EffectiveStats
}

type ScannerEnergyResult struct {
	Accepted bool
}

type ScannerEnergyProvider interface {
	CheckScanEnergy(input ScannerEnergyInput) (ScannerEnergyResult, error)
}
```

Add `Movement world.MovementState` to `ScannerPosition`, add `Energy ScannerEnergyProvider` to `ScannerServiceConfig`, and add `energy ScannerEnergyProvider` to `ScannerService`.

**Step 4: Add validation helpers**

In `scanner_helpers.go`, require non-nil `config.Energy`, validate `ScannerEnergyInput`, and add:

```go
func (position ScannerPosition) ValidateStationaryForScan() error {
	if err := position.Movement.Validate(); err != nil {
		return err
	}
	if position.Movement.Moving {
		return ErrScanMovementRestricted
	}
	return nil
}
```

Keep the provider check non-mutating at the discovery domain boundary; durable energy spend can be a runtime follow-up.

**Step 5: Wire service start order**

In `scanner.go`, after position/stats validation and before `StartScanCooldown`:

```go
if err := position.ValidateStationaryForScan(); err != nil {
	return StartScanPulseResult{}, err
}
energy, err := service.energy.CheckScanEnergy(ScannerEnergyInput{...})
if err != nil {
	return StartScanPulseResult{}, err
}
if !energy.Accepted {
	return StartScanPulseResult{}, ErrScannerEnergyUnavailable
}
```

Expected order: no cooldown, pulse, event, planet, intel, or XP mutation happens when energy or movement rejects.

**Step 6: Run focused tests**

Run:

```bash
go test ./internal/game/discovery -run 'TestStartScanPulse(EnergyUnavailable|MovingShip|WithoutModule|CooldownBlocksSpam|DuplicateReference|ZeroScanInterval)' -count=1
```

Expected: pass.

### Task 2: Update Scanner Test Fixtures

**Files:**
- Modify: `internal/game/discovery/scanner_test.go`

**Step 1: Update fixture options**

Add fields:

```go
scannerPosition ScannerPosition
energy          ScannerEnergyProvider
energyOK        bool
```

Default stationary `ScannerPosition` from `position` when `scannerPosition` is zero.

**Step 2: Add fixed and recording energy providers**

```go
type fixedScannerEnergyProvider struct{ accepted bool }
func (provider fixedScannerEnergyProvider) CheckScanEnergy(ScannerEnergyInput) (ScannerEnergyResult, error) {
	return ScannerEnergyResult{Accepted: provider.accepted}, nil
}

type recordingScannerEnergyProvider struct {
	accepted bool
	calls int
	inputs []ScannerEnergyInput
}
func (provider *recordingScannerEnergyProvider) CheckScanEnergy(input ScannerEnergyInput) (ScannerEnergyResult, error) {
	provider.calls++
	provider.inputs = append(provider.inputs, input)
	return ScannerEnergyResult{Accepted: provider.accepted}, nil
}
```

Default `energyOK` to true for existing tests.

**Step 3: Verify full discovery package**

Run:

```bash
go test ./internal/game/discovery -count=1
```

Expected: pass.

### Task 3: Update Docs And Roadmap

**Files:**
- Modify: `docs/roadmap/08-world-discovery-planets-intel.md`
- Modify: `docs/plans/modules/14-world-aoi-fog-security.md`
- Modify: `docs/2026-06-17-world-system-design.md`
- Optional: `docs/todo.md` if durable energy spend or slow-state application remains deferred.

**Step 1: Mark completed Phase 08 scanner TODOs**

Check off:

```markdown
- [x] Validate capacitor/energy. Verified 2026-06-18 by ...
- [x] Apply slow or stationary scan state. Verified 2026-06-18 by ...
```

Add test checklist entries for energy and movement gates if absent.

**Step 2: Document the boundary**

Document that Phase 08 validates scanner energy through a server-owned provider and requires authoritative stationary movement state before cooldown/pulse creation. If actual energy consumption or slow-state lease remains outside the in-memory domain MVP, add or preserve a `docs/todo.md` follow-up with enough context.

**Step 3: Run docs diff check**

Run:

```bash
git diff --check
```

Expected: no output.

### Task 4: Verify, Review, And Commit

**Files:**
- All files touched above.

**Step 1: Run required verification**

Run:

```bash
go test ./...
git diff --check
```

Expected: pass.

**Step 2: Run Symphony read-only review**

Create a local read-only Symphony review task for the live uncommitted diff. The prompt must require `cd /Users/canersevince/gameproject && pwd && git status --short`, forbid edits/commits/subagents, require `docs/symphony-worker-rules.md`, and focus on scanner energy/movement ordering, duplicate safety, client-trust boundaries, and docs.

**Step 3: Fix review findings or record follow-ups**

Fix concrete bugs before commit. Record deferred durable-runtime risks in `docs/todo.md`.

**Step 4: Stage and inspect**

Run:

```bash
git status --short
git diff --stat
git add <scanner/docs files>
git diff --cached --stat
git diff --cached --check
```

**Step 5: Commit**

Run:

```bash
git commit -m "game: validate scanner energy and movement"
```

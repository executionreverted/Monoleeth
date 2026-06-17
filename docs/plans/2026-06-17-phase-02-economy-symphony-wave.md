# Phase 02 Economy Symphony Wave Plan

Date: 2026-06-17

This plan is for the main Codex project-manager session. Do not paste this full
file into worker prompts. Worker prompts must point workers to
`docs/symphony-worker-rules.md` and must explicitly say "do not commit".

## Goal

Finish the next safe Phase 02 economy slices with multiple conflict-aware
Symphony workers, then review, apply, verify, and commit each completed patch
one at a time.

## Current Phase

- Roadmap: `docs/roadmap/02-economy-ledger-inventory.md`
- Module spec: `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- Current completed service slices: `AddItem`, `MoveItem`, `RemoveItem`
- Current missing service slices: reservations, wallet primitives, cargo
  capacity, item/premium policy helpers, idempotent ledger coverage, events

## Manager Rules

- Keep `AGENTS.md` and `docs/symphony-operating-model.md` out of worker prompts.
- Every worker prompt must include `docs/symphony-worker-rules.md`.
- Every worker prompt must include "Do not spawn subagents" and "Do not commit".
- Create 4 tasks in the first wave; the Symphony queue may run fewer at once if
  `WORKFLOW.md` concurrency remains lower.
- Use long `/wait` calls or one shell command that waits for all task IDs in
  parallel; do not repeatedly poll task streams.
- Fetch `/api/v1/tasks/{id}/workspace-diff` after completion.
- Apply one patch at a time in the merge order below.
- Run narrow tests for each applied patch, then `go test ./...` and
  `git diff --check` before each commit.

## Preflight

1. Confirm `git status --short` is clean.
2. Confirm Symphony is reachable:

```bash
curl -sS http://127.0.0.1:4000/api/v1/state
```

3. If we want 4 active workers, update `WORKFLOW.md` to:

```yaml
agent:
  max_concurrent_agents: 4
  max_turns: 12
```

Then run:

```bash
curl -sS -X POST http://127.0.0.1:4000/api/v1/refresh
```

If the machine feels noisy, keep concurrency at 2 and still create the four-task
wave; Symphony will drain it safely.

## Wave 1: Independent Phase 02 Slices

### Task W1-A: WalletService Credit/Debit/Transfer

**Purpose:** Implement currency mutations with idempotency and currency ledger
entries.

**Files:**
- Create: `internal/game/economy/wallet_service.go`
- Create: `internal/game/economy/wallet_service_test.go`
- Modify: `docs/roadmap/02-economy-ledger-inventory.md`

**Worker prompt scope:**
- Read `docs/symphony-worker-rules.md`.
- Read `docs/roadmap/02-economy-ledger-inventory.md`.
- Read `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`.
- Implement `CreditWallet`, `DebitWallet`, and `TransferCurrency`.
- Require positive amount, valid player, valid currency, non-empty reason, and
  non-empty reference ID.
- Idempotency key shape: player + operation + reference ID.
- Credit duplicate must not credit or ledger twice.
- Debit duplicate must not debit or ledger twice.
- Transfer duplicate must not transfer or ledger twice.
- Debit must reject insufficient funds without mutation or ledger entry.
- Transfer must write debit and credit currency ledger entries.
- Do not touch inventory files unless a test helper truly requires it.
- Do not commit.

**Validation:**

```bash
go test ./internal/game/economy -run 'Wallet|Credit|Debit|Transfer|CurrencyLedger'
git diff --check
```

**Roadmap checkboxes if verified:**
- `Implement CreditWallet`
- `Implement DebitWallet`
- `Implement TransferCurrency`
- Duplicate reference ID does not duplicate currency grants
- Debit fails when balance is insufficient
- Debit writes a matching ledger entry
- Credit writes a matching ledger entry
- Transfer writes debit and credit ledger entries

### Task W1-B: CargoService Capacity Wrapper

**Purpose:** Add server-side cargo capacity validation without changing the
generic `AddItem` primitive.

**Files:**
- Create: `internal/game/economy/cargo_service.go`
- Create: `internal/game/economy/cargo_service_test.go`
- Modify: `docs/roadmap/02-economy-ledger-inventory.md`

**Worker prompt scope:**
- Read `docs/symphony-worker-rules.md`.
- Read `docs/roadmap/02-economy-ledger-inventory.md`.
- Read `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`.
- Implement a small `CargoService` that wraps `InventoryService`.
- Input must include player ID, active cargo location, item definition,
  quantity, server-provided cargo capacity units, reason, and reference ID.
- Compute used cargo from inventory service state inside the service lock.
- Compute incoming weight from item definition weight times quantity.
- Reject over-capacity adds without mutation or ledger entry.
- Use `InventoryService.AddItem` for the actual mutation when capacity passes.
- Include a concurrent pickup test that only allows capacity-safe results.
- Do not commit.

**Validation:**

```bash
go test ./internal/game/economy -run 'Cargo|Capacity'
git diff --check
```

**Roadmap checkboxes if verified:**
- `Implement cargo capacity validation using server-side stat input`
- Cargo capacity blocks over-capacity add
- Concurrent cargo pickup simulation only allows capacity-safe result
- Cargo capacity race blocked

### Task W1-C: Item Trade And Premium Eligibility Helpers

**Purpose:** Add pure validation helpers used later by loadout, market, auction,
and premium flows.

**Files:**
- Create: `internal/game/economy/item_policy.go`
- Create: `internal/game/economy/item_policy_test.go`
- Create: `internal/game/economy/premium_policy.go`
- Create: `internal/game/economy/premium_policy_test.go`
- Modify: `docs/roadmap/02-economy-ledger-inventory.md`

**Worker prompt scope:**
- Read `docs/symphony-worker-rules.md`.
- Read `docs/roadmap/02-economy-ledger-inventory.md`.
- Read `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`.
- Implement helpers for trade flag checks:
  - market listing requires `TradeFlagMarketTradeable` or `TradeFlagTradeable`
  - auction listing requires `TradeFlagAuctionTradeable` or `TradeFlagTradeable`
  - droppable requires `TradeFlagDroppable`
  - destroyable requires `TradeFlagDestroyable`
- Implement helper that blocks equipped, escrow, reserved, and system locations
  for player trade/equip style flows.
- Implement premium eligibility helper:
  - paid premium allowed for paid-only use
  - earned premium rejected for paid-only use
  - market-acquired premium handled explicitly
- Keep this pure: no service mutation, no ledger mutation.
- Do not commit.

**Validation:**

```bash
go test ./internal/game/economy -run 'Policy|Trade|Premium|Eligibility'
git diff --check
```

**Roadmap checkboxes if verified:**
- `Implement item trade flag validation helpers`
- `Implement premium bucket eligibility helper`
- Craft reserved item cannot be listed or equipped
- Premium earned bucket cannot be used where paid premium is required
- Escrow bypass blocked
- Premium laundering blocked by bucket split

### Task W1-D: ReserveItems Only

**Purpose:** Add the first reservation service slice by moving player-owned
items into the reservation location. Do not implement release or commit yet.

**Files:**
- Create: `internal/game/economy/reservation_service.go`
- Create: `internal/game/economy/reservation_service_test.go`
- Modify: `internal/game/economy/reservations.go` only if new input structs are
  needed.
- Modify: `docs/roadmap/02-economy-ledger-inventory.md`

**Worker prompt scope:**
- Read `docs/symphony-worker-rules.md`.
- Read `docs/roadmap/02-economy-ledger-inventory.md`.
- Read `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`.
- Implement `ReserveItems` only.
- Move item requirements from player-owned source locations to the kind's
  reserved location:
  - craft -> `crafting_reserved`
  - market -> `market_escrow`
  - auction -> `auction_escrow`
- Use existing `InventoryService.MoveItem` where possible.
- Do not implement `ReleaseReservation` or `CommitReservation`.
- Do not add generic movement from reserved/escrow sources in this task.
- Duplicate reference must not reserve or ledger twice.
- Insufficient quantity must not create a reservation or ledger-only state.
- Do not commit.

**Validation:**

```bash
go test ./internal/game/economy -run 'Reserve|Reservation|MoveItem'
git diff --check
```

**Roadmap checkboxes if verified:**
- `Implement ReserveItems`
- Craft reserved item cannot be listed or equipped, only if Task W1-C helper is
  already merged and the reservation test uses it
- Escrow bypass blocked, only for the newly covered reserve path

## Wave 1 Merge Order

1. W1-C Item/Premium helpers
2. W1-A WalletService
3. W1-B CargoService
4. W1-D ReserveItems

Reasoning:

- Policy helpers are pure and unblock later validation tests.
- Wallet does not depend on inventory.
- Cargo wraps `InventoryService.AddItem` but should not alter inventory
  internals.
- ReserveItems depends most on inventory movement semantics and should be
  reviewed after helpers are available.

## Wave 1 Wait Strategy

After creating task IDs, wait with one shell command rather than manual polling:

```bash
for id in local-XXXX local-YYYY local-ZZZZ local-WWWW; do
  (
    curl -sS "http://127.0.0.1:4000/api/v1/tasks/${id}/wait?timeout_ms=3600000" \
      > "/tmp/symphony-${id}-wait.json"
  ) &
done
wait
```

Then fetch completed patch packets:

```bash
curl -sS http://127.0.0.1:4000/api/v1/tasks/local-XXXX/workspace-diff
```

Apply, verify, and commit one patch at a time.

## Wave 2: Sequential Follow-Up

Start Wave 2 only after Wave 1 is merged and `go test ./...` is clean.

### Task W2-A: ReleaseReservation

- Implement reservation release from reserved/escrow locations back to the
  original source.
- This likely needs a system-owned movement path that bypasses generic player
  move source restrictions while preserving ledger entries.
- Keep it separate from commit to reduce risk.

### Task W2-B: CommitReservation

- Implement final reservation consumption.
- For craft-style reservations, consume reserved items into `system_sink`.
- For market/auction style reservations, leave final ownership handoff to later
  market/auction phases unless Phase 02 has a minimal primitive ready.

### Task W2-C: Ledger Reference Uniqueness Audit

- Confirm inventory and wallet idempotency maps cover player + operation +
  reference ID.
- Add tests for duplicate references across operation types.
- Ensure failed mutations do not reserve reference keys.

## Wave 3: Events And Phase Close

Start Wave 3 only after reservation and wallet primitives are stable.

### Task W3-A: Economy Mutation Events

- Add event emission hooks for inventory added, moved, removed, cargo updated,
  wallet credited/debited, and ledger entry created.
- Emit after mutation succeeds.
- Keep event recording in-memory/testable for Phase 02.

### Task W3-B: Phase 02 Audit And Done Criteria

- Re-read every Phase 02 checkbox.
- Check only verified items.
- Add resume notes for any deferred release gates.
- Run:

```bash
go test ./...
git diff --check
```

## Commit Shape

Expected commits:

```text
game: add economy policy helpers
game: implement wallet service primitives
game: add cargo capacity service
game: implement item reservations
game: release item reservations
game: commit item reservations
game: emit economy mutation events
docs: audit phase 02 economy checklist
```

Each commit should include only the code, tests, and roadmap checkboxes for the
verified slice.


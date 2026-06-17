# Phase 01: Foundation Platform

## Status

- State: Not started
- Owner: Backend/game platform
- Depends on: none
- Unlocks: every later gameplay and economy phase

## Goal

Create the minimal Go gameplay foundation so all future modules share consistent IDs, errors, clocks, RNG, catalogs, events, idempotency, test helpers, and package boundaries.

## Why This Comes First

The game has no gameplay code yet. Starting with combat or market code would force every module to invent its own primitives. This phase prevents drift and gives later services a clean place to stand.

## Source Specs

Read before implementation:

- `AGENTS.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `docs/plans/modules/00-index.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`

## Package Direction

Keep gameplay separate from Symphony:

```text
internal/game/
  foundation/
  contracts/
  catalog/
  events/
  testutil/
```

Do not import `internal/symphony` from gameplay packages.

## Architecture Notes

The foundation should stay small and boring:

- typed IDs, not raw strings everywhere
- server-owned clock and RNG interfaces for tests
- common domain errors with stable error codes
- request envelope and event envelope structs
- idempotency key conventions
- catalog version structs
- deterministic test fixtures

Avoid adding PostgreSQL, Redis, NATS, or WebSocket infrastructure here unless a later phase needs it. Start with in-memory repositories and interfaces where useful.

## TODO

- [x] Create `internal/game/foundation` package.
- [x] Add typed identifiers for player, account, world, zone, entity, item, ship, module, quest, planet, route, listing, auction, event, and request.
- [x] Add a small `Clock` interface with real and fake implementations.
- [x] Add an RNG interface with deterministic test implementation.
- [x] Add `Money`, `Quantity`, and positive amount validation helpers.
- [x] Add common domain error type with public `Code`, safe `Message`, and internal detail support.
- [x] Add shared error codes from `15-api-events-errors.md`.
- [x] Add request envelope model with `request_id`, `op`, `payload`, `client_seq`, and version.
- [x] Add response and error envelope model.
- [x] Add event envelope model with `event_id`, `type`, `payload`, `server_time`, and sequence.
- [x] Add idempotency key helper conventions for domain operations.
- [x] Add `catalog` package for versioned static definitions.
- [x] Add catalog version field helpers so recipes, quests, loot tables, and auction lots can remember source versions.
- [ ] Add in-memory event recorder for service tests.
- [ ] Add test helpers for fake clock, fake RNG, and assertion of emitted events.
- [x] Document package boundaries in a short `internal/game/README.md`.

## Tests

- [x] Unit test positive amount validation rejects zero and negative values.
- [x] Unit test error codes serialize without leaking internal details.
- [x] Unit test fake clock can advance deterministically.
- [x] Unit test fake RNG returns deterministic values.
- [x] Unit test event envelopes include event type, ID, sequence, and server time.
- [x] Unit test idempotency key helpers produce stable keys.
- [x] Unit test gameplay packages do not import `internal/symphony`.

## Abuse And Safety Checks

- [x] Ensure client-facing error messages can be generic for hidden/not-found cases.
- [x] Ensure domain error internal details are not part of public response payload by default.
- [x] Ensure amount helpers cannot turn negative input into a positive mutation.
- [x] Ensure request IDs are modeled separately from domain idempotency keys.

## Done Criteria

- [x] Foundation packages compile.
- [x] Tests for foundation helpers pass.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.
- [ ] Later phase files can reference foundation primitives instead of defining their own.

## Resume Notes

If returning to this phase later, start by checking whether `internal/game` exists and whether tests cover IDs, errors, clocks, RNG, envelopes, and idempotency helpers. If those are missing, finish them before starting economy or combat.

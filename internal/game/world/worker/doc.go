// Package worker contains the in-process Phase 04 zone worker harness.
//
// A Worker is the single owner of one zone's live world state. Client-facing
// systems enqueue intent commands into a mailbox; the worker drains those
// commands in arrival order on each fixed tick, applies authoritative movement,
// updates local indexes, and only exposes snapshots of server-owned state.
//
// This package is intentionally not a network gateway. It does not perform
// WebSocket IO, AOI filtering, fog checks, combat, loot, persistence, or
// cross-zone handoff.
package worker

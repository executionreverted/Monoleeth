// Package quests models authoritative quest templates, generated board offers,
// accepted player quest state, objective schemas, progress, and reward payloads
// for the Phase 07 model slice.
//
// This package intentionally does not accept client-authored progress. Progress
// mutates only from server-owned event consumers, and reward claims grant value
// through explicit wallet, inventory, and progression boundaries.
package quests

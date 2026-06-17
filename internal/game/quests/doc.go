// Package quests models authoritative quest templates, generated board offers,
// accepted player quest state, objective schemas, progress, and reward payloads
// for the Phase 07 model slice.
//
// This package intentionally does not grant rewards or accept client progress.
// Later services must mutate progress only from server-owned events and must
// grant value through wallet, inventory, and progression services.
package quests

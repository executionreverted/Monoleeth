// Package death contains Phase 06 death domain models, cargo drop selection,
// and death orchestration primitives.
//
// Combat still owns lethal damage calculation. Inventory, loot, ships, and
// module durability remain separate service boundaries called by DeathService.
package death

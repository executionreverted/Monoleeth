// Package modules contains server-authoritative module catalog and loadout
// validation primitives.
//
// This package owns module definitions, equipped module state, saved loadouts,
// and the validation needed to apply loadouts to an active ship. Stat
// aggregation remains separate; loadout mutations return invalidation signals
// for a later stats cache/event integration.
package modules

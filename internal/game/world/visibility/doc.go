// Package visibility contains server-side world visibility and fog helpers.
//
// The package is intentionally separate from the world root package. World
// workers can pass local viewer/entity state into these helpers before sending
// snapshots or accepting interactions. Radar range must come from a
// server-calculated stats snapshot, not from client payloads.
//
// Fog memory records previously discovered summaries only. It is not live
// visibility and must not be used to authorize interaction with live entities.
package visibility

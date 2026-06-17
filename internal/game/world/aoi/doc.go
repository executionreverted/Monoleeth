// Package aoi builds client-safe area-of-interest snapshots and diffs.
//
// Snapshot inputs may contain server-only visibility state and hidden metadata.
// Outputs intentionally expose only public entity identity, type, position, and
// status flags after visibility filtering has passed.
package aoi

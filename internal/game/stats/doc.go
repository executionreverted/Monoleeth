// Package stats contains server-authoritative effective stat aggregation.
//
// The package intentionally starts with local input structs instead of wiring
// to ship, module, progression, or buff services. Those packages can translate
// their domain state into these simple modifier inputs once their slices exist.
package stats

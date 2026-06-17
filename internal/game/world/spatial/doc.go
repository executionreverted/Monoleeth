// Package spatial provides fixed-cell spatial hashing for world worker AOI
// candidate queries.
//
// It intentionally owns only coordinates, entity membership, and distance
// filtering. Gameplay visibility, fog, radar, hidden flags, and serialization
// permissions belong in higher-level world visibility services.
package spatial

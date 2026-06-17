package discovery

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
)

var (
	// ErrInvalidWorldSeed reports missing server-side world seed material.
	ErrInvalidWorldSeed = errors.New("invalid world seed")
	// ErrWorldSeedSerialization reports an attempted JSON serialization of
	// server-only gameplay seed material.
	ErrWorldSeedSerialization = errors.New("world seed cannot be serialized")
)

// WorldSeedInput carries server-side seed material loaded from config or
// storage. It is an input boundary only and must never be sent to clients.
type WorldSeedInput struct {
	StaticSeed []byte `json:"-"`
	EpochSeed  []byte `json:"-"`
}

// MarshalJSON fails closed so accidental logging or client payload generation
// cannot serialize gameplay seed material.
func (input WorldSeedInput) MarshalJSON() ([]byte, error) {
	return nil, ErrWorldSeedSerialization
}

// WorldSeed stores derived server-only seed keys for procedural world systems.
type WorldSeed struct {
	staticKey [sha256.Size]byte
	epochKey  [sha256.Size]byte
	hasStatic bool
	hasEpoch  bool
}

// NewWorldSeed derives fixed-size seed keys from server-only storage input.
func NewWorldSeed(input WorldSeedInput) (WorldSeed, error) {
	if len(input.StaticSeed) == 0 {
		return WorldSeed{}, ErrInvalidWorldSeed
	}

	seed := WorldSeed{
		staticKey: sha256.Sum256(input.StaticSeed),
		hasStatic: true,
	}
	if len(input.EpochSeed) > 0 {
		seed.epochKey = sha256.Sum256(input.EpochSeed)
		seed.hasEpoch = true
	}
	return seed, nil
}

// MarshalJSON fails closed so accidental client serialization cannot leak or
// imply access to gameplay seed material.
func (seed WorldSeed) MarshalJSON() ([]byte, error) {
	return nil, ErrWorldSeedSerialization
}

// Valid reports whether seed was constructed with static gameplay material.
func (seed WorldSeed) Valid() bool {
	return seed.hasStatic
}

var _ json.Marshaler = WorldSeedInput{}
var _ json.Marshaler = WorldSeed{}

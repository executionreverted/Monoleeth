package discovery

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

var (
	ErrInvalidPlanet             = errors.New("invalid planet")
	ErrInvalidPlanetBiome        = errors.New("invalid planet biome")
	ErrInvalidPlanetType         = errors.New("invalid planet type")
	ErrInvalidPlanetRarity       = errors.New("invalid planet rarity")
	ErrInvalidPlanetLevel        = errors.New("invalid planet level")
	ErrZeroPlanetTimestamp       = errors.New("zero planet timestamp")
	ErrInvalidMaterializationKey = errors.New("invalid planet materialization key")
	ErrUnknownPlanet             = errors.New("unknown planet")
	ErrStalePlanetOwnerChange    = errors.New("stale planet owner change")
	ErrInvalidOwnerChange        = errors.New("invalid planet owner change")
	ErrEmptyOwnerChangeReference = errors.New("empty planet owner change reference")
)

// PlanetMaterializationKey identifies one deterministic procedural candidate.
// It is server-generated and must not be accepted from clients.
type PlanetMaterializationKey string

// PlanetBiome is the high-level world biome classification for a materialized planet.
type PlanetBiome string

// PlanetType names the planet archetype used by later claim and production systems.
type PlanetType string

// PlanetRarity describes the rarity tier generated for a planet.
type PlanetRarity string

const (
	PlanetBiomeOriginBelt PlanetBiome = "origin_belt"
	PlanetBiomeOuterDrift PlanetBiome = "outer_drift"
	PlanetBiomeNebula     PlanetBiome = "nebula"
	PlanetBiomeDeepSpace  PlanetBiome = "deep_space"
	PlanetBiomeDeadZone   PlanetBiome = "dead_zone"

	PlanetTypeBarren      PlanetType = "barren"
	PlanetTypeTerrestrial PlanetType = "terrestrial"
	PlanetTypeIce         PlanetType = "ice"
	PlanetTypeOcean       PlanetType = "ocean"
	PlanetTypeGasGiant    PlanetType = "gas_giant"

	PlanetRarityCommon    PlanetRarity = "common"
	PlanetRarityUncommon  PlanetRarity = "uncommon"
	PlanetRarityRare      PlanetRarity = "rare"
	PlanetRarityEpic      PlanetRarity = "epic"
	PlanetRarityLegendary PlanetRarity = "legendary"
)

// Planet is the persistent overlay record created only after server-side discovery.
type Planet struct {
	ID          foundation.PlanetID
	WorldID     foundation.WorldID
	ZoneID      foundation.ZoneID
	Coordinates world.Vec2

	Biome  PlanetBiome
	Type   PlanetType
	Rarity PlanetRarity
	Level  int

	DiscoveredAt time.Time
	DiscoveredBy foundation.PlayerID

	OwnerPlayerID  foundation.PlayerID
	OwnerChangedAt *time.Time
}

// Validate reports whether planet is a complete persistent discovery record.
func (planet Planet) Validate() error {
	if err := planet.ID.Validate(); err != nil {
		return err
	}
	if err := planet.WorldID.Validate(); err != nil {
		return err
	}
	if err := planet.ZoneID.Validate(); err != nil {
		return err
	}
	if err := planet.Coordinates.Validate(); err != nil {
		return err
	}
	if err := planet.Biome.Validate(); err != nil {
		return err
	}
	if err := planet.Type.Validate(); err != nil {
		return err
	}
	if err := planet.Rarity.Validate(); err != nil {
		return err
	}
	if planet.Level <= 0 {
		return fmt.Errorf("planet level %d: %w", planet.Level, ErrInvalidPlanetLevel)
	}
	if planet.DiscoveredAt.IsZero() {
		return fmt.Errorf("discovered_at: %w", ErrZeroPlanetTimestamp)
	}
	if err := planet.DiscoveredBy.Validate(); err != nil {
		return err
	}
	if !planet.OwnerPlayerID.IsZero() {
		if err := planet.OwnerPlayerID.Validate(); err != nil {
			return err
		}
		if planet.OwnerChangedAt == nil || planet.OwnerChangedAt.IsZero() {
			return fmt.Errorf("owner_changed_at: %w", ErrZeroPlanetTimestamp)
		}
	}
	if planet.OwnerPlayerID.IsZero() && planet.OwnerChangedAt != nil {
		return fmt.Errorf("owner_player_id: %w", ErrInvalidPlanet)
	}
	return nil
}

// Validate reports whether key is a non-empty server-generated materialization key.
func (key PlanetMaterializationKey) Validate() error {
	if !validDiscoveryToken(string(key)) {
		return fmt.Errorf("materialization key %q: %w", key, ErrInvalidMaterializationKey)
	}
	return nil
}

// Validate reports whether biome is a supported MVP biome.
func (biome PlanetBiome) Validate() error {
	switch biome {
	case PlanetBiomeOriginBelt,
		PlanetBiomeOuterDrift,
		PlanetBiomeNebula,
		PlanetBiomeDeepSpace,
		PlanetBiomeDeadZone:
		return nil
	default:
		return fmt.Errorf("planet biome %q: %w", biome, ErrInvalidPlanetBiome)
	}
}

// Validate reports whether planetType is a supported MVP planet type.
func (planetType PlanetType) Validate() error {
	switch planetType {
	case PlanetTypeBarren,
		PlanetTypeTerrestrial,
		PlanetTypeIce,
		PlanetTypeOcean,
		PlanetTypeGasGiant:
		return nil
	default:
		return fmt.Errorf("planet type %q: %w", planetType, ErrInvalidPlanetType)
	}
}

// Validate reports whether rarity is a supported MVP rarity.
func (rarity PlanetRarity) Validate() error {
	switch rarity {
	case PlanetRarityCommon,
		PlanetRarityUncommon,
		PlanetRarityRare,
		PlanetRarityEpic,
		PlanetRarityLegendary:
		return nil
	default:
		return fmt.Errorf("planet rarity %q: %w", rarity, ErrInvalidPlanetRarity)
	}
}

func clonePlanet(planet Planet) Planet {
	clone := planet
	clone.OwnerChangedAt = cloneTimePtr(planet.OwnerChangedAt)
	return clone
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func validDiscoveryToken(value string) bool {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

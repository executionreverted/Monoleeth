package server

import (
	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
)

type sessionReadyPayload struct {
	Authenticated   bool                `json:"authenticated"`
	Account         *auth.PublicAccount `json:"account,omitempty"`
	Player          *auth.PublicPlayer  `json:"player,omitempty"`
	Roles           []string            `json:"roles,omitempty"`
	ExpiresAt       int64               `json:"expires_at"`
	ProtocolVersion int                 `json:"protocol_version"`
	ReconnectCursor uint64              `json:"reconnect_cursor"`
}

type playerSnapshotPayload struct {
	Callsign  string `json:"callsign"`
	Rank      int    `json:"rank"`
	HP        int    `json:"hp"`
	MaxHP     int    `json:"max_hp"`
	Shield    int    `json:"shield"`
	MaxShield int    `json:"max_shield"`
	Energy    int    `json:"energy"`
	MaxEnergy int    `json:"max_energy"`
}

type shipSnapshotPayload struct {
	ActiveShipID string `json:"active_ship_id"`
	DisplayName  string `json:"display_name"`
	Hull         int    `json:"hull"`
	MaxHull      int    `json:"max_hull"`
	Shield       int    `json:"shield"`
	MaxShield    int    `json:"max_shield"`
	Capacitor    int    `json:"capacitor"`
	MaxCapacitor int    `json:"max_capacitor"`
	Disabled     bool   `json:"disabled"`
	RepairState  string `json:"repair_state"`
}

type deathShipDisabledPayload struct {
	ShipID         string              `json:"ship_id"`
	DisabledReason string              `json:"disabled_reason"`
	Ship           shipSnapshotPayload `json:"ship"`
	RepairQuote    repairQuotePayload  `json:"repair_quote"`
}

type statSnapshotPayload struct {
	Speed                 float64 `json:"speed"`
	RadarRange            float64 `json:"radar_range"`
	DetectionPower        float64 `json:"detection_power,omitempty"`
	JammerResistance      float64 `json:"jammer_resistance,omitempty"`
	StealthDetectionBonus float64 `json:"stealth_detection_bonus,omitempty"`
	WeaponRange           float64 `json:"weapon_range"`
	CargoCapacity         int64   `json:"cargo_capacity"`
	LootPickupRange       float64 `json:"loot_pickup_range"`
	BasicLaserEnergyCost  int     `json:"basic_laser_energy_cost"`
	BasicLaserCooldownMS  int     `json:"basic_laser_cooldown_ms"`
}

type walletSnapshotPayload struct {
	Credits       int64 `json:"credits"`
	PremiumPaid   int64 `json:"premium_paid"`
	PremiumEarned int64 `json:"premium_earned"`
}

type cargoSnapshotPayload struct {
	Used     int64            `json:"used"`
	Capacity int64            `json:"capacity"`
	Items    []cargoItemStack `json:"items"`
}

type cargoItemStack struct {
	ItemID       string `json:"item_id"`
	DisplayName  string `json:"display_name"`
	Category     string `json:"category"`
	ArtKey       string `json:"art_key"`
	Rarity       string `json:"rarity,omitempty"`
	Quantity     int64  `json:"quantity"`
	UnitWeight   int64  `json:"unit_weight"`
	UsedUnits    int64  `json:"used_units"`
	Location     string `json:"location"`
	MoveEligible bool   `json:"move_eligible"`
	LockedReason string `json:"locked_reason,omitempty"`
}

type progressionSnapshotPayload struct {
	MainLevel   int   `json:"main_level"`
	MainXP      int64 `json:"main_xp"`
	Rank        int   `json:"rank"`
	CombatLevel int   `json:"combat_level,omitempty"`
	CombatXP    int64 `json:"combat_xp,omitempty"`
}

type worldSnapshotPayload struct {
	Sector               sectorPayload                 `json:"sector"`
	Map                  worldmaps.ClientMapProjection `json:"map"`
	Entities             []aoi.EntityPayload           `json:"entities"`
	Minimap              minimapPayload                `json:"minimap"`
	SnapshotCursor       uint64                        `json:"snapshot_cursor"`
	MapSubscriptionEpoch uint64                        `json:"map_subscription_epoch"`
}

type sectorPayload struct {
	SectorKey string `json:"sector_key,omitempty"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	Danger    string `json:"danger"`
	Contested bool   `json:"contested"`
}

type minimapPayload struct {
	RadarRange           float64                 `json:"radar_range"`
	ProjectionWindowSize float64                 `json:"projection_window_size"`
	LiveContacts         []minimapContactPayload `json:"live_contacts"`
	Remembered           []minimapMemoryPayload  `json:"remembered"`
}

type minimapContactPayload struct {
	EntityID         string           `json:"entity_id"`
	EntityType       world.EntityType `json:"entity_type"`
	Position         world.Vec2       `json:"position"`
	Disposition      string           `json:"disposition,omitempty"`
	StatusFlags      []aoi.StatusFlag `json:"status_flags,omitempty"`
	ProjectionSource string           `json:"projection_source"`
}

type minimapMemoryPayload struct {
	Kind             string     `json:"kind"`
	SectorKey        string     `json:"sector_key,omitempty"`
	PublicMapKey     string     `json:"public_map_key,omitempty"`
	PlanetID         string     `json:"planet_id,omitempty"`
	DetailID         string     `json:"detail_id,omitempty"`
	Label            string     `json:"label"`
	Position         world.Vec2 `json:"position"`
	Freshness        string     `json:"freshness"`
	ProjectionSource string     `json:"projection_source"`
}

type portalEnterIntent struct {
	PortalID worldmaps.PortalID `json:"portal_id"`
}

type mapTransferStartedPayload struct {
	PortalID             string `json:"portal_id"`
	FromPublicMapKey     string `json:"from_public_map_key"`
	ToPublicMapKey       string `json:"to_public_map_key"`
	MapSubscriptionEpoch uint64 `json:"map_subscription_epoch"`
}

type mapTransferCompletedPayload struct {
	PortalID             string               `json:"portal_id"`
	FromPublicMapKey     string               `json:"from_public_map_key"`
	ToPublicMapKey       string               `json:"to_public_map_key"`
	Position             world.Vec2           `json:"position"`
	MapSubscriptionEpoch uint64               `json:"map_subscription_epoch"`
	Snapshot             worldSnapshotPayload `json:"snapshot"`
}

type mapTransferFailedPayload struct {
	PortalID             string `json:"portal_id,omitempty"`
	FromPublicMapKey     string `json:"from_public_map_key,omitempty"`
	Reason               string `json:"reason"`
	MapSubscriptionEpoch uint64 `json:"map_subscription_epoch"`
}

func sectorPayloadFromMap(projection worldmaps.ClientMapProjection) sectorPayload {
	sectorKey := projection.PublicMapKey
	if sectorKey == "" {
		sectorKey = projection.MapKey
	}
	if sectorKey == "" {
		sectorKey = runtimeSectorKey
	}
	name := projection.DisplayName
	if name == "" {
		name = sectorKey
	}
	region := projection.Region
	if region == "" {
		region = "Unknown"
	}
	danger := projection.RiskBand
	if danger == "" {
		danger = "low"
	}
	return sectorPayload{
		SectorKey: sectorKey,
		Name:      name,
		Region:    region,
		Danger:    danger,
		Contested: projection.PVPPolicy == "pvp" || projection.PVPPolicy == "contested",
	}
}

func minimapFromAOI(snapshot aoi.Snapshot, radarRange float64) minimapPayload {
	contacts := make([]minimapContactPayload, 0, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		disposition := ""
		if entity.Display != nil {
			disposition = entity.Display.Disposition
		}
		contacts = append(contacts, minimapContactPayload{
			EntityID:         entity.ID.String(),
			EntityType:       entity.Type,
			Position:         entity.Position,
			Disposition:      disposition,
			StatusFlags:      append([]aoi.StatusFlag(nil), entity.StatusFlags...),
			ProjectionSource: runtimeProjectionSourceWorker,
		})
	}
	return minimapPayload{
		RadarRange:           radarRange,
		ProjectionWindowSize: radarRange * 2,
		LiveContacts:         contacts,
		Remembered:           []minimapMemoryPayload{},
	}
}

func publicMapKeyFromProjection(projection worldmaps.ClientMapProjection) string {
	if projection.PublicMapKey != "" {
		return projection.PublicMapKey
	}
	return projection.MapKey
}

func intelAndPlanetMatchActiveMap(intel discovery.PlayerPlanetIntel, planet discovery.Planet, worldID foundation.WorldID, zoneID foundation.ZoneID) bool {
	return intel.WorldID == worldID &&
		intel.ZoneID == zoneID &&
		planet.WorldID == worldID &&
		planet.ZoneID == zoneID
}

func planetMemoryLabel(planet discovery.Planet) string {
	if planet.Type != "" && planet.Biome != "" {
		return string(planet.Type) + " " + string(planet.Biome)
	}
	if planet.Type != "" {
		return string(planet.Type)
	}
	return planet.ID.String()
}

func cloneAOIEntities(entities []aoi.EntityPayload) []aoi.EntityPayload {
	if len(entities) == 0 {
		return nil
	}
	cloned := make([]aoi.EntityPayload, 0, len(entities))
	for _, entity := range entities {
		entity.StatusFlags = append([]aoi.StatusFlag(nil), entity.StatusFlags...)
		if entity.Display != nil {
			display := *entity.Display
			entity.Display = &display
		}
		if entity.Combat != nil {
			combatStatus := *entity.Combat
			entity.Combat = &combatStatus
		}
		if entity.Movement != nil {
			movementStatus := *entity.Movement
			entity.Movement = &movementStatus
		}
		cloned = append(cloned, entity)
	}
	return cloned
}

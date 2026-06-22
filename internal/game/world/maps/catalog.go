package maps

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

const (
	// PlayableMinCoordinate and PlayableMaxCoordinate define the bounded map
	// contract for this map-rework slice.
	PlayableMinCoordinate = 0
	PlayableMaxCoordinate = 10000

	StarterMapID   MapID   = "map_1_1"
	StarterSpawnID SpawnID = "starter"
)

const (
	riskBandLow    = "low"
	riskBandMedium = "medium"
	riskBandHigh   = "high"

	pvpPolicyPVE       = "pve"
	pvpPolicySafe      = "safe"
	pvpPolicyPVP       = "pvp"
	pvpPolicyContested = "contested"
)

var (
	ErrInvalidCatalog       = errors.New("invalid map catalog")
	ErrMapNotFound          = errors.New("map not found")
	ErrSpawnNotFound        = errors.New("spawn not found")
	ErrPortalNotFound       = errors.New("portal not found")
	ErrPositionOutOfBounds  = errors.New("position outside map bounds")
	ErrInvalidMapDefinition = errors.New("invalid map definition")
)

// MapID is the server-only internal gameplay map id.
type MapID string

// PublicMapKey is the stable client-safe map label.
type PublicMapKey string

// SpawnID identifies a server-owned spawn point within one map.
type SpawnID string

// PortalID identifies a portal scoped by source map.
type PortalID string

// SafeZoneID identifies a safe zone scoped by map.
type SafeZoneID string

func (id MapID) String() string         { return string(id) }
func (key PublicMapKey) String() string { return string(key) }
func (id SpawnID) String() string       { return string(id) }
func (id PortalID) String() string      { return string(id) }
func (id SafeZoneID) String() string    { return string(id) }

func (id MapID) ZoneID() world.ZoneID { return world.ZoneID(id) }

func (id MapID) Validate() error         { return validateCatalogID("map", string(id)) }
func (key PublicMapKey) Validate() error { return validateCatalogID("public map key", string(key)) }
func (id SpawnID) Validate() error       { return validateCatalogID("spawn", string(id)) }
func (id PortalID) Validate() error      { return validateCatalogID("portal", string(id)) }
func (id SafeZoneID) Validate() error    { return validateCatalogID("safe zone", string(id)) }

// Bounds describes inclusive local map coordinates.
type Bounds struct {
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
}

// ExactPlayableBounds returns the required 0..10000 map rectangle.
func ExactPlayableBounds() Bounds {
	return Bounds{
		MinX: PlayableMinCoordinate,
		MinY: PlayableMinCoordinate,
		MaxX: PlayableMaxCoordinate,
		MaxY: PlayableMaxCoordinate,
	}
}

func (bounds Bounds) ValidateExactPlayable() error {
	for _, value := range []float64{bounds.MinX, bounds.MinY, bounds.MaxX, bounds.MaxY} {
		if !isFinite(value) {
			return fmt.Errorf("bounds %+v: %w", bounds, ErrInvalidMapDefinition)
		}
	}
	if bounds != ExactPlayableBounds() {
		return fmt.Errorf("bounds %+v must equal 0..10000: %w", bounds, ErrInvalidMapDefinition)
	}
	return nil
}

func (bounds Bounds) Contains(position world.Vec2) bool {
	if err := position.Validate(); err != nil {
		return false
	}
	return position.X >= bounds.MinX &&
		position.Y >= bounds.MinY &&
		position.X <= bounds.MaxX &&
		position.Y <= bounds.MaxY
}

// SpawnPointDefinition is server-owned catalog data.
type SpawnPointDefinition struct {
	SpawnID  SpawnID    `json:"-"`
	Position world.Vec2 `json:"-"`
	Label    string     `json:"-"`
}

// PortalDefinition is server-owned catalog data. Destination fields must never
// be serialized to clients as gameplay truth.
type PortalDefinition struct {
	PortalID           PortalID `json:"-"`
	SourceMapID        MapID    `json:"-"`
	SourcePosition     world.Vec2
	InteractionRadius  float64
	DestinationMapID   MapID   `json:"-"`
	DestinationSpawnID SpawnID `json:"-"`
	DisplayName        string
	Visible            bool
}

// SafeZoneDefinition is server-owned catalog data used for hangar/station
// safety classification.
type SafeZoneDefinition struct {
	SafeZoneID    SafeZoneID `json:"-"`
	Center        world.Vec2
	Radius        float64
	DisplayName   string
	BlocksPVP     bool
	HangarActions bool
}

func (safeZone SafeZoneDefinition) Contains(position world.Vec2) bool {
	if safeZone.Radius <= 0 || !isFinite(safeZone.Radius) {
		return false
	}
	if err := position.Validate(); err != nil {
		return false
	}
	return safeZone.Center.DistanceSquared(position) <= safeZone.Radius*safeZone.Radius
}

// MapDefinition is server-owned bounded map catalog data.
type MapDefinition struct {
	InternalMapID    MapID
	PublicMapKey     PublicMapKey
	WorldID          world.WorldID
	ZoneID           world.ZoneID
	DisplayName      string
	Region           string
	RiskBand         string
	PVPPolicy        string
	VisualThemeKey   string
	Bounds           Bounds
	SpawnPoints      []SpawnPointDefinition
	Portals          []PortalDefinition
	SafeZones        []SafeZoneDefinition
	SpawnAreas       []MapSpawnAreaDefinition  `json:"-"`
	EnemyPools       []MapEnemyPoolDefinition  `json:"-"`
	NPCEventSpawns   []NPCEventSpawnDefinition `json:"-"`
	NPCStatTemplates []NPCStatTemplate         `json:"-"`
	NPCDropProfiles  []NPCDropProfile          `json:"-"`
	NPCAggroProfiles []NPCAggroProfile         `json:"-"`
	NPCLeashProfiles []NPCLeashProfile         `json:"-"`
}

// ClientMapProjection is the client-safe public subset of a map definition.
type ClientMapProjection struct {
	MapKey         string                     `json:"map_key"`
	PublicMapKey   string                     `json:"public_map_key"`
	DisplayName    string                     `json:"display_name"`
	Region         string                     `json:"region"`
	RiskBand       string                     `json:"risk_band"`
	PVPPolicy      string                     `json:"pvp_policy"`
	VisualThemeKey string                     `json:"visual_theme_key,omitempty"`
	Bounds         Bounds                     `json:"bounds"`
	VisiblePortals []ClientPortalProjection   `json:"visible_portals"`
	SafeZones      []ClientSafeZoneProjection `json:"safe_zones,omitempty"`
	SafeZone       *ClientSafeZoneSummary     `json:"safe_zone,omitempty"`
	Protection     *ClientProtectionSummary   `json:"protection,omitempty"`
}

// ClientPortalProjection is the client-safe public subset of a visible portal.
type ClientPortalProjection struct {
	PortalID          string     `json:"portal_id"`
	DisplayName       string     `json:"display_name,omitempty"`
	Position          world.Vec2 `json:"position"`
	InteractionRadius float64    `json:"interaction_radius"`
}

// ClientSafeZoneProjection is the client-safe public subset of a safe zone.
type ClientSafeZoneProjection struct {
	SafeAreaID    string     `json:"safe_area_id"`
	DisplayName   string     `json:"display_name,omitempty"`
	Center        world.Vec2 `json:"center"`
	Radius        float64    `json:"radius"`
	BlocksPVP     bool       `json:"blocks_pvp"`
	HangarActions bool       `json:"hangar_actions"`
}

// ClientSafeZoneSummary is the viewer's own current safe-zone state. It is
// populated by runtime snapshot projection, not by static catalog projection.
type ClientSafeZoneSummary struct {
	Inside              bool  `json:"inside"`
	BlocksPVP           bool  `json:"blocks_pvp"`
	ProtectionExpiresAt int64 `json:"protection_expires_at,omitempty"`
}

// ClientProtectionSummary is the viewer's own active spawn/portal protection.
type ClientProtectionSummary struct {
	Reason           string `json:"reason"`
	ExpiresAt        int64  `json:"expires_at"`
	BlocksPVP        bool   `json:"blocks_pvp"`
	BreakOnPVPAction bool   `json:"break_on_pvp_action"`
}

// Catalog stores validated server-owned map definitions.
type Catalog struct {
	byID           map[MapID]MapDefinition
	byPublicKey    map[PublicMapKey]MapID
	starterMapID   MapID
	starterSpawnID SpawnID
}

// NewCatalog validates definitions and returns an immutable in-memory catalog.
func NewCatalog(definitions []MapDefinition, starterMapID MapID, starterSpawnID SpawnID) (*Catalog, error) {
	if len(definitions) == 0 {
		return nil, fmt.Errorf("empty definitions: %w", ErrInvalidCatalog)
	}
	if err := starterMapID.Validate(); err != nil {
		return nil, fmt.Errorf("starter map: %w", err)
	}
	if err := starterSpawnID.Validate(); err != nil {
		return nil, fmt.Errorf("starter spawn: %w", err)
	}

	catalog := &Catalog{
		byID:           make(map[MapID]MapDefinition, len(definitions)),
		byPublicKey:    make(map[PublicMapKey]MapID, len(definitions)),
		starterMapID:   starterMapID,
		starterSpawnID: starterSpawnID,
	}

	for _, definition := range definitions {
		if err := validateMapDefinitionBasics(definition); err != nil {
			return nil, err
		}
		if _, exists := catalog.byID[definition.InternalMapID]; exists {
			return nil, fmt.Errorf("duplicate map %q: %w", definition.InternalMapID, ErrInvalidCatalog)
		}
		if _, exists := catalog.byPublicKey[definition.PublicMapKey]; exists {
			return nil, fmt.Errorf("duplicate public map key %q: %w", definition.PublicMapKey, ErrInvalidCatalog)
		}
		catalog.byID[definition.InternalMapID] = cloneDefinition(definition)
		catalog.byPublicKey[definition.PublicMapKey] = definition.InternalMapID
	}

	for _, definition := range catalog.byID {
		if err := catalog.validateMapContents(definition); err != nil {
			return nil, err
		}
	}
	if _, ok := catalog.Spawn(starterMapID, starterSpawnID); !ok {
		return nil, fmt.Errorf("starter spawn %q in map %q: %w", starterSpawnID, starterMapID, ErrSpawnNotFound)
	}
	return catalog, nil
}

func (catalog *Catalog) Get(mapID MapID) (MapDefinition, bool) {
	if catalog == nil {
		return MapDefinition{}, false
	}
	definition, ok := catalog.byID[mapID]
	if !ok {
		return MapDefinition{}, false
	}
	return cloneDefinition(definition), true
}

func (catalog *Catalog) ByPublicKey(key PublicMapKey) (MapDefinition, bool) {
	if catalog == nil {
		return MapDefinition{}, false
	}
	mapID, ok := catalog.byPublicKey[key]
	if !ok {
		return MapDefinition{}, false
	}
	return catalog.Get(mapID)
}

func (catalog *Catalog) Definitions() []MapDefinition {
	if catalog == nil {
		return nil
	}
	definitions := make([]MapDefinition, 0, len(catalog.byID))
	for _, definition := range catalog.byID {
		definitions = append(definitions, cloneDefinition(definition))
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].InternalMapID < definitions[j].InternalMapID
	})
	return definitions
}

func (catalog *Catalog) StarterDefinition() (MapDefinition, SpawnPointDefinition, error) {
	if catalog == nil {
		return MapDefinition{}, SpawnPointDefinition{}, ErrInvalidCatalog
	}
	definition, ok := catalog.Get(catalog.starterMapID)
	if !ok {
		return MapDefinition{}, SpawnPointDefinition{}, fmt.Errorf("starter map %q: %w", catalog.starterMapID, ErrMapNotFound)
	}
	spawn, ok := catalog.Spawn(catalog.starterMapID, catalog.starterSpawnID)
	if !ok {
		return MapDefinition{}, SpawnPointDefinition{}, fmt.Errorf("starter spawn %q: %w", catalog.starterSpawnID, ErrSpawnNotFound)
	}
	return definition, spawn, nil
}

func (catalog *Catalog) Spawn(mapID MapID, spawnID SpawnID) (SpawnPointDefinition, bool) {
	definition, ok := catalog.Get(mapID)
	if !ok {
		return SpawnPointDefinition{}, false
	}
	for _, spawn := range definition.SpawnPoints {
		if spawn.SpawnID == spawnID {
			return spawn, true
		}
	}
	return SpawnPointDefinition{}, false
}

func (catalog *Catalog) Portal(sourceMapID MapID, portalID PortalID) (PortalDefinition, bool) {
	definition, ok := catalog.Get(sourceMapID)
	if !ok {
		return PortalDefinition{}, false
	}
	for _, portal := range definition.Portals {
		if portal.PortalID == portalID {
			return portal, true
		}
	}
	return PortalDefinition{}, false
}

func (catalog *Catalog) ValidatePosition(mapID MapID, position world.Vec2) error {
	definition, ok := catalog.Get(mapID)
	if !ok {
		return fmt.Errorf("map %q: %w", mapID, ErrMapNotFound)
	}
	if err := position.Validate(); err != nil {
		return err
	}
	if !definition.Bounds.Contains(position) {
		return fmt.Errorf("map %q position %+v: %w", mapID, position, ErrPositionOutOfBounds)
	}
	return nil
}

func (catalog *Catalog) ClientProjection(mapID MapID) (ClientMapProjection, error) {
	definition, ok := catalog.Get(mapID)
	if !ok {
		return ClientMapProjection{}, fmt.Errorf("map %q: %w", mapID, ErrMapNotFound)
	}
	return definition.ClientProjection(), nil
}

func (definition MapDefinition) ClientProjection() ClientMapProjection {
	portalDefinitions := append([]PortalDefinition(nil), definition.Portals...)
	sort.Slice(portalDefinitions, func(i, j int) bool {
		return portalDefinitions[i].PortalID < portalDefinitions[j].PortalID
	})
	portals := make([]ClientPortalProjection, 0, len(portalDefinitions))
	for _, portal := range portalDefinitions {
		if !portal.Visible {
			continue
		}
		portals = append(portals, ClientPortalProjection{
			PortalID:          portal.PortalID.String(),
			DisplayName:       portal.DisplayName,
			Position:          portal.SourcePosition,
			InteractionRadius: portal.InteractionRadius,
		})
	}
	safeZoneDefinitions := append([]SafeZoneDefinition(nil), definition.SafeZones...)
	sort.Slice(safeZoneDefinitions, func(i, j int) bool {
		return safeZoneDefinitions[i].SafeZoneID < safeZoneDefinitions[j].SafeZoneID
	})
	safeZones := make([]ClientSafeZoneProjection, 0, len(safeZoneDefinitions))
	for _, safeZone := range safeZoneDefinitions {
		safeZones = append(safeZones, ClientSafeZoneProjection{
			SafeAreaID:    safeZone.SafeZoneID.String(),
			DisplayName:   safeZone.DisplayName,
			Center:        safeZone.Center,
			Radius:        safeZone.Radius,
			BlocksPVP:     safeZone.BlocksPVP,
			HangarActions: safeZone.HangarActions,
		})
	}
	return ClientMapProjection{
		MapKey:         definition.PublicMapKey.String(),
		PublicMapKey:   definition.PublicMapKey.String(),
		DisplayName:    definition.DisplayName,
		Region:         definition.Region,
		RiskBand:       definition.RiskBand,
		PVPPolicy:      definition.PVPPolicy,
		VisualThemeKey: definition.VisualThemeKey,
		Bounds:         definition.Bounds,
		VisiblePortals: portals,
		SafeZones:      safeZones,
	}
}

func (definition MapDefinition) SafeZoneAt(position world.Vec2) (SafeZoneDefinition, bool) {
	for _, safeZone := range definition.SafeZones {
		if safeZone.Contains(position) {
			return safeZone, true
		}
	}
	return SafeZoneDefinition{}, false
}

func (definition MapDefinition) PVPBlockingSafeZoneAt(position world.Vec2) (SafeZoneDefinition, bool) {
	safeZone, ok := definition.SafeZoneAt(position)
	return safeZone, ok && safeZone.BlocksPVP
}

// StarterCatalog returns the first DarkOrbit-style bounded map set.
func StarterCatalog(worldID world.WorldID) (*Catalog, error) {
	bounds := ExactPlayableBounds()
	definitions := []MapDefinition{
		{
			InternalMapID:  StarterMapID,
			PublicMapKey:   "1-1",
			WorldID:        worldID,
			ZoneID:         StarterMapID.ZoneID(),
			DisplayName:    "Origin Fringe",
			Region:         "Origin Belt",
			RiskBand:       "low",
			PVPPolicy:      "safe",
			VisualThemeKey: "starter-blue",
			Bounds:         bounds,
			SpawnPoints: []SpawnPointDefinition{
				{SpawnID: StarterSpawnID, Position: world.Vec2{X: 0, Y: 0}, Label: "Starter Dock"},
				{SpawnID: "east_gate", Position: world.Vec2{X: 9600, Y: 5000}, Label: "East Gate"},
			},
			SafeZones: []SafeZoneDefinition{
				{SafeZoneID: "starter_dock", Center: world.Vec2{X: 0, Y: 0}, Radius: 250, DisplayName: "Starter Dock", BlocksPVP: true, HangarActions: true},
				{SafeZoneID: "east_gate", Center: world.Vec2{X: 9600, Y: 5000}, Radius: 260, DisplayName: "East Gate", BlocksPVP: true, HangarActions: true},
			},
			Portals: []PortalDefinition{
				{
					PortalID:           "east_gate",
					SourceMapID:        StarterMapID,
					SourcePosition:     world.Vec2{X: 9800, Y: 5000},
					InteractionRadius:  180,
					DestinationMapID:   "map_1_2",
					DestinationSpawnID: "west_gate",
					DisplayName:        "East Gate",
					Visible:            true,
				},
			},
			SpawnAreas:       starterMapSpawnAreas(),
			EnemyPools:       starterMapEnemyPools(),
			NPCEventSpawns:   starterMapNPCEventSpawns(),
			NPCStatTemplates: starterMapNPCStatTemplates(),
			NPCDropProfiles:  starterMapNPCDropProfiles(),
			NPCAggroProfiles: starterMapNPCAggroProfiles(),
			NPCLeashProfiles: starterMapNPCLeashProfiles(),
		},
		{
			InternalMapID:  "map_1_2",
			PublicMapKey:   "1-2",
			WorldID:        worldID,
			ZoneID:         MapID("map_1_2").ZoneID(),
			DisplayName:    "Outer Ring",
			Region:         "Origin Belt",
			RiskBand:       "low",
			PVPPolicy:      "pve",
			VisualThemeKey: "starter-violet",
			Bounds:         bounds,
			SpawnPoints: []SpawnPointDefinition{
				{SpawnID: "west_gate", Position: world.Vec2{X: 400, Y: 5000}, Label: "West Gate"},
			},
			SafeZones: []SafeZoneDefinition{
				{SafeZoneID: "west_gate", Center: world.Vec2{X: 400, Y: 5000}, Radius: 260, DisplayName: "West Gate", BlocksPVP: true, HangarActions: true},
			},
			Portals: []PortalDefinition{
				{
					PortalID:           "west_gate",
					SourceMapID:        "map_1_2",
					SourcePosition:     world.Vec2{X: 200, Y: 5000},
					InteractionRadius:  180,
					DestinationMapID:   StarterMapID,
					DestinationSpawnID: StarterSpawnID,
					DisplayName:        "West Gate",
					Visible:            true,
				},
				{
					PortalID:           "skirmish_gate",
					SourceMapID:        "map_1_2",
					SourcePosition:     world.Vec2{X: 9800, Y: 5000},
					InteractionRadius:  180,
					DestinationMapID:   "map_1_3",
					DestinationSpawnID: "west_gate",
					DisplayName:        "Skirmish Gate",
					Visible:            true,
				},
			},
			SpawnAreas:       outerRingMapSpawnAreas(),
			EnemyPools:       outerRingMapEnemyPools(),
			NPCStatTemplates: outerRingMapNPCStatTemplates(),
			NPCDropProfiles:  outerRingMapNPCDropProfiles(),
			NPCAggroProfiles: outerRingMapNPCAggroProfiles(),
			NPCLeashProfiles: outerRingMapNPCLeashProfiles(),
		},
		{
			InternalMapID:  "map_1_3",
			PublicMapKey:   "1-3",
			WorldID:        worldID,
			ZoneID:         MapID("map_1_3").ZoneID(),
			DisplayName:    "Border Skirmish",
			Region:         "Origin Belt",
			RiskBand:       "medium",
			PVPPolicy:      "pvp",
			VisualThemeKey: "border-amber",
			Bounds:         bounds,
			SpawnPoints: []SpawnPointDefinition{
				{SpawnID: "west_gate", Position: world.Vec2{X: 400, Y: 5000}, Label: "West Gate"},
			},
			SafeZones: []SafeZoneDefinition{
				{SafeZoneID: "west_gate", Center: world.Vec2{X: 400, Y: 5000}, Radius: 260, DisplayName: "West Gate", BlocksPVP: true, HangarActions: true},
			},
			Portals: []PortalDefinition{
				{
					PortalID:           "west_gate",
					SourceMapID:        "map_1_3",
					SourcePosition:     world.Vec2{X: 200, Y: 5000},
					InteractionRadius:  180,
					DestinationMapID:   "map_1_2",
					DestinationSpawnID: "west_gate",
					DisplayName:        "West Gate",
					Visible:            true,
				},
			},
		},
	}
	return NewCatalog(definitions, StarterMapID, StarterSpawnID)
}

func validateMapDefinitionBasics(definition MapDefinition) error {
	if err := definition.InternalMapID.Validate(); err != nil {
		return fmt.Errorf("internal map id: %w", err)
	}
	if err := definition.PublicMapKey.Validate(); err != nil {
		return fmt.Errorf("public map key: %w", err)
	}
	if err := definition.WorldID.Validate(); err != nil {
		return fmt.Errorf("world id: %w", err)
	}
	if err := definition.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone id: %w", err)
	}
	if definition.ZoneID != definition.InternalMapID.ZoneID() {
		return fmt.Errorf("zone %q must equal internal map id %q: %w", definition.ZoneID, definition.InternalMapID, ErrInvalidMapDefinition)
	}
	if strings.TrimSpace(definition.DisplayName) == "" {
		return fmt.Errorf("display name: %w", ErrInvalidMapDefinition)
	}
	if err := validateRiskBand(definition.RiskBand); err != nil {
		return err
	}
	if err := validatePVPPolicy(definition.PVPPolicy); err != nil {
		return err
	}
	if err := definition.Bounds.ValidateExactPlayable(); err != nil {
		return err
	}
	return nil
}

func validateRiskBand(riskBand string) error {
	switch riskBand {
	case riskBandLow, riskBandMedium, riskBandHigh:
		return nil
	default:
		return fmt.Errorf("risk band %q: %w", riskBand, ErrInvalidMapDefinition)
	}
}

func validatePVPPolicy(policy string) error {
	switch policy {
	case pvpPolicyPVE, pvpPolicySafe, pvpPolicyPVP, pvpPolicyContested:
		return nil
	default:
		return fmt.Errorf("pvp policy %q: %w", policy, ErrInvalidMapDefinition)
	}
}

func (catalog *Catalog) validateMapContents(definition MapDefinition) error {
	spawnIDs := make(map[SpawnID]struct{}, len(definition.SpawnPoints))
	for _, spawn := range definition.SpawnPoints {
		if err := spawn.SpawnID.Validate(); err != nil {
			return fmt.Errorf("map %q spawn: %w", definition.InternalMapID, err)
		}
		if _, exists := spawnIDs[spawn.SpawnID]; exists {
			return fmt.Errorf("map %q duplicate spawn %q: %w", definition.InternalMapID, spawn.SpawnID, ErrInvalidCatalog)
		}
		spawnIDs[spawn.SpawnID] = struct{}{}
		if err := catalog.ValidatePosition(definition.InternalMapID, spawn.Position); err != nil {
			return fmt.Errorf("map %q spawn %q: %w", definition.InternalMapID, spawn.SpawnID, err)
		}
	}

	portalIDs := make(map[PortalID]struct{}, len(definition.Portals))
	for _, portal := range definition.Portals {
		if err := portal.PortalID.Validate(); err != nil {
			return fmt.Errorf("map %q portal: %w", definition.InternalMapID, err)
		}
		if portal.SourceMapID != definition.InternalMapID {
			return fmt.Errorf("portal %q source %q must equal map %q: %w", portal.PortalID, portal.SourceMapID, definition.InternalMapID, ErrInvalidCatalog)
		}
		if _, exists := portalIDs[portal.PortalID]; exists {
			return fmt.Errorf("map %q duplicate portal %q: %w", definition.InternalMapID, portal.PortalID, ErrInvalidCatalog)
		}
		portalIDs[portal.PortalID] = struct{}{}
		if err := catalog.ValidatePosition(definition.InternalMapID, portal.SourcePosition); err != nil {
			return fmt.Errorf("map %q portal %q: %w", definition.InternalMapID, portal.PortalID, err)
		}
		if portal.InteractionRadius <= 0 || !isFinite(portal.InteractionRadius) {
			return fmt.Errorf("map %q portal %q radius %v: %w", definition.InternalMapID, portal.PortalID, portal.InteractionRadius, ErrInvalidMapDefinition)
		}
		if _, ok := catalog.Get(portal.DestinationMapID); !ok {
			return fmt.Errorf("portal %q destination map %q: %w", portal.PortalID, portal.DestinationMapID, ErrMapNotFound)
		}
		if _, ok := catalog.Spawn(portal.DestinationMapID, portal.DestinationSpawnID); !ok {
			return fmt.Errorf("portal %q destination spawn %q: %w", portal.PortalID, portal.DestinationSpawnID, ErrSpawnNotFound)
		}
	}

	safeZoneIDs := make(map[SafeZoneID]struct{}, len(definition.SafeZones))
	for _, safeZone := range definition.SafeZones {
		if err := safeZone.SafeZoneID.Validate(); err != nil {
			return fmt.Errorf("map %q safe zone: %w", definition.InternalMapID, err)
		}
		if _, exists := safeZoneIDs[safeZone.SafeZoneID]; exists {
			return fmt.Errorf("map %q duplicate safe zone %q: %w", definition.InternalMapID, safeZone.SafeZoneID, ErrInvalidCatalog)
		}
		safeZoneIDs[safeZone.SafeZoneID] = struct{}{}
		if err := catalog.ValidatePosition(definition.InternalMapID, safeZone.Center); err != nil {
			return fmt.Errorf("map %q safe zone %q: %w", definition.InternalMapID, safeZone.SafeZoneID, err)
		}
		if safeZone.Radius <= 0 || !isFinite(safeZone.Radius) {
			return fmt.Errorf("map %q safe zone %q radius %v: %w", definition.InternalMapID, safeZone.SafeZoneID, safeZone.Radius, ErrInvalidMapDefinition)
		}
	}
	if err := validateEnemyContent(definition); err != nil {
		return err
	}
	return nil
}

func cloneDefinition(definition MapDefinition) MapDefinition {
	definition.SpawnPoints = append([]SpawnPointDefinition(nil), definition.SpawnPoints...)
	definition.Portals = append([]PortalDefinition(nil), definition.Portals...)
	definition.SafeZones = append([]SafeZoneDefinition(nil), definition.SafeZones...)
	definition.SpawnAreas = append([]MapSpawnAreaDefinition(nil), definition.SpawnAreas...)
	definition.EnemyPools = cloneEnemyPoolDefinitions(definition.EnemyPools)
	definition.NPCEventSpawns = cloneNPCEventSpawnDefinitions(definition.NPCEventSpawns)
	definition.NPCStatTemplates = append([]NPCStatTemplate(nil), definition.NPCStatTemplates...)
	definition.NPCDropProfiles = append([]NPCDropProfile(nil), definition.NPCDropProfiles...)
	definition.NPCAggroProfiles = append([]NPCAggroProfile(nil), definition.NPCAggroProfiles...)
	definition.NPCLeashProfiles = append([]NPCLeashProfile(nil), definition.NPCLeashProfiles...)
	return definition
}

func validateCatalogID(kind string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s id: %w", kind, foundation.ErrEmptyID)
	}
	if value != strings.TrimSpace(value) || strings.Contains(value, ":") {
		return fmt.Errorf("%s id %q: %w", kind, value, foundation.ErrInvalidID)
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

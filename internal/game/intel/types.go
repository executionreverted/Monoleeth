package intel

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

// IntelState records how trustworthy a player's personal planet memory is.
type IntelState string

const (
	IntelStateFresh            IntelState = "fresh"
	IntelStateStale            IntelState = "stale"
	IntelStateVerified         IntelState = "verified"
	IntelStateInvalidated      IntelState = "invalidated"
	IntelStateColonizedByOther IntelState = "colonized_by_other"
)

// IntelSourceType names the server-side source that wrote a player intel row.
type IntelSourceType string

const (
	IntelSourceScanSuccess        IntelSourceType = "scan_success"
	IntelSourceShareReceived      IntelSourceType = "share_received"
	IntelSourceCoordinateItemUsed IntelSourceType = "coordinate_item_used"
	IntelSourceQuestReward        IntelSourceType = "quest_reward"
	IntelSourceMarketPurchase     IntelSourceType = "market_purchase"
	IntelSourceAdmin              IntelSourceType = "admin"
)

// PlayerPlanetIntel is one player's server-owned planet memory record.
type PlayerPlanetIntel struct {
	PlayerID    foundation.PlayerID `json:"player_id"`
	PlanetID    foundation.PlanetID `json:"planet_id"`
	WorldID     foundation.WorldID  `json:"world_id"`
	ZoneID      foundation.ZoneID   `json:"zone_id"`
	Coordinates world.Vec2          `json:"coordinates"`

	State      IntelState `json:"state"`
	Confidence int        `json:"confidence"`
	LastSeenAt time.Time  `json:"last_seen_at"`

	SourceType      IntelSourceType `json:"source_type"`
	SourceReference string          `json:"source_reference"`
}

// CoordinateItem is the server-owned payload for one coordinate item instance.
type CoordinateItem struct {
	ItemInstanceID foundation.ItemID   `json:"item_instance_id"`
	OwnerPlayerID  foundation.PlayerID `json:"owner_player_id"`
	PlanetID       foundation.PlanetID `json:"planet_id"`
	WorldID        foundation.WorldID  `json:"world_id"`
	ZoneID         foundation.ZoneID   `json:"zone_id"`
	Coordinates    world.Vec2          `json:"coordinates"`

	State          IntelState `json:"state"`
	Confidence     int        `json:"confidence"`
	LastVerifiedAt time.Time  `json:"last_verified_at"`

	CreatedAt            time.Time                 `json:"created_at"`
	CreatedBy            foundation.PlayerID       `json:"created_by"`
	CreateReference      foundation.IdempotencyKey `json:"create_reference"`
	SourceIntelReference string                    `json:"source_intel_reference"`

	UsedAt       *time.Time                `json:"used_at,omitempty"`
	UsedBy       foundation.PlayerID       `json:"used_by,omitempty"`
	UseReference foundation.IdempotencyKey `json:"use_reference,omitempty"`
}

type SharePlanetIntelInput struct {
	FromPlayerID foundation.PlayerID       `json:"from_player_id"`
	ToPlayerID   foundation.PlayerID       `json:"to_player_id"`
	PlanetID     foundation.PlanetID       `json:"planet_id"`
	Reference    foundation.IdempotencyKey `json:"reference"`
}

type SharePlanetIntelResult struct {
	SourceIntel     PlayerPlanetIntel `json:"source_intel"`
	ReceiverIntel   PlayerPlanetIntel `json:"receiver_intel"`
	Shared          bool              `json:"shared"`
	ReceiverUpdated bool              `json:"receiver_updated"`
	Duplicate       bool              `json:"duplicate,omitempty"`
}

type CreateCoordinateItemInput struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	PlanetID       foundation.PlanetID       `json:"planet_id"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id"`
	Reference      foundation.IdempotencyKey `json:"reference"`
}

type CreateCoordinateItemResult struct {
	Item      CoordinateItem `json:"item"`
	Created   bool           `json:"created"`
	Duplicate bool           `json:"duplicate,omitempty"`
}

type UseCoordinateItemInput struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id"`
	Reference      foundation.IdempotencyKey `json:"reference"`
}

type UseCoordinateItemResult struct {
	Item         CoordinateItem    `json:"item"`
	Intel        PlayerPlanetIntel `json:"intel"`
	Used         bool              `json:"used"`
	IntelUpdated bool              `json:"intel_updated"`
	Duplicate    bool              `json:"duplicate,omitempty"`
}

func (state IntelState) Validate() error {
	switch state {
	case IntelStateFresh,
		IntelStateStale,
		IntelStateVerified,
		IntelStateInvalidated,
		IntelStateColonizedByOther:
		return nil
	default:
		return fmt.Errorf("intel state %q: %w", state, ErrInvalidIntelState)
	}
}

func (state IntelState) Known() bool {
	return state != "" && state != IntelStateInvalidated
}

func (source IntelSourceType) Validate() error {
	switch source {
	case IntelSourceScanSuccess,
		IntelSourceShareReceived,
		IntelSourceCoordinateItemUsed,
		IntelSourceQuestReward,
		IntelSourceMarketPurchase,
		IntelSourceAdmin:
		return nil
	default:
		return fmt.Errorf("intel source %q: %w", source, ErrInvalidIntelSource)
	}
}

func (intel PlayerPlanetIntel) Validate() error {
	if err := intel.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := intel.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := intel.WorldID.Validate(); err != nil {
		return fmt.Errorf("world_id: %w", err)
	}
	if err := intel.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone_id: %w", err)
	}
	if err := intel.Coordinates.Validate(); err != nil {
		return err
	}
	if err := intel.State.Validate(); err != nil {
		return err
	}
	if intel.Confidence < 0 || intel.Confidence > 100 {
		return fmt.Errorf("confidence %d: %w", intel.Confidence, ErrInvalidIntelConfidence)
	}
	if intel.LastSeenAt.IsZero() {
		return fmt.Errorf("last_seen_at: %w", ErrZeroTimestamp)
	}
	if err := intel.SourceType.Validate(); err != nil {
		return err
	}
	if !validReferenceToken(intel.SourceReference) {
		return fmt.Errorf("source_reference %q: %w", intel.SourceReference, ErrInvalidReference)
	}
	return nil
}

func (item CoordinateItem) Validate() error {
	if err := item.ItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("item_instance_id: %w", err)
	}
	if err := item.OwnerPlayerID.Validate(); err != nil {
		return fmt.Errorf("owner_player_id: %w", err)
	}
	if err := item.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := item.WorldID.Validate(); err != nil {
		return fmt.Errorf("world_id: %w", err)
	}
	if err := item.ZoneID.Validate(); err != nil {
		return fmt.Errorf("zone_id: %w", err)
	}
	if err := item.Coordinates.Validate(); err != nil {
		return err
	}
	if err := item.State.Validate(); err != nil {
		return err
	}
	if item.State == IntelStateInvalidated {
		return ErrPlanetIntelInvalidated
	}
	if item.Confidence < 0 || item.Confidence > 100 {
		return fmt.Errorf("confidence %d: %w", item.Confidence, ErrInvalidIntelConfidence)
	}
	if item.LastVerifiedAt.IsZero() {
		return fmt.Errorf("last_verified_at: %w", ErrZeroTimestamp)
	}
	if item.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroTimestamp)
	}
	if err := item.CreatedBy.Validate(); err != nil {
		return fmt.Errorf("created_by: %w", err)
	}
	if err := validateReference(item.CreateReference); err != nil {
		return fmt.Errorf("create_reference: %w", err)
	}
	if !validReferenceToken(item.SourceIntelReference) {
		return fmt.Errorf("source_intel_reference %q: %w", item.SourceIntelReference, ErrInvalidReference)
	}
	if item.UsedAt == nil {
		if !item.UsedBy.IsZero() || !item.UseReference.IsZero() {
			return fmt.Errorf("unused coordinate item has use fields: %w", ErrInvalidCoordinateItem)
		}
		return nil
	}
	if item.UsedAt.IsZero() {
		return fmt.Errorf("used_at: %w", ErrZeroTimestamp)
	}
	if err := item.UsedBy.Validate(); err != nil {
		return fmt.Errorf("used_by: %w", err)
	}
	if err := validateReference(item.UseReference); err != nil {
		return fmt.Errorf("use_reference: %w", err)
	}
	return nil
}

func (input SharePlanetIntelInput) Validate() error {
	if err := input.FromPlayerID.Validate(); err != nil {
		return fmt.Errorf("from_player_id: %w", err)
	}
	if err := input.ToPlayerID.Validate(); err != nil {
		return fmt.Errorf("to_player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := foundation.ValidateIntelShareIdempotencyKey(input.Reference, input.FromPlayerID, input.ToPlayerID, input.PlanetID); err != nil {
		return fmt.Errorf("reference: %w", ErrInvalidReference)
	}
	return nil
}

func (input CreateCoordinateItemInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("item_instance_id: %w", err)
	}
	if err := foundation.ValidateCoordinateItemCreateIdempotencyKey(input.Reference, input.PlayerID, input.PlanetID, input.ItemInstanceID); err != nil {
		return fmt.Errorf("reference: %w", ErrInvalidReference)
	}
	return nil
}

func (input UseCoordinateItemInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	if err := input.ItemInstanceID.Validate(); err != nil {
		return fmt.Errorf("item_instance_id: %w", err)
	}
	if err := foundation.ValidateCoordinateItemUseIdempotencyKey(input.Reference, input.PlayerID, input.ItemInstanceID); err != nil {
		return fmt.Errorf("reference: %w", ErrInvalidReference)
	}
	return nil
}

func validateReference(reference foundation.IdempotencyKey) error {
	if err := reference.Validate(); err != nil {
		return fmt.Errorf("reference %q: %w", reference, ErrInvalidReference)
	}
	return nil
}

func validReferenceToken(value string) bool {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

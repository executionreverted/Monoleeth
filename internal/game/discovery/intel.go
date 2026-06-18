package discovery

import (
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

var (
	ErrInvalidPlayerPlanetIntel = errors.New("invalid player planet intel")
	ErrInvalidIntelState        = errors.New("invalid intel state")
	ErrInvalidIntelConfidence   = errors.New("invalid intel confidence")
	ErrInvalidIntelSource       = errors.New("invalid intel source")
	ErrZeroIntelTimestamp       = errors.New("zero intel timestamp")
	ErrEmptyIntelSourceRef      = errors.New("empty intel source reference")
)

// IntelState records how trustworthy a player's planet memory is.
type IntelState string

const (
	IntelStateFresh            IntelState = "fresh"
	IntelStateStale            IntelState = "stale"
	IntelStateVerified         IntelState = "verified"
	IntelStateInvalidated      IntelState = "invalidated"
	IntelStateColonizedByOther IntelState = "colonized_by_other"
)

// IntelSourceType names the server-side source that created or updated intel.
type IntelSourceType string

const (
	IntelSourceScanSuccess          IntelSourceType = "scan_success"
	IntelSourceShareReceived        IntelSourceType = "share_received"
	IntelSourceCoordinateScrollUsed IntelSourceType = "coordinate_scroll_used"
	IntelSourceQuestReward          IntelSourceType = "quest_reward"
	IntelSourceMarketPurchase       IntelSourceType = "market_purchase"
	IntelSourceAdmin                IntelSourceType = "admin"
	IntelSourcePlanetOwnerChanged   IntelSourceType = "planet_owner_changed"
)

const staleIntelConfidence = 40

// PlayerPlanetIntel is one player's personal fog-memory record for a planet.
type PlayerPlanetIntel struct {
	PlayerID    foundation.PlayerID
	PlanetID    foundation.PlanetID
	Coordinates world.Vec2

	State      IntelState
	Confidence int
	LastSeenAt time.Time

	SourceType      IntelSourceType
	SourceReference string
}

// Validate reports whether intel is a complete server-authored player memory record.
func (intel PlayerPlanetIntel) Validate() error {
	if err := intel.PlayerID.Validate(); err != nil {
		return err
	}
	if err := intel.PlanetID.Validate(); err != nil {
		return err
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
		return fmt.Errorf("last_seen_at: %w", ErrZeroIntelTimestamp)
	}
	if err := intel.SourceType.Validate(); err != nil {
		return err
	}
	if !validDiscoveryToken(intel.SourceReference) {
		return fmt.Errorf("source_reference %q: %w", intel.SourceReference, ErrEmptyIntelSourceRef)
	}
	return nil
}

// Validate reports whether state is a supported planet intel state.
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

// Validate reports whether sourceType is a supported server-side intel source.
func (sourceType IntelSourceType) Validate() error {
	switch sourceType {
	case IntelSourceScanSuccess,
		IntelSourceShareReceived,
		IntelSourceCoordinateScrollUsed,
		IntelSourceQuestReward,
		IntelSourceMarketPurchase,
		IntelSourceAdmin,
		IntelSourcePlanetOwnerChanged:
		return nil
	default:
		return fmt.Errorf("intel source %q: %w", sourceType, ErrInvalidIntelSource)
	}
}

func clonePlayerPlanetIntel(intel PlayerPlanetIntel) PlayerPlanetIntel {
	return intel
}

func shouldReplacePlayerPlanetIntel(existing PlayerPlanetIntel, incoming PlayerPlanetIntel) bool {
	if incoming.LastSeenAt.After(existing.LastSeenAt) {
		return true
	}
	if incoming.LastSeenAt.Before(existing.LastSeenAt) {
		return false
	}
	if incoming.Confidence > existing.Confidence {
		return true
	}
	if incoming.Confidence < existing.Confidence {
		return false
	}
	return intelStateRank(incoming.State) > intelStateRank(existing.State)
}

func intelStateRank(state IntelState) int {
	switch state {
	case IntelStateInvalidated:
		return 0
	case IntelStateStale:
		return 1
	case IntelStateColonizedByOther:
		return 2
	case IntelStateFresh:
		return 3
	case IntelStateVerified:
		return 4
	default:
		return -1
	}
}

func staleMarkedIntel(intel PlayerPlanetIntel, changedAt time.Time, sourceReference string) PlayerPlanetIntel {
	next := clonePlayerPlanetIntel(intel)
	next.State = IntelStateStale
	if next.Confidence > staleIntelConfidence {
		next.Confidence = staleIntelConfidence
	}
	next.LastSeenAt = changedAt.UTC()
	next.SourceType = IntelSourcePlanetOwnerChanged
	next.SourceReference = sourceReference
	return next
}

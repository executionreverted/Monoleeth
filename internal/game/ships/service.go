package ships

import (
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrMissingStarterShipDefinition = errors.New("missing starter ship definition")
	ErrShipNotUnlocked              = errors.New("ship not unlocked")
	ErrCannotSwapInCombat           = errors.New("cannot swap ship in combat")
	ErrNotInHangarArea              = errors.New("not in hangar or safe area")
	ErrShipDisabled                 = errors.New("ship disabled")
	ErrShipUnavailable              = errors.New("ship unavailable")
	ErrInvalidCurrentCargoAmount    = errors.New("invalid current cargo amount")
	ErrCargoExceedsTargetCapacity   = errors.New("current cargo exceeds target ship capacity")
	ErrNilPlayerRankProvider        = errors.New("nil player rank provider")
	ErrShipRankRequirementNotMet    = errors.New("ship rank requirement not met")
)

// ShipService is the ship-facing name for the hangar service slice.
type ShipService = HangarService

// HangarService owns starter ship guarantee, ship unlock, and active ship
// selection for the current Phase 03 slice.
type HangarService struct {
	catalog Catalog
	store   *InMemoryHangarStore
	ranks   PlayerRankProvider
	clock   foundation.Clock
}

// PlayerRankProvider returns the authoritative current rank for a player.
type PlayerRankProvider interface {
	RankForPlayer(playerID foundation.PlayerID) (int, error)
}

// StaticPlayerRankProvider is a deterministic provider for tests and early
// single-process slices.
type StaticPlayerRankProvider map[foundation.PlayerID]int

// UnlockShipInput describes one authoritative ship unlock request.
type UnlockShipInput struct {
	PlayerID    foundation.PlayerID `json:"player_id"`
	ShipID      foundation.ShipID   `json:"ship_id"`
	Source      string              `json:"source,omitempty"`
	ReferenceID string              `json:"reference_id,omitempty"`
}

// ShipSwapContext contains server-derived state used to validate active ship
// changes. CurrentCargoUnits is caller-provided by the authoritative cargo owner.
type ShipSwapContext struct {
	InSafeHangarArea  bool  `json:"in_safe_hangar_area"`
	InCombat          bool  `json:"in_combat"`
	CurrentCargoUnits int64 `json:"current_cargo_units"`
}

// SetActiveShipInput describes an active ship selection or swap request.
type SetActiveShipInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	ShipID   foundation.ShipID   `json:"ship_id"`
	Context  ShipSwapContext     `json:"context"`
}

// StatInvalidationReason identifies why stat cache consumers should refresh.
type StatInvalidationReason string

const (
	// StatInvalidationReasonActiveShipChanged is returned when active ship
	// changes and effective stats must be recalculated by the stats package.
	StatInvalidationReasonActiveShipChanged StatInvalidationReason = "active_ship_changed"
)

// StatInvalidationSignal is returned by this package instead of wiring directly
// into stat aggregation or realtime packages.
type StatInvalidationSignal struct {
	PlayerID       foundation.PlayerID    `json:"player_id"`
	PreviousShipID foundation.ShipID      `json:"previous_ship_id,omitempty"`
	ActiveShipID   foundation.ShipID      `json:"active_ship_id"`
	Reason         StatInvalidationReason `json:"reason"`
	CreatedAt      time.Time              `json:"created_at"`
}

// EnsureStarterShipResult reports starter guarantee effects.
type EnsureStarterShipResult struct {
	PlayerShip       PlayerShipState         `json:"player_ship"`
	Created          bool                    `json:"created"`
	ActiveShip       ActiveShipState         `json:"active_ship,omitempty"`
	HasActiveShip    bool                    `json:"has_active_ship"`
	ActiveChanged    bool                    `json:"active_changed"`
	StatInvalidation *StatInvalidationSignal `json:"stat_invalidation,omitempty"`
}

// UnlockShipResult reports ship unlock effects.
type UnlockShipResult struct {
	PlayerShip PlayerShipState `json:"player_ship"`
	Unlocked   bool            `json:"unlocked"`
	Duplicate  bool            `json:"duplicate"`
}

// SetActiveShipResult reports active ship selection effects.
type SetActiveShipResult struct {
	ActiveShip       ActiveShipState         `json:"active_ship"`
	PreviousShipID   foundation.ShipID       `json:"previous_ship_id,omitempty"`
	ActiveChanged    bool                    `json:"active_changed"`
	StatInvalidation *StatInvalidationSignal `json:"stat_invalidation,omitempty"`
}

// HangarSnapshot is a read-only player hangar view.
type HangarSnapshot struct {
	PlayerID      foundation.PlayerID `json:"player_id"`
	Ships         []PlayerShipState   `json:"ships"`
	ActiveShip    ActiveShipState     `json:"active_ship,omitempty"`
	HasActiveShip bool                `json:"has_active_ship"`
}

// NewShipService returns a ship service backed by an in-memory hangar store.
func NewShipService(
	catalogRows Catalog,
	store *InMemoryHangarStore,
	ranks PlayerRankProvider,
	clock foundation.Clock,
) (*ShipService, error) {
	return NewHangarService(catalogRows, store, ranks, clock)
}

// NewHangarService returns a hangar service backed by an in-memory hangar store.
func NewHangarService(
	catalogRows Catalog,
	store *InMemoryHangarStore,
	ranks PlayerRankProvider,
	clock foundation.Clock,
) (*HangarService, error) {
	if _, ok := catalogRows.Get(ShipIDStarter); !ok {
		return nil, ErrMissingStarterShipDefinition
	}
	if ranks == nil {
		return nil, ErrNilPlayerRankProvider
	}
	if store == nil {
		store = NewInMemoryHangarStore()
	}
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &HangarService{
		catalog: catalogRows,
		store:   store,
		ranks:   ranks,
		clock:   clock,
	}, nil
}

// EnsureStarterShip guarantees the starter ship exists. If the player has no
// active ship and the starter is usable, it also becomes active.
func (service *HangarService) EnsureStarterShip(playerID foundation.PlayerID) (EnsureStarterShipResult, error) {
	if err := playerID.Validate(); err != nil {
		return EnsureStarterShipResult{}, err
	}
	if _, ok := service.catalog.Get(ShipIDStarter); !ok {
		return EnsureStarterShipResult{}, ErrMissingStarterShipDefinition
	}

	now := service.clock.Now()
	var result EnsureStarterShipResult
	err := service.store.updatePlayerHangar(playerID, func(record *hangarRecord) error {
		starter, exists := record.ship(ShipIDStarter)
		if !exists {
			var err error
			starter, err = NewPlayerShipState(playerID, ShipIDStarter, ShipStateAvailable)
			if err != nil {
				return err
			}
			starter.UnlockedAt = now
			record.putShip(starter)
			result.Created = true
		}

		active, hasActive := record.activeShip()
		if !hasActive || active.ShipID == ShipIDStarter {
			activated, activeShip, err := activateStarterIfUsable(record, starter, now)
			if err != nil {
				return err
			}
			if activated {
				result.ActiveChanged = true
				result.StatInvalidation = newStatInvalidationSignal(playerID, "", ShipIDStarter, now)
			}
			if activeShip.PlayerID != "" {
				result.ActiveShip = activeShip
				result.HasActiveShip = true
			}
		} else {
			result.ActiveShip = active
			result.HasActiveShip = true
		}

		starter, _ = record.ship(ShipIDStarter)
		result.PlayerShip = starter
		return nil
	})
	if err != nil {
		return EnsureStarterShipResult{}, err
	}
	return result, nil
}

// UnlockShip unlocks one catalog ship once for the player.
func (service *HangarService) UnlockShip(input UnlockShipInput) (UnlockShipResult, error) {
	if err := input.validate(); err != nil {
		return UnlockShipResult{}, err
	}
	if _, err := service.catalog.MustGet(input.ShipID); err != nil {
		return UnlockShipResult{}, err
	}
	if err := service.validateRankRequirement(input.PlayerID, input.ShipID); err != nil {
		return UnlockShipResult{}, err
	}

	now := service.clock.Now()
	var result UnlockShipResult
	err := service.store.updatePlayerHangar(input.PlayerID, func(record *hangarRecord) error {
		if existing, ok := record.ship(input.ShipID); ok {
			result = UnlockShipResult{
				PlayerShip: existing,
				Duplicate:  true,
			}
			return nil
		}

		playerShip, err := NewPlayerShipState(input.PlayerID, input.ShipID, ShipStateAvailable)
		if err != nil {
			return err
		}
		playerShip.UnlockedAt = now
		record.putShip(playerShip)
		result = UnlockShipResult{
			PlayerShip: playerShip,
			Unlocked:   true,
		}
		return nil
	})
	if err != nil {
		return UnlockShipResult{}, err
	}
	return result, nil
}

// SetActiveShip validates and applies an active ship change.
func (service *HangarService) SetActiveShip(input SetActiveShipInput) (SetActiveShipResult, error) {
	if err := input.validate(); err != nil {
		return SetActiveShipResult{}, err
	}
	targetDefinition, err := service.catalog.MustGet(input.ShipID)
	if err != nil {
		return SetActiveShipResult{}, err
	}
	if err := service.validateRankRequirement(input.PlayerID, input.ShipID); err != nil {
		return SetActiveShipResult{}, err
	}

	now := service.clock.Now()
	var result SetActiveShipResult
	err = service.store.updatePlayerHangar(input.PlayerID, func(record *hangarRecord) error {
		targetShip, ok := record.ship(input.ShipID)
		if !ok {
			return fmt.Errorf("ship %q: %w", input.ShipID, ErrShipNotUnlocked)
		}
		if err := validateTargetShipForActivation(targetShip); err != nil {
			return err
		}
		if err := input.Context.validateForTarget(targetDefinition); err != nil {
			return err
		}

		currentActive, hasActive := record.activeShip()
		if hasActive && currentActive.ShipID == input.ShipID && targetShip.State == ShipStateActive {
			result = SetActiveShipResult{
				ActiveShip: currentActive,
			}
			return nil
		}

		previousShipID := foundation.ShipID("")
		if hasActive {
			previousShipID = currentActive.ShipID
		}
		markOtherActiveShipsAvailable(record, input.ShipID)

		targetShip.State = ShipStateActive
		record.putShip(targetShip)
		activeShip := ActiveShipState{
			PlayerID:    input.PlayerID,
			ShipID:      input.ShipID,
			ActivatedAt: now,
			UpdatedAt:   now,
		}
		if err := activeShip.Validate(); err != nil {
			return err
		}
		record.putActiveShip(activeShip)

		result = SetActiveShipResult{
			ActiveShip:       activeShip,
			PreviousShipID:   previousShipID,
			ActiveChanged:    true,
			StatInvalidation: newStatInvalidationSignal(input.PlayerID, previousShipID, input.ShipID, now),
		}
		return nil
	})
	if err != nil {
		return SetActiveShipResult{}, err
	}
	return result, nil
}

// SwapShip is an alias for SetActiveShip.
func (service *HangarService) SwapShip(input SetActiveShipInput) (SetActiveShipResult, error) {
	return service.SetActiveShip(input)
}

// GetHangar returns a snapshot of a player's ships and active pointer.
func (service *HangarService) GetHangar(playerID foundation.PlayerID) (HangarSnapshot, error) {
	if err := playerID.Validate(); err != nil {
		return HangarSnapshot{}, err
	}

	snapshot := HangarSnapshot{PlayerID: playerID}
	err := service.store.viewPlayerHangar(playerID, func(record hangarRecord) error {
		snapshot.Ships = record.sortedShips()
		if active, ok := record.activeShip(); ok {
			snapshot.ActiveShip = active
			snapshot.HasActiveShip = true
		}
		return nil
	})
	if err != nil {
		return HangarSnapshot{}, err
	}
	return snapshot, nil
}

func (input UnlockShipInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ShipID.Validate(); err != nil {
		return err
	}
	return nil
}

func (input SetActiveShipInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ShipID.Validate(); err != nil {
		return err
	}
	return input.Context.validateCargoAmount()
}

func (context ShipSwapContext) validateForTarget(target ShipDefinition) error {
	if err := context.validateCargoAmount(); err != nil {
		return err
	}
	if context.InCombat {
		return ErrCannotSwapInCombat
	}
	if !context.InSafeHangarArea {
		return ErrNotInHangarArea
	}
	if context.CurrentCargoUnits > target.BaseStats.CargoCapacity {
		return fmt.Errorf(
			"current cargo %d target capacity %d: %w",
			context.CurrentCargoUnits,
			target.BaseStats.CargoCapacity,
			ErrCargoExceedsTargetCapacity,
		)
	}
	return nil
}

func (context ShipSwapContext) validateCargoAmount() error {
	if context.CurrentCargoUnits < 0 {
		return fmt.Errorf("current cargo %d: %w", context.CurrentCargoUnits, ErrInvalidCurrentCargoAmount)
	}
	if context.CurrentCargoUnits > foundation.MaxAmount {
		return fmt.Errorf("current cargo %d exceeds max %d: %w", context.CurrentCargoUnits, foundation.MaxAmount, ErrInvalidCurrentCargoAmount)
	}
	return nil
}

// RankForPlayer returns a configured rank for playerID.
func (provider StaticPlayerRankProvider) RankForPlayer(playerID foundation.PlayerID) (int, error) {
	if err := playerID.Validate(); err != nil {
		return 0, err
	}
	rank, ok := provider[playerID]
	if !ok {
		return 0, fmt.Errorf("player %q: %w", playerID, ErrShipRankRequirementNotMet)
	}
	if rank < 1 {
		return 0, fmt.Errorf("rank %d: %w", rank, ErrShipRankRequirementNotMet)
	}
	return rank, nil
}

func (service *HangarService) validateRankRequirement(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	definition, err := service.catalog.MustGet(shipID)
	if err != nil {
		return err
	}
	rank, err := service.ranks.RankForPlayer(playerID)
	if err != nil {
		return err
	}
	if rank < definition.RankRequirement {
		return fmt.Errorf("player rank %d ship %q requires %d: %w", rank, shipID, definition.RankRequirement, ErrShipRankRequirementNotMet)
	}
	return nil
}

func validateTargetShipForActivation(playerShip PlayerShipState) error {
	if playerShip.State == ShipStateDisabled {
		return ErrShipDisabled
	}
	if playerShip.State != ShipStateAvailable && playerShip.State != ShipStateActive {
		return fmt.Errorf("ship state %q: %w", playerShip.State, ErrShipUnavailable)
	}
	return nil
}

func activateStarterIfUsable(record *hangarRecord, starter PlayerShipState, now time.Time) (bool, ActiveShipState, error) {
	if starter.State == ShipStateDisabled {
		return false, ActiveShipState{}, nil
	}
	if starter.State != ShipStateAvailable && starter.State != ShipStateActive {
		return false, ActiveShipState{}, nil
	}

	active, hasActive := record.activeShip()
	if hasActive && active.ShipID == ShipIDStarter && starter.State == ShipStateActive {
		return false, active, nil
	}

	starter.State = ShipStateActive
	record.putShip(starter)
	activeShip := ActiveShipState{
		PlayerID:    starter.PlayerID,
		ShipID:      ShipIDStarter,
		ActivatedAt: now,
		UpdatedAt:   now,
	}
	if err := activeShip.Validate(); err != nil {
		return false, ActiveShipState{}, err
	}
	record.putActiveShip(activeShip)
	return true, activeShip, nil
}

func markOtherActiveShipsAvailable(record *hangarRecord, targetShipID foundation.ShipID) {
	for shipID, playerShip := range record.ships {
		if shipID == targetShipID || playerShip.State != ShipStateActive {
			continue
		}
		playerShip.State = ShipStateAvailable
		record.putShip(playerShip)
	}
}

func newStatInvalidationSignal(
	playerID foundation.PlayerID,
	previousShipID foundation.ShipID,
	activeShipID foundation.ShipID,
	now time.Time,
) *StatInvalidationSignal {
	return &StatInvalidationSignal{
		PlayerID:       playerID,
		PreviousShipID: previousShipID,
		ActiveShipID:   activeShipID,
		Reason:         StatInvalidationReasonActiveShipChanged,
		CreatedAt:      now,
	}
}

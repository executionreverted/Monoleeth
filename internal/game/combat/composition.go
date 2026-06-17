package combat

import (
	"fmt"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

// ActorFromSnapshotInput is the server-side composition boundary from an
// authoritative stat snapshot into live combat state.
type ActorFromSnapshotInput struct {
	EntityID  world.EntityID
	Type      world.EntityType
	PlayerID  foundation.PlayerID
	WorldID   world.WorldID
	ZoneID    world.ZoneID
	Position  world.Vec2
	Signature visibility.EntitySignature
	Hidden    bool
	Snapshot  stats.StatSnapshot
}

// NewActorFromSnapshot creates live combat state from authoritative server
// stats. Clients may read snapshots, but they must not submit these values.
func NewActorFromSnapshot(input ActorFromSnapshotInput) (ActorState, error) {
	if input.Snapshot.IsInvalidated() {
		return ActorState{}, fmt.Errorf("stat snapshot invalidated: %w", ErrInvalidActorState)
	}
	if input.Snapshot.Version == 0 {
		return ActorState{}, fmt.Errorf("stat snapshot version: %w", ErrInvalidActorState)
	}
	if err := input.Snapshot.ShipID.Validate(); err != nil {
		return ActorState{}, fmt.Errorf("stat snapshot ship %q: %w", input.Snapshot.ShipID, ErrInvalidActorState)
	}
	if input.Type == world.EntityTypePlayer && input.Snapshot.PlayerID != input.PlayerID {
		return ActorState{}, fmt.Errorf("snapshot player %q for player %q: %w", input.Snapshot.PlayerID, input.PlayerID, ErrInvalidActorState)
	}

	actor := ActorState{
		EntityID:      input.EntityID,
		Type:          input.Type,
		PlayerID:      input.PlayerID,
		WorldID:       input.WorldID,
		ZoneID:        input.ZoneID,
		Position:      input.Position,
		Signature:     input.Signature,
		Hidden:        input.Hidden,
		Stats:         input.Snapshot,
		HP:            input.Snapshot.Stats.Core.HPMax,
		Shield:        input.Snapshot.Stats.Core.ShieldMax,
		Energy:        input.Snapshot.Stats.Core.EnergyMax,
		Cooldowns:     CooldownState{},
		Contributions: make(map[foundation.PlayerID]float64),
	}
	if err := actor.validate(); err != nil {
		return ActorState{}, err
	}
	return actor, nil
}

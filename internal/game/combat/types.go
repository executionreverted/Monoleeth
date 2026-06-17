package combat

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

const BasicLaserCooldownKey = "basic_laser"

var (
	ErrUnknownActor       = errors.New("unknown combat actor")
	ErrAttackerDead       = errors.New("attacker dead")
	ErrTargetDead         = errors.New("target dead")
	ErrDifferentWorldZone = errors.New("different world or zone")
	ErrTargetNotVisible   = errors.New("target not visible")
	ErrOutOfRange         = errors.New("target out of range")
	ErrCooldownNotReady   = errors.New("cooldown not ready")
	ErrNotEnoughEnergy    = errors.New("not enough energy")
	ErrInvalidActorState  = errors.New("invalid combat actor state")
	ErrInvalidCooldown    = errors.New("invalid cooldown")
	ErrInvalidDamage      = errors.New("invalid damage")
)

// ActorState is server-owned live combat state for a world entity.
type ActorState struct {
	EntityID world.EntityID
	Type     world.EntityType
	PlayerID foundation.PlayerID

	WorldID  world.WorldID
	ZoneID   world.ZoneID
	Position world.Vec2

	Signature visibility.EntitySignature
	Hidden    bool

	Stats  stats.StatSnapshot
	HP     float64
	Shield float64
	Energy float64

	Dead   bool
	DiedAt *time.Time

	Cooldowns     CooldownState
	Contributions map[foundation.PlayerID]float64
}

// CooldownState records server-owned cooldown readiness by key.
type CooldownState map[string]time.Time

// BasicAttackInput identifies a server-authorized basic laser intent.
type BasicAttackInput struct {
	AttackerID world.EntityID
	TargetID   world.EntityID
}

// BasicAttackResult reports the state transition from one basic laser attempt.
type BasicAttackResult struct {
	Attacker ActorState
	Target   ActorState

	Hit          bool
	Damage       float64
	ShieldDamage float64
	HPDamage     float64

	Killed          bool
	KillEvent       *NPCKilledEvent
	CooldownReadyAt time.Time
}

// NPCKilledEvent is the domain payload consumed by loot/progression hooks.
type NPCKilledEvent struct {
	SourceID      world.EntityID
	NPCEntityID   world.EntityID
	WorldID       world.WorldID
	ZoneID        world.ZoneID
	Position      world.Vec2
	OwnerPlayerID foundation.PlayerID
	KilledAt      time.Time
}

func (actor ActorState) validate() error {
	if err := actor.EntityID.Validate(); err != nil {
		return err
	}
	if err := actor.Type.Validate(); err != nil {
		return err
	}
	if actor.Type == world.EntityTypePlayer {
		if err := actor.PlayerID.Validate(); err != nil {
			return err
		}
	}
	if err := actor.WorldID.Validate(); err != nil {
		return err
	}
	if err := actor.ZoneID.Validate(); err != nil {
		return err
	}
	if err := actor.Position.Validate(); err != nil {
		return err
	}
	if !finiteNonNegative(actor.HP) || !finiteNonNegative(actor.Shield) || !finiteNonNegative(actor.Energy) {
		return ErrInvalidActorState
	}
	if actor.Dead && actor.DiedAt == nil {
		return ErrInvalidActorState
	}
	if actor.Cooldowns == nil {
		actor.Cooldowns = CooldownState{}
	}
	return nil
}

func (input BasicAttackInput) validate() error {
	if err := input.AttackerID.Validate(); err != nil {
		return err
	}
	if err := input.TargetID.Validate(); err != nil {
		return err
	}
	if input.AttackerID == input.TargetID {
		return ErrInvalidActorState
	}
	return nil
}

func (cooldowns CooldownState) Ready(key string, now time.Time) bool {
	if cooldowns == nil {
		return true
	}
	readyAt := cooldowns[key]
	return readyAt.IsZero() || !now.Before(readyAt)
}

func (cooldowns CooldownState) Start(key string, now time.Time, duration time.Duration) (time.Time, error) {
	if duration < 0 {
		return time.Time{}, fmt.Errorf("duration %s: %w", duration, ErrInvalidCooldown)
	}
	if cooldowns == nil {
		return time.Time{}, ErrInvalidCooldown
	}
	if !cooldowns.Ready(key, now) {
		return cooldowns[key], ErrCooldownNotReady
	}
	readyAt := now.Add(duration)
	cooldowns[key] = readyAt
	return readyAt, nil
}

func (actor *ActorState) SpendEnergy(cost float64) error {
	if !finiteNonNegative(cost) {
		return fmt.Errorf("cost %v: %w", cost, ErrInvalidActorState)
	}
	if actor.Energy < cost {
		return ErrNotEnoughEnergy
	}
	actor.Energy -= cost
	return nil
}

func (actor *ActorState) RegenerateEnergy(elapsed time.Duration) {
	if actor == nil || elapsed <= 0 || actor.Dead {
		return
	}
	actor.Energy += actor.Stats.Stats.Core.EnergyRegen * elapsed.Seconds()
	if actor.Energy > actor.Stats.Stats.Core.EnergyMax {
		actor.Energy = actor.Stats.Stats.Core.EnergyMax
	}
	if actor.Energy < 0 || math.IsNaN(actor.Energy) || math.IsInf(actor.Energy, 0) {
		actor.Energy = 0
	}
}

func cloneActor(actor ActorState) ActorState {
	clone := actor
	if actor.DiedAt != nil {
		diedAt := *actor.DiedAt
		clone.DiedAt = &diedAt
	}
	if actor.Cooldowns != nil {
		clone.Cooldowns = make(CooldownState, len(actor.Cooldowns))
		for key, readyAt := range actor.Cooldowns {
			clone.Cooldowns[key] = readyAt
		}
	}
	if actor.Contributions != nil {
		clone.Contributions = make(map[foundation.PlayerID]float64, len(actor.Contributions))
		for playerID, damage := range actor.Contributions {
			clone.Contributions[playerID] = damage
		}
	}
	return clone
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

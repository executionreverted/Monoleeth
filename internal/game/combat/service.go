package combat

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

const (
	EventBasicAttack = "combat.basic_attack"
	EventNPCKilled   = "combat.npc_killed"
)

// EventEmitter is the optional post-mutation event hook.
type EventEmitter interface {
	Record(events.EventEnvelope)
}

// Service owns in-memory Phase 05 combat state.
type Service struct {
	mu    sync.Mutex
	clock foundation.Clock
	rng   foundation.RNG

	actors map[world.EntityID]ActorState

	emitter           EventEmitter
	nextEventSequence uint64
}

// NewService returns an in-memory combat service.
func NewService(clock foundation.Clock, rng foundation.RNG) *Service {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &Service{
		clock:  clock,
		rng:    rng,
		actors: make(map[world.EntityID]ActorState),
	}
}

// SetEventEmitter configures the optional post-mutation event hook.
func (service *Service) SetEventEmitter(emitter EventEmitter) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.emitter = emitter
}

// UpsertActor inserts or replaces one server-owned combat actor.
func (service *Service) UpsertActor(actor ActorState) error {
	if actor.Cooldowns == nil {
		actor.Cooldowns = CooldownState{}
	}
	if actor.Contributions == nil {
		actor.Contributions = make(map[foundation.PlayerID]float64)
	}
	if err := actor.validate(); err != nil {
		return err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	service.actors[actor.EntityID] = cloneActor(actor)
	return nil
}

// Actor returns a copy of one combat actor.
func (service *Service) Actor(entityID world.EntityID) (ActorState, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	actor, ok := service.actors[entityID]
	return cloneActor(actor), ok
}

// RegenerateEnergy applies server-timed energy regeneration to one actor.
func (service *Service) RegenerateEnergy(entityID world.EntityID, elapsed time.Duration) (ActorState, error) {
	if err := entityID.Validate(); err != nil {
		return ActorState{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	actor, ok := service.actors[entityID]
	if !ok {
		return ActorState{}, fmt.Errorf("entity %q: %w", entityID, ErrUnknownActor)
	}
	actor.RegenerateEnergy(elapsed)
	service.actors[entityID] = cloneActor(actor)
	return cloneActor(actor), nil
}

// ExecuteBasicAttack validates and resolves one server-authoritative basic laser attack.
func (service *Service) ExecuteBasicAttack(input BasicAttackInput) (BasicAttackResult, error) {
	if err := input.validate(); err != nil {
		return BasicAttackResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()

	now := service.clock.Now()
	attacker, target, err := service.validatedAttackActorsLocked(input, now)
	if err != nil {
		return BasicAttackResult{}, err
	}

	cost := attacker.Stats.Stats.Combat.WeaponEnergyCost
	if err := attacker.SpendEnergy(cost); err != nil {
		return BasicAttackResult{}, err
	}
	readyAt, err := attacker.Cooldowns.Start(BasicLaserCooldownKey, now, weaponCooldownDuration(attacker))
	if err != nil {
		return BasicAttackResult{}, err
	}

	result := BasicAttackResult{
		Hit:             rollHit(service.rng, attacker, target),
		CooldownReadyAt: readyAt,
	}
	if result.Hit {
		result.Damage, result.ShieldDamage, result.HPDamage = applyLaserDamage(&target, attacker)
		recordContribution(&target, attacker, result.Damage)
		if target.HP <= 0 && !target.Dead {
			target.Dead = true
			target.HP = 0
			diedAt := now
			target.DiedAt = &diedAt
			if target.Type == world.EntityTypeNPCPlaceholder {
				result.Killed = true
				killEvent := NPCKilledEvent{
					SourceID:      target.EntityID,
					NPCEntityID:   target.EntityID,
					WorldID:       target.WorldID,
					ZoneID:        target.ZoneID,
					Position:      target.Position,
					OwnerPlayerID: highestContributor(target),
					KilledAt:      now,
				}
				result.KillEvent = &killEvent
			}
		}
	}

	service.actors[attacker.EntityID] = cloneActor(attacker)
	service.actors[target.EntityID] = cloneActor(target)

	result.Attacker = cloneActor(attacker)
	result.Target = cloneActor(target)
	emitter = service.emitter
	if emitter != nil {
		emitted = append(emitted, service.newEventLocked(EventBasicAttack, basicAttackPayload(input, result), now))
		if result.KillEvent != nil {
			emitted = append(emitted, service.newEventLocked(EventNPCKilled, result.KillEvent, now))
		}
	}
	return result, nil
}

func (service *Service) validatedAttackActorsLocked(input BasicAttackInput, now time.Time) (ActorState, ActorState, error) {
	attacker, ok := service.actors[input.AttackerID]
	if !ok {
		return ActorState{}, ActorState{}, fmt.Errorf("attacker %q: %w", input.AttackerID, ErrUnknownActor)
	}
	target, ok := service.actors[input.TargetID]
	if !ok {
		return ActorState{}, ActorState{}, fmt.Errorf("target %q: %w", input.TargetID, ErrUnknownActor)
	}
	attacker = cloneActor(attacker)
	target = cloneActor(target)

	if err := attacker.validate(); err != nil {
		return ActorState{}, ActorState{}, err
	}
	if err := target.validate(); err != nil {
		return ActorState{}, ActorState{}, err
	}
	if attacker.Dead || attacker.HP <= 0 {
		return ActorState{}, ActorState{}, ErrAttackerDead
	}
	if target.Dead || target.HP <= 0 {
		return ActorState{}, ActorState{}, ErrTargetDead
	}
	if attacker.WorldID != target.WorldID || attacker.ZoneID != target.ZoneID {
		return ActorState{}, ActorState{}, ErrDifferentWorldZone
	}
	if err := visibility.CanInteract(viewerFromActor(attacker), visibilityEntityFromActor(target)); err != nil {
		return ActorState{}, ActorState{}, ErrTargetNotVisible
	}
	if attacker.Position.Distance(target.Position) > attacker.Stats.Stats.Combat.WeaponRange {
		return ActorState{}, ActorState{}, ErrOutOfRange
	}
	if !attacker.Cooldowns.Ready(BasicLaserCooldownKey, now) {
		return ActorState{}, ActorState{}, ErrCooldownNotReady
	}
	if attacker.Energy < attacker.Stats.Stats.Combat.WeaponEnergyCost {
		return ActorState{}, ActorState{}, ErrNotEnoughEnergy
	}
	return attacker, target, nil
}

func weaponCooldownDuration(attacker ActorState) time.Duration {
	return time.Duration(attacker.Stats.Stats.Combat.WeaponCooldown * float64(time.Second))
}

func rollHit(rng foundation.RNG, attacker ActorState, target ActorState) bool {
	hitChance := attacker.Stats.Stats.Combat.Accuracy + attacker.Stats.Stats.Combat.Tracking - target.Stats.Stats.Combat.Evasion
	if hitChance < 0.05 {
		hitChance = 0.05
	}
	if hitChance > 0.95 {
		hitChance = 0.95
	}
	if rng == nil {
		return true
	}
	return rng.Float64() <= hitChance
}

func applyLaserDamage(target *ActorState, attacker ActorState) (total float64, shieldDamage float64, hpDamage float64) {
	effectiveResist := target.Stats.Stats.Combat.ResistLaser - attacker.Stats.Stats.Combat.Penetration
	if effectiveResist < 0 {
		effectiveResist = 0
	}
	if effectiveResist > 0.95 {
		effectiveResist = 0.95
	}
	total = attacker.Stats.Stats.Combat.WeaponDamage * (1 - effectiveResist)
	if total < 0 {
		total = 0
	}
	shieldDamage = minFloat(target.Shield, total)
	target.Shield -= shieldDamage
	hpDamage = minFloat(target.HP, total-shieldDamage)
	target.HP -= hpDamage
	if target.HP < 0 {
		target.HP = 0
	}
	return total, shieldDamage, hpDamage
}

func recordContribution(target *ActorState, attacker ActorState, damage float64) {
	if attacker.PlayerID.IsZero() || damage <= 0 {
		return
	}
	if target.Contributions == nil {
		target.Contributions = make(map[foundation.PlayerID]float64)
	}
	target.Contributions[attacker.PlayerID] += damage
}

func highestContributor(target ActorState) foundation.PlayerID {
	var bestPlayer foundation.PlayerID
	var bestDamage float64
	for playerID, damage := range target.Contributions {
		if damage > bestDamage || (damage == bestDamage && (bestPlayer == "" || playerID < bestPlayer)) {
			bestPlayer = playerID
			bestDamage = damage
		}
	}
	return bestPlayer
}

func viewerFromActor(actor ActorState) visibility.Viewer {
	return visibility.Viewer{
		WorldID:    actor.WorldID,
		ZoneID:     actor.ZoneID,
		Position:   actor.Position,
		RadarRange: visibility.RadarRangeFromStatSnapshot(actor.Stats),
	}
}

func visibilityEntityFromActor(actor ActorState) visibility.Entity {
	return visibility.Entity{
		WorldID:   actor.WorldID,
		ZoneID:    actor.ZoneID,
		ID:        actor.EntityID,
		Position:  actor.Position,
		Signature: actor.Signature,
		Hidden:    actor.Hidden,
	}
}

func basicAttackPayload(input BasicAttackInput, result BasicAttackResult) any {
	return struct {
		AttackerID world.EntityID `json:"attacker_id"`
		TargetID   world.EntityID `json:"target_id"`
		Hit        bool           `json:"hit"`
		Damage     float64        `json:"damage"`
		Killed     bool           `json:"killed"`
	}{
		AttackerID: input.AttackerID,
		TargetID:   input.TargetID,
		Hit:        result.Hit,
		Damage:     result.Damage,
		Killed:     result.Killed,
	}
}

func (service *Service) newEventLocked(eventType string, payload any, now time.Time) events.EventEnvelope {
	service.nextEventSequence++
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		rawPayload = json.RawMessage(`{}`)
	}
	return events.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("combat-%d", service.nextEventSequence)),
		eventType,
		rawPayload,
		now.UnixMilli(),
		service.nextEventSequence,
	)
}

func emitEvents(emitter EventEmitter, emitted []events.EventEnvelope) {
	if emitter == nil {
		return
	}
	for _, event := range emitted {
		emitter.Record(event)
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

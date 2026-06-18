package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

const CombatSkillBasicLaser = "basic_laser"

var (
	ErrNilCombatService       = errors.New("nil combat service")
	ErrNilCombatActorResolver = errors.New("nil combat actor resolver")
	ErrUnsupportedCombatSkill = errors.New("unsupported combat skill")
)

// CombatActorResolver maps authenticated command context to the active
// server-owned combat actor. It must not read attacker identity from payloads.
type CombatActorResolver interface {
	ActiveCombatActor(realtime.CommandContext) (world.EntityID, error)
}

// CombatCommandHandler adapts realtime combat intents to the combat domain.
type CombatCommandHandler struct {
	combat *combat.Service
	actors CombatActorResolver
}

// NewCombatCommandHandler returns realtime handlers for Phase 05 combat intents.
func NewCombatCommandHandler(service *combat.Service, actors CombatActorResolver) (*CombatCommandHandler, error) {
	if service == nil {
		return nil, ErrNilCombatService
	}
	if actors == nil {
		return nil, ErrNilCombatActorResolver
	}
	return &CombatCommandHandler{
		combat: service,
		actors: actors,
	}, nil
}

// Handlers exposes operation handlers for registration with realtime.Gateway.
func (handler *CombatCommandHandler) Handlers() map[realtime.Operation]realtime.CommandHandler {
	if handler == nil {
		return nil
	}
	return map[realtime.Operation]realtime.CommandHandler{
		realtime.OperationCombatUseSkill: handler.HandleUseSkill,
	}
}

// HandleUseSkill handles the MVP combat.use_skill operation. Client timestamp
// is accepted for client-side correlation but deliberately ignored; cooldowns
// and resolution use CombatService's server clock.
func (handler *CombatCommandHandler) HandleUseSkill(
	ctx realtime.CommandContext,
	request realtime.RequestEnvelope,
) (json.RawMessage, error) {
	payload, err := decodeCombatUseSkillPayload(request.Payload)
	if err != nil {
		return nil, err
	}
	attackerID, err := handler.actors.ActiveCombatActor(ctx)
	if err != nil {
		return nil, domainErrorForCombatCommand(err)
	}
	result, err := handler.combat.ExecuteBasicAttack(combat.BasicAttackInput{
		AttackerID: attackerID,
		TargetID:   payload.TargetID,
	})
	if err != nil {
		return nil, domainErrorForCombatCommand(err)
	}

	response, err := json.Marshal(combatUseSkillResponse{
		SkillID:           payload.SkillID,
		TargetID:          payload.TargetID,
		Hit:               result.Hit,
		Damage:            result.Damage,
		Killed:            result.Killed,
		CooldownReadyAtMS: result.CooldownReadyAt.UTC().UnixMilli(),
	})
	if err != nil {
		return nil, domainErrorForCombatCommand(err)
	}
	return json.RawMessage(response), nil
}

type combatUseSkillPayload struct {
	SkillID         string         `json:"skill_id"`
	TargetID        world.EntityID `json:"target_id"`
	ClientTimestamp int64          `json:"client_timestamp,omitempty"`
}

type combatUseSkillResponse struct {
	SkillID           string         `json:"skill_id"`
	TargetID          world.EntityID `json:"target_id"`
	Hit               bool           `json:"hit"`
	Damage            float64        `json:"damage"`
	Killed            bool           `json:"killed"`
	CooldownReadyAtMS int64          `json:"cooldown_ready_at_ms"`
}

func decodeCombatUseSkillPayload(raw json.RawMessage) (combatUseSkillPayload, error) {
	var payload combatUseSkillPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return combatUseSkillPayload{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid combat skill payload.",
			foundation.WithCause(err),
		)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return combatUseSkillPayload{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid combat skill payload.",
		)
	}
	if strings.TrimSpace(payload.SkillID) == "" {
		return combatUseSkillPayload{}, foundation.NewDomainError(foundation.CodeInvalidPayload, "skill_id is required.")
	}
	if payload.SkillID != CombatSkillBasicLaser {
		return combatUseSkillPayload{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Unsupported combat skill.",
			foundation.WithCause(ErrUnsupportedCombatSkill),
		)
	}
	if err := payload.TargetID.Validate(); err != nil {
		return combatUseSkillPayload{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"target_id is required.",
			foundation.WithCause(err),
		)
	}
	return payload, nil
}

func domainErrorForCombatCommand(err error) error {
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, combat.ErrCooldownNotReady):
		return foundation.NewDomainError(foundation.CodeCooldown, "Skill is on cooldown.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrNotEnoughEnergy):
		return foundation.NewDomainError(foundation.CodeNotEnoughEnergy, "Not enough energy.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrOutOfRange):
		return foundation.NewDomainError(foundation.CodeOutOfRange, "Target is out of range.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrTargetNotVisible):
		return foundation.NewDomainError(foundation.CodeNotVisible, "Target is not visible.", foundation.WithCause(err))
	case errors.Is(err, combat.ErrUnknownActor), errors.Is(err, combat.ErrTargetDead), errors.Is(err, combat.ErrAttackerDead):
		return foundation.NewDomainError(foundation.CodeNotFound, "Target is not available.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Combat command failed.", foundation.WithCause(err))
	}
}

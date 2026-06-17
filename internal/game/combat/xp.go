package combat

import (
	"errors"
	"fmt"

	"gameproject/internal/game/progression"
)

var (
	ErrNilXPGranter       = errors.New("nil xp granter")
	ErrInvalidXPReward    = errors.New("invalid combat xp reward")
	ErrInvalidNPCKillXP   = errors.New("invalid npc kill xp event")
	ErrUnsupportedXPEvent = errors.New("unsupported combat xp event")
)

// XPGranter is the progression boundary used by combat XP handlers.
type XPGranter interface {
	GrantXP(progression.GrantXPInput) (progression.GrantXPResult, error)
}

// NPCKillXPReward describes the configured MVP combat XP payout for one NPC
// kill event.
type NPCKillXPReward struct {
	MainXP int64
	Role   progression.RoleType
	RoleXP int64
}

// DefaultNPCKillXPReward returns the Phase 05 MVP combat payout.
func DefaultNPCKillXPReward() NPCKillXPReward {
	return NPCKillXPReward{
		MainXP: 20,
		Role:   progression.RoleTypeCombat,
		RoleXP: 20,
	}
}

// NPCKillXPHandler applies combat XP for authoritative NPC kill events.
type NPCKillXPHandler struct {
	granter XPGranter
	reward  NPCKillXPReward
}

// NewNPCKillXPHandler returns an idempotent combat XP handler.
func NewNPCKillXPHandler(granter XPGranter, reward NPCKillXPReward) (*NPCKillXPHandler, error) {
	if granter == nil {
		return nil, ErrNilXPGranter
	}
	if err := reward.validate(); err != nil {
		return nil, err
	}
	return &NPCKillXPHandler{
		granter: granter,
		reward:  reward,
	}, nil
}

// GrantNPCKillXP grants combat XP once for event.SourceID/NPCEntityID.
func (handler *NPCKillXPHandler) GrantNPCKillXP(event NPCKilledEvent) (progression.GrantXPResult, error) {
	if err := event.validateForXP(); err != nil {
		return progression.GrantXPResult{}, err
	}
	return handler.granter.GrantXP(progression.GrantXPInput{
		PlayerID:       event.OwnerPlayerID,
		Amount:         handler.reward.MainXP,
		SourceType:     progression.XPSourceTypeCombat,
		SourceID:       progression.XPSourceID(event.NPCEntityID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("combat_kill:" + event.NPCEntityID.String()),
		RoleXP: []progression.RoleXPGrant{{
			Role:   handler.reward.Role,
			Amount: handler.reward.RoleXP,
		}},
	})
}

func (reward NPCKillXPReward) validate() error {
	if reward.MainXP <= 0 {
		return fmt.Errorf("main_xp %d: %w", reward.MainXP, ErrInvalidXPReward)
	}
	if err := reward.Role.Validate(); err != nil {
		return fmt.Errorf("role: %w", err)
	}
	if reward.RoleXP <= 0 {
		return fmt.Errorf("role_xp %d: %w", reward.RoleXP, ErrInvalidXPReward)
	}
	return nil
}

func (event NPCKilledEvent) validateForXP() error {
	if err := event.NPCEntityID.Validate(); err != nil {
		return fmt.Errorf("npc_entity_id: %w", err)
	}
	if event.SourceID != "" && event.SourceID != event.NPCEntityID {
		return fmt.Errorf("source_id %q npc_entity_id %q: %w", event.SourceID, event.NPCEntityID, ErrUnsupportedXPEvent)
	}
	if err := event.OwnerPlayerID.Validate(); err != nil {
		return fmt.Errorf("owner_player_id: %w", err)
	}
	if event.KilledAt.IsZero() {
		return fmt.Errorf("killed_at: %w", ErrInvalidNPCKillXP)
	}
	return nil
}

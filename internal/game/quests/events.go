package quests

import (
	"fmt"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// QuestProgressEventKey is the durable idempotency identity for applying one
// server-owned domain event to quest progress.
type QuestProgressEventKey string

// CombatNPCKilledInput is the server-owned combat.npc_killed event shape that
// may progress kill objectives.
type CombatNPCKilledInput struct {
	EventID          foundation.EventID    `json:"event_id"`
	ProgressEventKey QuestProgressEventKey `json:"-"`
	PlayerID         foundation.PlayerID   `json:"player_id"`
	NPCType          string                `json:"npc_type"`
}

// LootPickedUpInput is the server-owned loot.picked_up event shape that may
// progress collect objectives.
type LootPickedUpInput struct {
	EventID          foundation.EventID    `json:"event_id"`
	ProgressEventKey QuestProgressEventKey `json:"-"`
	PlayerID         foundation.PlayerID   `json:"player_id"`
	ItemID           foundation.ItemID     `json:"item_id"`
	Quantity         foundation.Quantity   `json:"quantity"`
}

// CraftJobCompletedInput is the server-owned craft.job_completed event shape
// that may progress craft objectives.
type CraftJobCompletedInput struct {
	EventID          foundation.EventID    `json:"event_id"`
	ProgressEventKey QuestProgressEventKey `json:"-"`
	PlayerID         foundation.PlayerID   `json:"player_id"`
	RecipeID         catalog.DefinitionID  `json:"recipe_id,omitempty"`
	ItemID           foundation.ItemID     `json:"item_id,omitempty"`
	Quantity         foundation.Quantity   `json:"quantity"`
}

// ScanCompletedInput is the server-owned scanner skeleton event shape. The
// consumer validates it but intentionally no-ops until the scanner provider is
// authoritative.
type ScanCompletedInput struct {
	EventID          foundation.EventID  `json:"event_id"`
	PlayerID         foundation.PlayerID `json:"player_id"`
	TargetSignalType string              `json:"target_signal_type"`
}

// BuildingCompletedInput is the server-owned building skeleton event shape.
// The consumer validates it but intentionally no-ops until building ownership
// and completion providers exist.
type BuildingCompletedInput struct {
	EventID      foundation.EventID  `json:"event_id"`
	PlayerID     foundation.PlayerID `json:"player_id"`
	BuildingType string              `json:"building_type"`
}

// DeliveryCompletedInput is the server-owned delivery skeleton event shape.
// The consumer validates it but intentionally no-ops until delivery settlement
// providers exist.
type DeliveryCompletedInput struct {
	EventID         foundation.EventID  `json:"event_id"`
	PlayerID        foundation.PlayerID `json:"player_id"`
	ItemID          foundation.ItemID   `json:"item_id"`
	Quantity        foundation.Quantity `json:"quantity"`
	DestinationType string              `json:"destination_type"`
	DestinationID   string              `json:"destination_id,omitempty"`
}

// Validate reports whether input names one authoritative NPC kill event.
func (input CombatNPCKilledInput) Validate() error {
	if err := validateQuestEventEnvelope(input.EventID, input.PlayerID); err != nil {
		return err
	}
	if err := input.ProgressEventKey.ValidateOptional(); err != nil {
		return err
	}
	if strings.TrimSpace(input.NPCType) == "" {
		return fmt.Errorf("npc_type: %w", ErrInvalidQuestEvent)
	}
	return nil
}

// Validate reports whether input names one authoritative loot pickup event.
func (input LootPickedUpInput) Validate() error {
	if err := validateQuestEventEnvelope(input.EventID, input.PlayerID); err != nil {
		return err
	}
	if err := input.ProgressEventKey.ValidateOptional(); err != nil {
		return err
	}
	if err := input.ItemID.Validate(); err != nil {
		return fmt.Errorf("loot item: %w", err)
	}
	if err := input.Quantity.Validate(); err != nil {
		return fmt.Errorf("loot quantity: %w", err)
	}
	return nil
}

// Validate reports whether input names one authoritative craft completion
// event. A recipe id, output item id, or both may identify the completed work.
func (input CraftJobCompletedInput) Validate() error {
	if err := validateQuestEventEnvelope(input.EventID, input.PlayerID); err != nil {
		return err
	}
	if err := input.ProgressEventKey.ValidateOptional(); err != nil {
		return err
	}
	if input.RecipeID.IsZero() && input.ItemID.IsZero() {
		return fmt.Errorf("craft target: %w", ErrInvalidQuestEvent)
	}
	if !input.RecipeID.IsZero() {
		if err := input.RecipeID.Validate(); err != nil {
			return fmt.Errorf("craft recipe: %w", err)
		}
	}
	if !input.ItemID.IsZero() {
		if err := input.ItemID.Validate(); err != nil {
			return fmt.Errorf("craft item: %w", err)
		}
	}
	if err := input.Quantity.Validate(); err != nil {
		return fmt.Errorf("craft quantity: %w", err)
	}
	return nil
}

// Validate reports whether input names one server scanner event.
func (input ScanCompletedInput) Validate() error {
	if err := validateQuestEventEnvelope(input.EventID, input.PlayerID); err != nil {
		return err
	}
	if strings.TrimSpace(input.TargetSignalType) == "" {
		return fmt.Errorf("target_signal_type: %w", ErrInvalidQuestEvent)
	}
	return nil
}

// Validate reports whether input names one server building completion event.
func (input BuildingCompletedInput) Validate() error {
	if err := validateQuestEventEnvelope(input.EventID, input.PlayerID); err != nil {
		return err
	}
	if strings.TrimSpace(input.BuildingType) == "" {
		return fmt.Errorf("building_type: %w", ErrInvalidQuestEvent)
	}
	return nil
}

// Validate reports whether input names one server delivery settlement event.
func (input DeliveryCompletedInput) Validate() error {
	if err := validateQuestEventEnvelope(input.EventID, input.PlayerID); err != nil {
		return err
	}
	if err := input.ItemID.Validate(); err != nil {
		return fmt.Errorf("delivery item: %w", err)
	}
	if err := input.Quantity.Validate(); err != nil {
		return fmt.Errorf("delivery quantity: %w", err)
	}
	if strings.TrimSpace(input.DestinationType) == "" {
		return fmt.Errorf("destination_type: %w", ErrInvalidQuestEvent)
	}
	return nil
}

// String returns the stable quest progress idempotency identity.
func (key QuestProgressEventKey) String() string {
	return string(key)
}

// IsZero reports whether key is absent. Direct event consumer calls may omit
// it and fall back to the validated event id.
func (key QuestProgressEventKey) IsZero() bool {
	return key == ""
}

// Validate reports whether key is usable as an internal quest progress
// idempotency identity.
func (key QuestProgressEventKey) Validate() error {
	if strings.TrimSpace(string(key)) == "" {
		return fmt.Errorf("progress_event_key: %w", ErrInvalidQuestEvent)
	}
	if strings.TrimSpace(string(key)) != string(key) {
		return fmt.Errorf("progress_event_key %q: %w", key, ErrInvalidQuestEvent)
	}
	return nil
}

// ValidateOptional validates key only when an authoritative router supplied
// one. Empty keys fall back to the validated envelope id.
func (key QuestProgressEventKey) ValidateOptional() error {
	if key.IsZero() {
		return nil
	}
	return key.Validate()
}

func questProgressEventKey(eventID foundation.EventID, progressKey QuestProgressEventKey) QuestProgressEventKey {
	if !progressKey.IsZero() {
		return progressKey
	}
	return QuestProgressEventKey(eventID.String())
}

func validateQuestEventEnvelope(eventID foundation.EventID, playerID foundation.PlayerID) error {
	if err := eventID.Validate(); err != nil {
		return fmt.Errorf("event_id: %w", err)
	}
	if err := playerID.Validate(); err != nil {
		return fmt.Errorf("player_id: %w", err)
	}
	return nil
}

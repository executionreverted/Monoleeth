package quests

import (
	"fmt"
	"reflect"
	"time"

	"gameproject/internal/game/foundation"
)

// NewGeneratedBoardOffer stores server-generated target and reward payloads for
// an offer. Rewards are attached before acceptance to avoid reroll/claim
// manipulation.
func NewGeneratedBoardOffer(
	offerID foundation.QuestID,
	playerID foundation.PlayerID,
	template QuestTemplate,
	generatedPayload GeneratedPayload,
	rewardPayload RewardPayload,
	createdAt time.Time,
	expiresAt time.Time,
) (GeneratedBoardOffer, error) {
	if err := template.Validate(); err != nil {
		return GeneratedBoardOffer{}, err
	}
	if generatedPayload.Objective.IsZero() {
		generatedPayload.Objective = template.ObjectiveSchema
	} else if !reflect.DeepEqual(generatedPayload.Objective, template.ObjectiveSchema) {
		return GeneratedBoardOffer{}, fmt.Errorf("generated objective does not match template: %w", ErrObjectivePayloadMismatch)
	}
	offer := GeneratedBoardOffer{
		OfferID:          offerID,
		PlayerID:         playerID,
		TemplateSource:   sourceForTemplate(template),
		TemplateID:       template.TemplateID,
		Type:             template.Type,
		GeneratedPayload: generatedPayload,
		RewardPayload:    rewardPayload,
		CreatedAt:        createdAt,
		ExpiresAt:        expiresAt,
	}
	if err := offer.Validate(); err != nil {
		return GeneratedBoardOffer{}, err
	}
	return offer, nil
}

// NewAcceptedPlayerQuest creates durable accepted quest state from an offer.
func NewAcceptedPlayerQuest(
	playerQuestID foundation.QuestID,
	offer GeneratedBoardOffer,
	schema ObjectiveSchema,
	acceptedAt time.Time,
	expiresAt *time.Time,
) (PlayerQuest, error) {
	if err := offer.Validate(); err != nil {
		return PlayerQuest{}, err
	}
	if schema.IsZero() {
		schema = offer.GeneratedPayload.Objective
	} else if !reflect.DeepEqual(schema, offer.GeneratedPayload.Objective) {
		return PlayerQuest{}, fmt.Errorf("accepted objective does not match generated payload: %w", ErrObjectivePayloadMismatch)
	}
	progress, err := NewQuestProgressFromSchema(schema)
	if err != nil {
		return PlayerQuest{}, err
	}
	quest := PlayerQuest{
		PlayerQuestID:     playerQuestID,
		PlayerID:          offer.PlayerID,
		TemplateSource:    offer.TemplateSource,
		TemplateID:        offer.TemplateID,
		Type:              offer.Type,
		GeneratedPayload:  offer.GeneratedPayload,
		RewardPayload:     offer.RewardPayload,
		State:             QuestStateAccepted,
		Progress:          progress,
		AcceptedAt:        acceptedAt,
		ExpiresAt:         cloneTimePtr(expiresAt),
		RewardReferenceID: RewardReferenceForPlayerQuest(playerQuestID),
	}
	if err := quest.ValidateAgainst(schema); err != nil {
		return PlayerQuest{}, err
	}
	return quest, nil
}

// RewardReferenceForPlayerQuest returns the required idempotency reference used
// by later reward services.
func RewardReferenceForPlayerQuest(playerQuestID foundation.QuestID) string {
	return fmt.Sprintf("quest_reward:%s", playerQuestID)
}

// ValidateTransition reports whether this player quest may move to next.
func (quest PlayerQuest) ValidateTransition(next QuestState) error {
	return quest.State.ValidateTransition(next)
}

// Validate reports whether quest is valid against its generated objective
// payload.
func (quest PlayerQuest) Validate() error {
	return quest.ValidateAgainst(quest.GeneratedPayload.Objective)
}

// Validate reports whether offer has durable ids, generated payload, reward
// payload, and a valid expiry window.
func (offer GeneratedBoardOffer) Validate() error {
	if err := offer.OfferID.Validate(); err != nil {
		return err
	}
	if err := offer.PlayerID.Validate(); err != nil {
		return err
	}
	if err := offer.TemplateSource.Validate(); err != nil {
		return err
	}
	if err := offer.TemplateID.Validate(); err != nil {
		return err
	}
	if offer.TemplateSource.DefinitionID != offer.TemplateID {
		return fmt.Errorf("source %q template %q: %w", offer.TemplateSource.DefinitionID, offer.TemplateID, ErrQuestSourceMismatch)
	}
	if err := offer.Type.Validate(); err != nil {
		return err
	}
	if err := offer.GeneratedPayload.Validate(); err != nil {
		return err
	}
	if err := offer.RewardPayload.Validate(); err != nil {
		return err
	}
	if offer.CreatedAt.IsZero() || offer.ExpiresAt.IsZero() {
		return ErrZeroQuestTime
	}
	if !offer.ExpiresAt.After(offer.CreatedAt) {
		return fmt.Errorf("offer expires_at %s created_at %s: %w", offer.ExpiresAt, offer.CreatedAt, ErrInvalidQuestTime)
	}
	if offer.AcceptedAt != nil && offer.AcceptedAt.Before(offer.CreatedAt) {
		return fmt.Errorf("offer accepted_at %s before created_at %s: %w", *offer.AcceptedAt, offer.CreatedAt, ErrInvalidQuestTime)
	}
	return nil
}

// ValidateAgainst reports whether accepted quest state matches its objective schema.
func (quest PlayerQuest) ValidateAgainst(schema ObjectiveSchema) error {
	if err := quest.PlayerQuestID.Validate(); err != nil {
		return err
	}
	if err := quest.PlayerID.Validate(); err != nil {
		return err
	}
	if err := quest.TemplateSource.Validate(); err != nil {
		return err
	}
	if err := quest.TemplateID.Validate(); err != nil {
		return err
	}
	if quest.TemplateSource.DefinitionID != quest.TemplateID {
		return fmt.Errorf("source %q template %q: %w", quest.TemplateSource.DefinitionID, quest.TemplateID, ErrQuestSourceMismatch)
	}
	if err := quest.Type.Validate(); err != nil {
		return err
	}
	if err := quest.GeneratedPayload.Validate(); err != nil {
		return err
	}
	if err := quest.RewardPayload.Validate(); err != nil {
		return err
	}
	if err := quest.State.Validate(); err != nil {
		return err
	}
	if quest.State == QuestStateOffered {
		return fmt.Errorf("player quest state %q: %w", quest.State, ErrInvalidQuestState)
	}
	if quest.AcceptedAt.IsZero() {
		return fmt.Errorf("accepted_at: %w", ErrZeroQuestTime)
	}
	if quest.ExpiresAt != nil && !quest.ExpiresAt.After(quest.AcceptedAt) {
		return fmt.Errorf("expires_at %s accepted_at %s: %w", *quest.ExpiresAt, quest.AcceptedAt, ErrAcceptedQuestExpiresTooEarly)
	}
	if err := quest.Progress.ValidateAgainst(schema); err != nil {
		return err
	}
	if err := quest.validateStateTimestamps(); err != nil {
		return err
	}
	expectedReference := RewardReferenceForPlayerQuest(quest.PlayerQuestID)
	if quest.RewardReferenceID != "" && quest.RewardReferenceID != expectedReference {
		return fmt.Errorf("reward reference %q want %q: %w", quest.RewardReferenceID, expectedReference, ErrInvalidQuestClaim)
	}
	return nil
}

func (quest PlayerQuest) validateStateTimestamps() error {
	if quest.CompletedAt != nil && quest.CompletedAt.Before(quest.AcceptedAt) {
		return fmt.Errorf("completed_at %s before accepted_at %s: %w", *quest.CompletedAt, quest.AcceptedAt, ErrInvalidQuestTime)
	}
	claimedAt, err := quest.claimedAt()
	if err != nil {
		return err
	}
	if claimedAt != nil {
		if quest.CompletedAt == nil {
			return fmt.Errorf("claimed_at without completed_at: %w", ErrInvalidQuestClaim)
		}
		if claimedAt.Before(*quest.CompletedAt) {
			return fmt.Errorf("claimed_at %s before completed_at %s: %w", *claimedAt, *quest.CompletedAt, ErrInvalidQuestTime)
		}
	}
	switch quest.State {
	case QuestStateAccepted:
		if quest.CompletedAt != nil || claimedAt != nil {
			return fmt.Errorf("accepted quest has completion or claim time: %w", ErrInvalidQuestTime)
		}
		if quest.Progress.Complete() {
			return fmt.Errorf("accepted quest progress complete: %w", ErrInvalidQuestCompletion)
		}
	case QuestStateCompleted:
		if quest.CompletedAt == nil || quest.CompletedAt.IsZero() {
			return fmt.Errorf("completed_at: %w", ErrZeroQuestTime)
		}
		if claimedAt != nil {
			return fmt.Errorf("completed quest already claimed: %w", ErrInvalidQuestClaim)
		}
		if !quest.Progress.Complete() {
			return fmt.Errorf("completed quest progress incomplete: %w", ErrInvalidQuestProgress)
		}
	case QuestStateClaimed:
		if quest.CompletedAt == nil || quest.CompletedAt.IsZero() {
			return fmt.Errorf("completed_at: %w", ErrZeroQuestTime)
		}
		if claimedAt == nil || claimedAt.IsZero() {
			return fmt.Errorf("claimed_at: %w", ErrZeroQuestTime)
		}
		if !quest.Progress.Complete() {
			return fmt.Errorf("claimed quest progress incomplete: %w", ErrInvalidQuestProgress)
		}
	case QuestStateExpired, QuestStateAbandoned:
		if claimedAt != nil {
			return fmt.Errorf("terminal quest has claim time: %w", ErrInvalidQuestClaim)
		}
	}
	return nil
}

func (quest PlayerQuest) claimedAt() (*time.Time, error) {
	switch {
	case quest.ClaimedAt == nil:
		return quest.RewardClaimedAt, nil
	case quest.RewardClaimedAt == nil:
		return quest.ClaimedAt, nil
	case !quest.ClaimedAt.Equal(*quest.RewardClaimedAt):
		return nil, fmt.Errorf("claimed_at %s reward_claimed_at %s: %w", *quest.ClaimedAt, *quest.RewardClaimedAt, ErrInvalidQuestTime)
	default:
		return quest.ClaimedAt, nil
	}
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

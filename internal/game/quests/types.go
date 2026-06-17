package quests

import (
	"encoding/json"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// QuestType identifies the high-level board activity.
type QuestType string

const (
	QuestTypeKill    QuestType = "kill"
	QuestTypeCollect QuestType = "collect"
	QuestTypeCraft   QuestType = "craft"
	QuestTypeScan    QuestType = "scan"
	QuestTypeBuild   QuestType = "build"
	QuestTypeDeliver QuestType = "deliver"
)

// QuestState records the durable lifecycle of an offered or accepted quest.
type QuestState string

const (
	QuestStateOffered   QuestState = "offered"
	QuestStateAccepted  QuestState = "accepted"
	QuestStateCompleted QuestState = "completed"
	QuestStateClaimed   QuestState = "claimed"
	QuestStateExpired   QuestState = "expired"
	QuestStateAbandoned QuestState = "abandoned"
)

// ObjectiveKind identifies an MVP objective schema shape.
type ObjectiveKind string

const (
	ObjectiveKindKill    ObjectiveKind = "kill"
	ObjectiveKindCollect ObjectiveKind = "collect"
	ObjectiveKindCraft   ObjectiveKind = "craft"
	ObjectiveKindScan    ObjectiveKind = "scan"
	ObjectiveKindBuild   ObjectiveKind = "build"
	ObjectiveKindDeliver ObjectiveKind = "deliver"
)

// ScanTargetKind is the public skeleton target family for scan objectives.
type ScanTargetKind string

const (
	ScanTargetSignal ScanTargetKind = "signal"
	ScanTargetPlanet ScanTargetKind = "planet"
)

// DeliveryTargetKind is the public skeleton destination family for delivery objectives.
type DeliveryTargetKind string

const (
	DeliveryTargetStation DeliveryTargetKind = "station"
	DeliveryTargetPlanet  DeliveryTargetKind = "planet"
)

// RewardKind identifies a reward grant that ClaimReward applies through the
// owning wallet, inventory, or progression boundary.
type RewardKind string

const (
	RewardKindCredits RewardKind = "credits"
	RewardKindItem    RewardKind = "item"
	RewardKindMainXP  RewardKind = "main_xp"
	RewardKindRoleXP  RewardKind = "role_xp"
)

// RewardHookKind names server-side cap/rarity hooks stored with generated
// rewards. Hooks are policy markers only; they do not grant value.
type RewardHookKind string

const (
	RewardHookRareOfferCap RewardHookKind = "rare_offer_cap"
	RewardHookXCoreCap     RewardHookKind = "x_core_cap"
	RewardHookPremiumCap   RewardHookKind = "premium_cap"

	RewardHookRareCap = RewardHookRareOfferCap
	RewardHookXCore   = RewardHookXCoreCap
	RewardHookPremium = RewardHookPremiumCap
)

// QuestRequirement stores rank/role gates for catalog templates.
type QuestRequirement struct {
	MinRank   int                  `json:"min_rank,omitempty"`
	MaxRank   int                  `json:"max_rank,omitempty"`
	Role      progression.RoleType `json:"role,omitempty"`
	RoleLevel int                  `json:"role_level,omitempty"`
}

// QuestTemplate is one static quest catalog row.
type QuestTemplate struct {
	Source          catalog.VersionedDefinition `json:"source"`
	TemplateID      catalog.DefinitionID        `json:"template_id"`
	Type            QuestType                   `json:"quest_type"`
	TitleKey        string                      `json:"title_key"`
	DescriptionKey  string                      `json:"description_key"`
	DifficultyRules json.RawMessage             `json:"difficulty_rules_json,omitempty"`
	ObjectiveSchema ObjectiveSchema             `json:"objective_schema_json"`
	RewardRules     json.RawMessage             `json:"reward_rules_json,omitempty"`
	ExpirationRules json.RawMessage             `json:"expiration_rules_json,omitempty"`
	Requirements    []QuestRequirement          `json:"requirements_json,omitempty"`
}

// GeneratedPayload stores server-generated target metadata for one board offer.
// It is intentionally generic so generation can vary targets without leaking
// hidden world seeds or accepting client-authored progress.
type GeneratedPayload struct {
	Seed         int64           `json:"seed,omitempty"`
	Objective    ObjectiveSchema `json:"objective,omitempty"`
	Difficulty   int             `json:"difficulty,omitempty"`
	TargetRegion string          `json:"target_region,omitempty"`
	MetadataJSON json.RawMessage `json:"metadata_json,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
}

// GeneratedBoardOffer is a durable board offer before acceptance.
type GeneratedBoardOffer struct {
	OfferID          foundation.QuestID          `json:"offer_id"`
	PlayerID         foundation.PlayerID         `json:"player_id"`
	TemplateSource   catalog.VersionedDefinition `json:"template_source"`
	TemplateID       catalog.DefinitionID        `json:"template_id"`
	Type             QuestType                   `json:"quest_type"`
	GeneratedPayload GeneratedPayload            `json:"generated_payload_json"`
	RewardPayload    RewardPayload               `json:"reward_payload_json"`
	CreatedAt        time.Time                   `json:"created_at"`
	ExpiresAt        time.Time                   `json:"expires_at"`
	AcceptedAt       *time.Time                  `json:"accepted_at,omitempty"`
}

// QuestOffer is the shorter roadmap name for a generated board offer.
type QuestOffer = GeneratedBoardOffer

// PlayerQuest is an accepted quest owned by one player.
type PlayerQuest struct {
	PlayerQuestID     foundation.QuestID          `json:"player_quest_id"`
	PlayerID          foundation.PlayerID         `json:"player_id"`
	TemplateSource    catalog.VersionedDefinition `json:"template_source"`
	TemplateID        catalog.DefinitionID        `json:"template_id"`
	Type              QuestType                   `json:"quest_type"`
	GeneratedPayload  GeneratedPayload            `json:"generated_payload_json"`
	RewardPayload     RewardPayload               `json:"reward_payload_json"`
	State             QuestState                  `json:"state"`
	Progress          QuestProgress               `json:"progress_json"`
	AcceptedAt        time.Time                   `json:"accepted_at"`
	ExpiresAt         *time.Time                  `json:"expires_at,omitempty"`
	CompletedAt       *time.Time                  `json:"completed_at,omitempty"`
	ClaimedAt         *time.Time                  `json:"claimed_at,omitempty"`
	RewardClaimedAt   *time.Time                  `json:"reward_claimed_at,omitempty"`
	RewardReferenceID string                      `json:"reward_reference_id,omitempty"`
}

// ObjectiveSchema stores the server-owned objective definitions for a template
// or accepted/generated quest.
type ObjectiveSchema struct {
	Objectives []Objective              `json:"objectives,omitempty"`
	Kind       ObjectiveKind            `json:"kind,omitempty"`
	Kill       *KillObjectiveDetails    `json:"kill,omitempty"`
	Collect    *CollectObjectiveDetails `json:"collect,omitempty"`
	Craft      *CraftObjectiveDetails   `json:"craft,omitempty"`
	Scan       *ScanObjectiveDetails    `json:"scan,omitempty"`
	Build      *BuildObjectiveDetails   `json:"build,omitempty"`
	Deliver    *DeliverObjectiveDetails `json:"deliver,omitempty"`
}

// Objective stores one objective. Exactly one detail pointer must be populated
// and it must match Kind.
type Objective struct {
	ID      string            `json:"id"`
	Kind    ObjectiveKind     `json:"kind"`
	Kill    *KillObjective    `json:"kill,omitempty"`
	Collect *CollectObjective `json:"collect,omitempty"`
	Craft   *CraftObjective   `json:"craft,omitempty"`
	Scan    *ScanObjective    `json:"scan,omitempty"`
	Build   *BuildObjective   `json:"build,omitempty"`
	Deliver *DeliverObjective `json:"deliver,omitempty"`
}

// KillObjective is progressed only from validated combat kill events.
type KillObjective struct {
	TargetNPCType string              `json:"target_npc_type"`
	RequiredCount foundation.Quantity `json:"required_count"`
}

// CollectObjective is progressed only from validated loot/inventory events.
type CollectObjective struct {
	ItemID   foundation.ItemID   `json:"item_id"`
	Quantity foundation.Quantity `json:"quantity"`
}

// CraftObjective is progressed only from validated craft completion events.
type CraftObjective struct {
	RecipeID catalog.DefinitionID `json:"recipe_id,omitempty"`
	ItemID   foundation.ItemID    `json:"item_id,omitempty"`
	Quantity foundation.Quantity  `json:"quantity"`
}

// ScanObjective is an MVP skeleton for server-validated scan events.
type ScanObjective struct {
	TargetSignalType string              `json:"target_signal_type,omitempty"`
	RequiredCount    foundation.Quantity `json:"required_count"`
}

// BuildObjective is an MVP skeleton for server-validated building completion.
type BuildObjective struct {
	BuildingType  string              `json:"building_type"`
	RequiredCount foundation.Quantity `json:"required_count"`
}

// DeliverObjective is an MVP skeleton for server-validated delivery settlement.
type DeliverObjective struct {
	ItemID          foundation.ItemID   `json:"item_id"`
	Quantity        foundation.Quantity `json:"quantity"`
	DestinationType string              `json:"destination_type"`
	DestinationID   string              `json:"destination_id,omitempty"`
}

// KillObjectiveDetails is the single-objective kill schema shape.
type KillObjectiveDetails struct {
	NPCType       string `json:"npc_type"`
	RequiredCount int64  `json:"required_count"`
}

// CollectObjectiveDetails is the single-objective collect schema shape.
type CollectObjectiveDetails struct {
	ItemID           foundation.ItemID `json:"item_id"`
	RequiredQuantity int64             `json:"required_quantity"`
}

// CraftObjectiveDetails is the single-objective craft schema shape.
type CraftObjectiveDetails struct {
	RecipeID      catalog.DefinitionID `json:"recipe_id,omitempty"`
	ItemID        foundation.ItemID    `json:"item_id,omitempty"`
	RequiredCount int64                `json:"required_count"`
}

// ScanObjectiveDetails is the single-objective scan skeleton shape.
type ScanObjectiveDetails struct {
	TargetKind    ScanTargetKind `json:"target_kind"`
	RequiredCount int64          `json:"required_count"`
}

// BuildObjectiveDetails is the single-objective build skeleton shape.
type BuildObjectiveDetails struct {
	BuildingID    string `json:"building_id"`
	RequiredCount int64  `json:"required_count"`
}

// DeliverObjectiveDetails is the single-objective delivery skeleton shape.
type DeliverObjectiveDetails struct {
	ItemID           foundation.ItemID  `json:"item_id"`
	RequiredQuantity int64              `json:"required_quantity"`
	DestinationKind  DeliveryTargetKind `json:"destination_kind"`
	DestinationID    string             `json:"destination_id,omitempty"`
}

// ObjectiveProgress stores server-owned progress for one objective.
type ObjectiveProgress struct {
	ObjectiveID string `json:"objective_id"`
	Current     int64  `json:"current"`
	Required    int64  `json:"required"`
	Completed   bool   `json:"completed"`
}

// QuestProgress stores the progress rows for an accepted quest.
type QuestProgress struct {
	Objectives []ObjectiveProgress `json:"objectives"`
}

// RewardPayload stores all generated rewards for an offer or accepted quest.
// Later services must apply each grant idempotently with
// quest_reward:<player_quest_id> as the domain reference.
type RewardPayload struct {
	Grants       []RewardGrant `json:"grants,omitempty"`
	RareCapHooks []RewardHook  `json:"rare_cap_hooks,omitempty"`
	Hooks        []RewardHook  `json:"hooks,omitempty"`
}

// RewardGrant describes one concrete reward output generated upfront.
type RewardGrant struct {
	Kind     RewardKind             `json:"kind"`
	Currency economy.CurrencyBucket `json:"currency,omitempty"`
	ItemID   foundation.ItemID      `json:"item_id,omitempty"`
	Role     progression.RoleType   `json:"role,omitempty"`
	Amount   int64                  `json:"amount"`
}

// RewardHook is a placeholder for later rare reward cap and policy checks.
type RewardHook struct {
	Kind  RewardHookKind `json:"kind"`
	Key   string         `json:"key,omitempty"`
	Limit int64          `json:"limit,omitempty"`
}

func (questType QuestType) String() string { return string(questType) }

func (state QuestState) String() string { return string(state) }

func (kind ObjectiveKind) String() string { return string(kind) }

func (kind ScanTargetKind) String() string { return string(kind) }

func (kind DeliveryTargetKind) String() string { return string(kind) }

func (kind RewardKind) String() string { return string(kind) }

func (kind RewardHookKind) String() string { return string(kind) }

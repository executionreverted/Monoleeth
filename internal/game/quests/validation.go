package quests

import (
	"encoding/json"
	"fmt"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// Validate reports whether questType is supported by the Phase 07 MVP model.
func (questType QuestType) Validate() error {
	switch questType {
	case QuestTypeKill, QuestTypeCollect, QuestTypeCraft, QuestTypeScan, QuestTypeBuild, QuestTypeDeliver:
		return nil
	default:
		return fmt.Errorf("quest type %q: %w", questType, ErrInvalidQuestType)
	}
}

// Validate reports whether state is a supported quest lifecycle state.
func (state QuestState) Validate() error {
	switch state {
	case QuestStateOffered,
		QuestStateAccepted,
		QuestStateCompleted,
		QuestStateClaimed,
		QuestStateExpired,
		QuestStateAbandoned:
		return nil
	default:
		return fmt.Errorf("quest state %q: %w", state, ErrInvalidQuestState)
	}
}

// CanTransitionTo reports whether a quest state transition is valid.
func (state QuestState) CanTransitionTo(next QuestState) bool {
	if err := state.Validate(); err != nil {
		return false
	}
	if err := next.Validate(); err != nil {
		return false
	}
	switch state {
	case QuestStateOffered:
		return next == QuestStateAccepted || next == QuestStateExpired
	case QuestStateAccepted:
		return next == QuestStateCompleted || next == QuestStateExpired || next == QuestStateAbandoned
	case QuestStateCompleted:
		return next == QuestStateClaimed
	case QuestStateClaimed, QuestStateExpired, QuestStateAbandoned:
		return false
	default:
		return false
	}
}

// ValidateTransition reports whether the transition is supported by the model.
func (state QuestState) ValidateTransition(next QuestState) error {
	if !state.CanTransitionTo(next) {
		return fmt.Errorf("%q -> %q: %w", state, next, ErrInvalidQuestStateTransition)
	}
	return nil
}

// Validate reports whether kind is one of the MVP objective schema kinds.
func (kind ObjectiveKind) Validate() error {
	switch kind {
	case ObjectiveKindKill, ObjectiveKindCollect, ObjectiveKindCraft, ObjectiveKindScan, ObjectiveKindBuild, ObjectiveKindDeliver:
		return nil
	default:
		return fmt.Errorf("objective kind %q: %w", kind, ErrInvalidObjectiveKind)
	}
}

// Validate reports whether kind is a supported MVP scan target.
func (kind ScanTargetKind) Validate() error {
	switch kind {
	case ScanTargetSignal, ScanTargetPlanet:
		return nil
	default:
		return fmt.Errorf("scan target kind %q: %w", kind, ErrInvalidScanTargetKind)
	}
}

// Validate reports whether kind is a supported MVP delivery destination.
func (kind DeliveryTargetKind) Validate() error {
	switch kind {
	case DeliveryTargetStation, DeliveryTargetPlanet:
		return nil
	default:
		return fmt.Errorf("delivery target kind %q: %w", kind, ErrInvalidDeliveryTargetKind)
	}
}

// Validate reports whether template is a complete static quest catalog row.
func (template QuestTemplate) Validate() error {
	if err := template.Source.Validate(); err != nil {
		return err
	}
	if err := template.TemplateID.Validate(); err != nil {
		return err
	}
	if template.Source.DefinitionID != template.TemplateID {
		return fmt.Errorf("source %q template %q: %w", template.Source.DefinitionID, template.TemplateID, ErrQuestSourceMismatch)
	}
	if err := template.Type.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(template.TitleKey) == "" {
		return fmt.Errorf("title key: %w", ErrEmptyQuestTextKey)
	}
	if strings.TrimSpace(template.DescriptionKey) == "" {
		return fmt.Errorf("description key: %w", ErrEmptyQuestTextKey)
	}
	if err := template.ObjectiveSchema.Validate(); err != nil {
		return err
	}
	if template.BoardWeight < 0 {
		return fmt.Errorf("board weight %d: %w", template.BoardWeight, ErrInvalidQuestRequirement)
	}
	if template.RewardPayload != nil {
		if err := template.RewardPayload.Validate(); err != nil {
			return err
		}
	}
	for _, requirement := range template.Requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
	}
	if err := validateRawJSON("difficulty rules json", template.DifficultyRules); err != nil {
		return err
	}
	if err := validateRawJSON("reward rules json", template.RewardRules); err != nil {
		return err
	}
	if err := validateRawJSON("expiration rules json", template.ExpirationRules); err != nil {
		return err
	}
	return nil
}

// Validate reports whether requirement contains valid optional rank and role gates.
func (requirement QuestRequirement) Validate() error {
	if requirement.MinRank > 0 {
		if err := progression.ValidateRank(requirement.MinRank); err != nil {
			return fmt.Errorf("min rank %d: %w", requirement.MinRank, ErrInvalidQuestRequirement)
		}
	}
	if requirement.MaxRank > 0 {
		if err := progression.ValidateRank(requirement.MaxRank); err != nil {
			return fmt.Errorf("max rank %d: %w", requirement.MaxRank, ErrInvalidQuestRequirement)
		}
	}
	if requirement.MinRank > 0 && requirement.MaxRank > 0 && requirement.MaxRank < requirement.MinRank {
		return fmt.Errorf("rank range %d-%d: %w", requirement.MinRank, requirement.MaxRank, ErrInvalidQuestRequirement)
	}
	if !requirement.Role.IsZero() {
		if err := requirement.Role.Validate(); err != nil {
			return fmt.Errorf("role %q: %w", requirement.Role, ErrInvalidQuestRequirement)
		}
		if err := progression.ValidateRoleLevel(requirement.RoleLevel); err != nil {
			return fmt.Errorf("role level %d: %w", requirement.RoleLevel, ErrInvalidQuestRequirement)
		}
	} else if requirement.RoleLevel != 0 {
		return fmt.Errorf("role level without role: %w", ErrInvalidQuestRequirement)
	}
	return nil
}

// Validate reports whether generated payload is valid JSON when populated.
func (payload GeneratedPayload) Validate() error {
	if payload.Difficulty < 0 {
		return fmt.Errorf("difficulty %d: %w", payload.Difficulty, ErrInvalidGeneratedPayload)
	}
	if !payload.Objective.IsZero() {
		if err := payload.Objective.Validate(); err != nil {
			return fmt.Errorf("generated objective: %w", err)
		}
	}
	if err := validateRawJSON("generated payload metadata", payload.MetadataJSON); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidGeneratedPayload, err)
	}
	if err := validateRawJSON("generated payload data", payload.Data); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidGeneratedPayload, err)
	}
	return nil
}

// Validate reports whether schema contains unique, valid objective definitions.
func (schema ObjectiveSchema) Validate() error {
	if len(schema.Objectives) > 0 {
		return schema.validateObjectiveList()
	}
	if err := schema.Kind.Validate(); err != nil {
		return err
	}
	if countObjectiveSchemaDetails(schema) != 1 {
		return ErrInvalidObjectiveSchema
	}
	switch schema.Kind {
	case ObjectiveKindKill:
		if schema.Kill == nil {
			return ErrInvalidObjectiveSchema
		}
		return schema.Kill.Validate()
	case ObjectiveKindCollect:
		if schema.Collect == nil {
			return ErrInvalidObjectiveSchema
		}
		return schema.Collect.Validate()
	case ObjectiveKindCraft:
		if schema.Craft == nil {
			return ErrInvalidObjectiveSchema
		}
		return schema.Craft.Validate()
	case ObjectiveKindScan:
		if schema.Scan == nil {
			return ErrInvalidObjectiveSchema
		}
		return schema.Scan.Validate()
	case ObjectiveKindBuild:
		if schema.Build == nil {
			return ErrInvalidObjectiveSchema
		}
		return schema.Build.Validate()
	case ObjectiveKindDeliver:
		if schema.Deliver == nil {
			return ErrInvalidObjectiveSchema
		}
		return schema.Deliver.Validate()
	default:
		return fmt.Errorf("objective kind %q: %w", schema.Kind, ErrInvalidObjectiveKind)
	}
}

// IsZero reports whether schema has no objective shape.
func (schema ObjectiveSchema) IsZero() bool {
	return len(schema.Objectives) == 0 &&
		schema.Kind == "" &&
		schema.Kill == nil &&
		schema.Collect == nil &&
		schema.Craft == nil &&
		schema.Scan == nil &&
		schema.Build == nil &&
		schema.Deliver == nil
}

func (schema ObjectiveSchema) validateObjectiveList() error {
	seen := make(map[string]struct{}, len(schema.Objectives))
	for _, objective := range schema.Objectives {
		if err := objective.Validate(); err != nil {
			return err
		}
		if _, ok := seen[objective.ID]; ok {
			return fmt.Errorf("objective id %q: %w", objective.ID, ErrDuplicateObjectiveID)
		}
		seen[objective.ID] = struct{}{}
	}
	return nil
}

func countObjectiveSchemaDetails(schema ObjectiveSchema) int {
	count := 0
	if schema.Kill != nil {
		count++
	}
	if schema.Collect != nil {
		count++
	}
	if schema.Craft != nil {
		count++
	}
	if schema.Scan != nil {
		count++
	}
	if schema.Build != nil {
		count++
	}
	if schema.Deliver != nil {
		count++
	}
	return count
}

// Validate reports whether objective has one detail shape matching its kind.
func (objective Objective) Validate() error {
	if strings.TrimSpace(objective.ID) == "" {
		return ErrEmptyObjectiveSchema
	}
	if err := objective.Kind.Validate(); err != nil {
		return err
	}
	if countObjectiveDetails(objective) != 1 {
		return fmt.Errorf("objective %q: %w", objective.ID, ErrEmptyObjectiveSchema)
	}
	switch objective.Kind {
	case ObjectiveKindKill:
		if objective.Kill == nil {
			return fmt.Errorf("objective %q kill: %w", objective.ID, ErrEmptyObjectiveSchema)
		}
		return objective.Kill.Validate()
	case ObjectiveKindCollect:
		if objective.Collect == nil {
			return fmt.Errorf("objective %q collect: %w", objective.ID, ErrEmptyObjectiveSchema)
		}
		return objective.Collect.Validate()
	case ObjectiveKindCraft:
		if objective.Craft == nil {
			return fmt.Errorf("objective %q craft: %w", objective.ID, ErrEmptyObjectiveSchema)
		}
		return objective.Craft.Validate()
	case ObjectiveKindScan:
		if objective.Scan == nil {
			return fmt.Errorf("objective %q scan: %w", objective.ID, ErrEmptyObjectiveSchema)
		}
		return objective.Scan.Validate()
	case ObjectiveKindBuild:
		if objective.Build == nil {
			return fmt.Errorf("objective %q build: %w", objective.ID, ErrEmptyObjectiveSchema)
		}
		return objective.Build.Validate()
	case ObjectiveKindDeliver:
		if objective.Deliver == nil {
			return fmt.Errorf("objective %q deliver: %w", objective.ID, ErrEmptyObjectiveSchema)
		}
		return objective.Deliver.Validate()
	default:
		return fmt.Errorf("objective kind %q: %w", objective.Kind, ErrInvalidObjectiveKind)
	}
}

// Validate reports whether objective names a kill target and positive count.
func (objective KillObjective) Validate() error {
	if strings.TrimSpace(objective.TargetNPCType) == "" {
		return ErrEmptyObjectiveTarget
	}
	return validateRequiredQuantity("kill required count", objective.RequiredCount)
}

// Validate reports whether objective names an item and positive quantity.
func (objective CollectObjective) Validate() error {
	if err := objective.ItemID.Validate(); err != nil {
		return fmt.Errorf("collect item: %w", err)
	}
	return validateRequiredQuantity("collect quantity", objective.Quantity)
}

// Validate reports whether objective names a recipe or item and positive quantity.
func (objective CraftObjective) Validate() error {
	if objective.RecipeID.IsZero() && objective.ItemID.IsZero() {
		return ErrEmptyObjectiveTarget
	}
	if !objective.RecipeID.IsZero() {
		if err := objective.RecipeID.Validate(); err != nil {
			return err
		}
	}
	if !objective.ItemID.IsZero() {
		if err := objective.ItemID.Validate(); err != nil {
			return err
		}
	}
	return validateRequiredQuantity("craft quantity", objective.Quantity)
}

// Validate reports whether objective has a positive required scan count.
func (objective ScanObjective) Validate() error {
	return validateRequiredQuantity("scan required count", objective.RequiredCount)
}

// Validate reports whether objective names a building target and positive count.
func (objective BuildObjective) Validate() error {
	if strings.TrimSpace(objective.BuildingType) == "" {
		return ErrEmptyObjectiveTarget
	}
	return validateRequiredQuantity("build required count", objective.RequiredCount)
}

// Validate reports whether objective names an item, destination type, and positive quantity.
func (objective DeliverObjective) Validate() error {
	if err := objective.ItemID.Validate(); err != nil {
		return fmt.Errorf("deliver item: %w", err)
	}
	if strings.TrimSpace(objective.DestinationType) == "" {
		return ErrEmptyObjectiveTarget
	}
	return validateRequiredQuantity("deliver quantity", objective.Quantity)
}

// Validate reports whether objective names a kill target and positive count.
func (objective KillObjectiveDetails) Validate() error {
	if strings.TrimSpace(objective.NPCType) == "" {
		return ErrInvalidObjectiveTarget
	}
	return validatePositiveObjectiveAmount("kill required count", objective.RequiredCount)
}

// Validate reports whether objective names an item and positive quantity.
func (objective CollectObjectiveDetails) Validate() error {
	if objective.ItemID.IsZero() {
		return ErrInvalidObjectiveTarget
	}
	if err := objective.ItemID.Validate(); err != nil {
		return fmt.Errorf("collect item: %w", ErrInvalidObjectiveTarget)
	}
	return validatePositiveObjectiveAmount("collect quantity", objective.RequiredQuantity)
}

// Validate reports whether objective names a recipe or item and positive count.
func (objective CraftObjectiveDetails) Validate() error {
	if objective.RecipeID.IsZero() && objective.ItemID.IsZero() {
		return ErrInvalidObjectiveTarget
	}
	if !objective.RecipeID.IsZero() {
		if err := objective.RecipeID.Validate(); err != nil {
			return fmt.Errorf("craft recipe: %w", ErrInvalidObjectiveTarget)
		}
	}
	if !objective.ItemID.IsZero() {
		if err := objective.ItemID.Validate(); err != nil {
			return fmt.Errorf("craft item: %w", ErrInvalidObjectiveTarget)
		}
	}
	return validatePositiveObjectiveAmount("craft required count", objective.RequiredCount)
}

// Validate reports whether objective has a supported scan target and positive count.
func (objective ScanObjectiveDetails) Validate() error {
	if err := objective.TargetKind.Validate(); err != nil {
		return err
	}
	return validatePositiveObjectiveAmount("scan required count", objective.RequiredCount)
}

// Validate reports whether objective names a building target and positive count.
func (objective BuildObjectiveDetails) Validate() error {
	if strings.TrimSpace(objective.BuildingID) == "" {
		return ErrInvalidObjectiveTarget
	}
	return validatePositiveObjectiveAmount("build required count", objective.RequiredCount)
}

// Validate reports whether objective names an item, destination, and positive quantity.
func (objective DeliverObjectiveDetails) Validate() error {
	if objective.ItemID.IsZero() {
		return ErrInvalidObjectiveTarget
	}
	if err := objective.ItemID.Validate(); err != nil {
		return fmt.Errorf("deliver item: %w", ErrInvalidObjectiveTarget)
	}
	if err := objective.DestinationKind.Validate(); err != nil {
		return err
	}
	return validatePositiveObjectiveAmount("deliver quantity", objective.RequiredQuantity)
}

func countObjectiveDetails(objective Objective) int {
	count := 0
	if objective.Kill != nil {
		count++
	}
	if objective.Collect != nil {
		count++
	}
	if objective.Craft != nil {
		count++
	}
	if objective.Scan != nil {
		count++
	}
	if objective.Build != nil {
		count++
	}
	if objective.Deliver != nil {
		count++
	}
	return count
}

func validateRequiredQuantity(label string, quantity foundation.Quantity) error {
	if err := quantity.Validate(); err != nil {
		return fmt.Errorf("%s: %w", label, ErrInvalidObjectiveRequired)
	}
	return nil
}

func validatePositiveObjectiveAmount(label string, amount int64) error {
	if err := foundation.ValidatePositiveAmount(amount); err != nil {
		return fmt.Errorf("%s: %w", label, ErrInvalidObjectiveAmount)
	}
	return nil
}

func validateRawJSON(label string, payload json.RawMessage) error {
	if len(payload) == 0 {
		return nil
	}
	if !json.Valid(payload) {
		return fmt.Errorf("%s: invalid json", label)
	}
	return nil
}

func sourceForTemplate(template QuestTemplate) catalog.VersionedDefinition {
	return catalog.VersionedDefinition{
		DefinitionID: template.TemplateID,
		Version:      template.Source.Version,
	}
}

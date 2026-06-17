package quests

import "fmt"

// NewQuestProgressFromSchema creates zero current progress for server-owned
// objective definitions.
func NewQuestProgressFromSchema(schema ObjectiveSchema) (QuestProgress, error) {
	return NewQuestProgress(schema)
}

// NewQuestProgress creates zero current progress for server-owned objective
// definitions.
func NewQuestProgress(schema ObjectiveSchema) (QuestProgress, error) {
	if err := schema.Validate(); err != nil {
		return QuestProgress{}, err
	}
	if len(schema.Objectives) == 0 {
		return QuestProgress{
			Objectives: []ObjectiveProgress{{
				ObjectiveID: schema.Kind.String(),
				Current:     0,
				Required:    schemaRequiredAmount(schema),
				Completed:   false,
			}},
		}, nil
	}
	progress := QuestProgress{
		Objectives: make([]ObjectiveProgress, 0, len(schema.Objectives)),
	}
	for _, objective := range schema.Objectives {
		progress.Objectives = append(progress.Objectives, ObjectiveProgress{
			ObjectiveID: objective.ID,
			Current:     0,
			Required:    objectiveRequiredAmount(objective),
			Completed:   false,
		})
	}
	return progress, nil
}

// ValidateAgainst reports whether progress mirrors the objective schema and
// does not contain client-authored overflow or unknown objective rows.
func (progress QuestProgress) ValidateAgainst(schema ObjectiveSchema) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if len(schema.Objectives) == 0 {
		if len(progress.Objectives) != 1 {
			return fmt.Errorf("progress objectives %d schema objectives 1: %w", len(progress.Objectives), ErrInvalidQuestProgress)
		}
		return progress.Objectives[0].Validate(schemaRequiredAmount(schema))
	}
	if len(progress.Objectives) != len(schema.Objectives) {
		return fmt.Errorf("progress objectives %d schema objectives %d: %w", len(progress.Objectives), len(schema.Objectives), ErrInvalidQuestProgress)
	}
	requiredByID := make(map[string]int64, len(schema.Objectives))
	for _, objective := range schema.Objectives {
		requiredByID[objective.ID] = objectiveRequiredAmount(objective)
	}
	seen := make(map[string]struct{}, len(progress.Objectives))
	for _, objectiveProgress := range progress.Objectives {
		required, ok := requiredByID[objectiveProgress.ObjectiveID]
		if !ok {
			return fmt.Errorf("objective progress %q: %w", objectiveProgress.ObjectiveID, ErrUnexpectedQuestProgress)
		}
		if err := objectiveProgress.Validate(required); err != nil {
			return err
		}
		if _, ok := seen[objectiveProgress.ObjectiveID]; ok {
			return fmt.Errorf("objective progress %q: %w", objectiveProgress.ObjectiveID, ErrDuplicateObjectiveID)
		}
		seen[objectiveProgress.ObjectiveID] = struct{}{}
	}
	return nil
}

// Complete reports whether every objective progress row is complete.
func (progress QuestProgress) Complete() bool {
	if len(progress.Objectives) == 0 {
		return false
	}
	for _, objective := range progress.Objectives {
		if objective.Current < objective.Required {
			return false
		}
	}
	return true
}

// Validate reports whether one objective progress row is internally coherent.
func (progress ObjectiveProgress) Validate(required int64) error {
	if progress.ObjectiveID == "" {
		return ErrInvalidQuestProgress
	}
	if required <= 0 || progress.Required != required {
		return fmt.Errorf("objective %q required %d want %d: %w", progress.ObjectiveID, progress.Required, required, ErrInvalidQuestProgress)
	}
	if progress.Current < 0 || progress.Current > progress.Required {
		return fmt.Errorf("objective %q current %d required %d: %w", progress.ObjectiveID, progress.Current, progress.Required, ErrInvalidQuestProgress)
	}
	if progress.Completed && progress.Current < progress.Required {
		return fmt.Errorf("objective %q completed %t current %d required %d: %w", progress.ObjectiveID, progress.Completed, progress.Current, progress.Required, ErrInvalidQuestProgress)
	}
	return nil
}

func schemaRequiredAmount(schema ObjectiveSchema) int64 {
	switch schema.Kind {
	case ObjectiveKindKill:
		return schema.Kill.RequiredCount
	case ObjectiveKindCollect:
		return schema.Collect.RequiredQuantity
	case ObjectiveKindCraft:
		return schema.Craft.RequiredCount
	case ObjectiveKindScan:
		return schema.Scan.RequiredCount
	case ObjectiveKindBuild:
		return schema.Build.RequiredCount
	case ObjectiveKindDeliver:
		return schema.Deliver.RequiredQuantity
	default:
		return 0
	}
}

func objectiveRequiredAmount(objective Objective) int64 {
	switch objective.Kind {
	case ObjectiveKindKill:
		return objective.Kill.RequiredCount.Int64()
	case ObjectiveKindCollect:
		return objective.Collect.Quantity.Int64()
	case ObjectiveKindCraft:
		return objective.Craft.Quantity.Int64()
	case ObjectiveKindScan:
		return objective.Scan.RequiredCount.Int64()
	case ObjectiveKindBuild:
		return objective.Build.RequiredCount.Int64()
	case ObjectiveKindDeliver:
		return objective.Deliver.Quantity.Int64()
	default:
		return 0
	}
}

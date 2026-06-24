package content

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/quests"
)

// QuestObjectiveSchemaRow is the JSON-friendly CMS shape for quest objectives.
type QuestObjectiveSchemaRow struct {
	Objectives []QuestObjectiveRow             `json:"objectives,omitempty"`
	Kind       quests.ObjectiveKind            `json:"kind,omitempty"`
	Kill       *quests.KillObjectiveDetails    `json:"kill,omitempty"`
	Collect    *quests.CollectObjectiveDetails `json:"collect,omitempty"`
	Craft      *quests.CraftObjectiveDetails   `json:"craft,omitempty"`
	Scan       *quests.ScanObjectiveDetails    `json:"scan,omitempty"`
	Build      *quests.BuildObjectiveDetails   `json:"build,omitempty"`
	Deliver    *quests.DeliverObjectiveDetails `json:"deliver,omitempty"`
}

// QuestObjectiveRow is one objective list entry with primitive quantities.
type QuestObjectiveRow struct {
	ID      string                    `json:"id"`
	Kind    quests.ObjectiveKind      `json:"kind"`
	Kill    *QuestKillObjectiveRow    `json:"kill,omitempty"`
	Collect *QuestCollectObjectiveRow `json:"collect,omitempty"`
	Craft   *QuestCraftObjectiveRow   `json:"craft,omitempty"`
	Scan    *QuestScanObjectiveRow    `json:"scan,omitempty"`
	Build   *QuestBuildObjectiveRow   `json:"build,omitempty"`
	Deliver *QuestDeliverObjectiveRow `json:"deliver,omitempty"`
}

type QuestKillObjectiveRow struct {
	TargetNPCType string `json:"target_npc_type"`
	RequiredCount int64  `json:"required_count"`
}

type QuestCollectObjectiveRow struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

type QuestCraftObjectiveRow struct {
	RecipeID catalog.DefinitionID `json:"recipe_id,omitempty"`
	ItemID   foundation.ItemID    `json:"item_id,omitempty"`
	Quantity int64                `json:"quantity"`
}

type QuestScanObjectiveRow struct {
	TargetSignalType string `json:"target_signal_type,omitempty"`
	RequiredCount    int64  `json:"required_count"`
}

type QuestBuildObjectiveRow struct {
	BuildingType  string `json:"building_type"`
	RequiredCount int64  `json:"required_count"`
}

type QuestDeliverObjectiveRow struct {
	ItemID          foundation.ItemID `json:"item_id"`
	Quantity        int64             `json:"quantity"`
	DestinationType string            `json:"destination_type"`
	DestinationID   string            `json:"destination_id,omitempty"`
}

// ObjectiveSchema converts primitive CMS quantities into runtime quantities.
func (schema QuestObjectiveSchemaRow) ObjectiveSchema() (quests.ObjectiveSchema, error) {
	if len(schema.Objectives) > 0 {
		objectives := make([]quests.Objective, 0, len(schema.Objectives))
		for index, objectiveRow := range schema.Objectives {
			objective, err := objectiveRow.Objective()
			if err != nil {
				return quests.ObjectiveSchema{}, fmt.Errorf("objective %d: %w", index, err)
			}
			objectives = append(objectives, objective)
		}
		return quests.ObjectiveSchema{Objectives: objectives}, nil
	}
	return quests.ObjectiveSchema{
		Kind:    schema.Kind,
		Kill:    cloneQuestPtr(schema.Kill),
		Collect: cloneQuestPtr(schema.Collect),
		Craft:   cloneQuestPtr(schema.Craft),
		Scan:    cloneQuestPtr(schema.Scan),
		Build:   cloneQuestPtr(schema.Build),
		Deliver: cloneQuestPtr(schema.Deliver),
	}, nil
}

// Objective converts one CMS objective row into the runtime objective shape.
func (row QuestObjectiveRow) Objective() (quests.Objective, error) {
	objective := quests.Objective{ID: row.ID, Kind: row.Kind}
	if row.Kill != nil {
		value, err := row.Kill.Objective()
		if err != nil {
			return quests.Objective{}, err
		}
		objective.Kill = &value
	}
	if row.Collect != nil {
		value, err := row.Collect.Objective()
		if err != nil {
			return quests.Objective{}, err
		}
		objective.Collect = &value
	}
	if row.Craft != nil {
		value, err := row.Craft.Objective()
		if err != nil {
			return quests.Objective{}, err
		}
		objective.Craft = &value
	}
	if row.Scan != nil {
		value, err := row.Scan.Objective()
		if err != nil {
			return quests.Objective{}, err
		}
		objective.Scan = &value
	}
	if row.Build != nil {
		value, err := row.Build.Objective()
		if err != nil {
			return quests.Objective{}, err
		}
		objective.Build = &value
	}
	if row.Deliver != nil {
		value, err := row.Deliver.Objective()
		if err != nil {
			return quests.Objective{}, err
		}
		objective.Deliver = &value
	}
	return objective, nil
}

func (row QuestKillObjectiveRow) Objective() (quests.KillObjective, error) {
	quantity, err := questRowQuantity("kill required_count", row.RequiredCount)
	if err != nil {
		return quests.KillObjective{}, err
	}
	return quests.KillObjective{TargetNPCType: row.TargetNPCType, RequiredCount: quantity}, nil
}

func (row QuestCollectObjectiveRow) Objective() (quests.CollectObjective, error) {
	quantity, err := questRowQuantity("collect quantity", row.Quantity)
	if err != nil {
		return quests.CollectObjective{}, err
	}
	return quests.CollectObjective{ItemID: row.ItemID, Quantity: quantity}, nil
}

func (row QuestCraftObjectiveRow) Objective() (quests.CraftObjective, error) {
	quantity, err := questRowQuantity("craft quantity", row.Quantity)
	if err != nil {
		return quests.CraftObjective{}, err
	}
	return quests.CraftObjective{RecipeID: row.RecipeID, ItemID: row.ItemID, Quantity: quantity}, nil
}

func (row QuestScanObjectiveRow) Objective() (quests.ScanObjective, error) {
	quantity, err := questRowQuantity("scan required_count", row.RequiredCount)
	if err != nil {
		return quests.ScanObjective{}, err
	}
	return quests.ScanObjective{TargetSignalType: row.TargetSignalType, RequiredCount: quantity}, nil
}

func (row QuestBuildObjectiveRow) Objective() (quests.BuildObjective, error) {
	quantity, err := questRowQuantity("build required_count", row.RequiredCount)
	if err != nil {
		return quests.BuildObjective{}, err
	}
	return quests.BuildObjective{BuildingType: row.BuildingType, RequiredCount: quantity}, nil
}

func (row QuestDeliverObjectiveRow) Objective() (quests.DeliverObjective, error) {
	quantity, err := questRowQuantity("deliver quantity", row.Quantity)
	if err != nil {
		return quests.DeliverObjective{}, err
	}
	return quests.DeliverObjective{
		ItemID:          row.ItemID,
		Quantity:        quantity,
		DestinationType: row.DestinationType,
		DestinationID:   row.DestinationID,
	}, nil
}

func questRowQuantity(path string, amount int64) (foundation.Quantity, error) {
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		return foundation.Quantity{}, fmt.Errorf("%s: %w", path, err)
	}
	return quantity, nil
}

func cloneQuestPtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

package production

import (
	"errors"
	"fmt"
)

var ErrInvalidBuildingMutationOutboxDispatch = errors.New("invalid building mutation outbox dispatch")

// BuildingMutationOutboxDispatchPlan is the after-commit handoff contract from
// a durable planet building mutation commit to a publisher scheduler.
type BuildingMutationOutboxDispatchPlan struct {
	Reference     BuildingMutationReferenceRecord
	OutboxRecords []ProductionOutboxRecord
}

// NewBuildingMutationOutboxDispatchPlan validates that committed building
// mutation outbox rows are pending, tied to the same mutation reference, and
// include building update evidence before a publisher scheduler sees them.
func NewBuildingMutationOutboxDispatchPlan(
	reference *BuildingMutationReferenceRecord,
	records []ProductionOutboxRecord,
) (BuildingMutationOutboxDispatchPlan, error) {
	if reference == nil {
		if len(records) == 0 {
			return BuildingMutationOutboxDispatchPlan{}, nil
		}
		return BuildingMutationOutboxDispatchPlan{}, fmt.Errorf("reference: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}

	clonedReference := cloneBuildingMutationReferenceRecord(*reference)
	clonedRecords := cloneProductionOutboxRecords(records)
	if err := validateBuildingMutationOutboxDispatchReference(clonedReference); err != nil {
		return BuildingMutationOutboxDispatchPlan{}, err
	}
	if len(clonedRecords) == 0 {
		return BuildingMutationOutboxDispatchPlan{}, fmt.Errorf("outbox: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	hasBuildingUpdate := false
	for index, record := range clonedRecords {
		if err := validateBuildingMutationOutboxDispatchRecord(clonedReference, record); err != nil {
			return BuildingMutationOutboxDispatchPlan{}, fmt.Errorf("outbox[%d]: %w", index, err)
		}
		hasBuildingUpdate = hasBuildingUpdate || record.Event.Type == EventPlanetBuildingUpdated
	}
	if !buildingMutationOutboxRecordsEqual(clonedReference.Result.OutboxRecords, clonedRecords) {
		return BuildingMutationOutboxDispatchPlan{}, fmt.Errorf("outbox.result: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	if !hasBuildingUpdate {
		return BuildingMutationOutboxDispatchPlan{}, fmt.Errorf("outbox.building_update: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	return BuildingMutationOutboxDispatchPlan{
		Reference:     clonedReference,
		OutboxRecords: clonedRecords,
	}, nil
}

func validateBuildingMutationOutboxDispatchReference(record BuildingMutationReferenceRecord) error {
	if err := validateBuildingMutationDurableCommitReference(record); err != nil {
		return fmt.Errorf("reference: %w: %v", ErrInvalidBuildingMutationOutboxDispatch, err)
	}
	return nil
}

func validateBuildingMutationOutboxDispatchRecord(
	reference BuildingMutationReferenceRecord,
	record ProductionOutboxRecord,
) error {
	if record.OutboxID == "" || record.Sequence == 0 {
		return ErrInvalidBuildingMutationOutboxDispatch
	}
	if record.Status != ProductionOutboxStatusPending {
		return fmt.Errorf("status %q: %w", record.Status, ErrInvalidBuildingMutationOutboxDispatch)
	}
	if record.ReferenceKey != reference.ReferenceKey || record.SettlementWindow != "" {
		return fmt.Errorf("evidence: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	if record.Event.Type == "" || len(record.Event.Payload) == 0 {
		return fmt.Errorf("event: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.PublishedAt.IsZero() || !record.FailedAt.IsZero() {
		return fmt.Errorf("delivery_state: %w", ErrInvalidBuildingMutationOutboxDispatch)
	}
	return nil
}

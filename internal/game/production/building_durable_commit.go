package production

import (
	"errors"
	"fmt"
)

var ErrInvalidBuildingMutationDurableCommit = errors.New("invalid building mutation durable commit")

// BuildingMutationDurableCommitPlan validates the row bundle a future durable
// DB transaction must commit for one planet building mutation: idempotency
// reference, pending outbox rows, and production-local material ledger rows.
type BuildingMutationDurableCommitPlan struct {
	Reference      BuildingMutationReferenceRecord
	OutboxRecords  []ProductionOutboxRecord
	MaterialLedger []BuildingMaterialLedgerEntry
}

// ApplyDurableCommit validates and records this durable building mutation plan
// through a durable commit adapter.
func (plan BuildingMutationDurableCommitPlan) ApplyDurableCommit(
	store BuildingMutationDurableCommitStore,
) (BuildingMutationDurableCommitResult, error) {
	if store == nil {
		return BuildingMutationDurableCommitResult{}, ErrInvalidBuildingMutationDurableCommit
	}
	return store.ApplyBuildingMutationDurableCommitPlan(plan)
}

// NewBuildingMutationDurableCommitPlan validates one committed building
// mutation row bundle. Empty reference/outbox/ledger input is a no-op plan.
func NewBuildingMutationDurableCommitPlan(
	reference *BuildingMutationReferenceRecord,
	outbox []ProductionOutboxRecord,
	materialLedger []BuildingMaterialLedgerEntry,
) (BuildingMutationDurableCommitPlan, error) {
	if reference == nil {
		if len(outbox) == 0 && len(materialLedger) == 0 {
			return BuildingMutationDurableCommitPlan{}, nil
		}
		return BuildingMutationDurableCommitPlan{}, fmt.Errorf("reference: %w", ErrInvalidBuildingMutationDurableCommit)
	}

	clonedReference := cloneBuildingMutationReferenceRecord(*reference)
	clonedOutbox := cloneProductionOutboxRecords(outbox)
	clonedLedger := cloneBuildingMaterialLedgerEntries(materialLedger)
	if err := validateBuildingMutationDurableCommitReference(clonedReference); err != nil {
		return BuildingMutationDurableCommitPlan{}, err
	}
	if err := validateBuildingMutationDurableCommitOutbox(clonedReference, clonedOutbox); err != nil {
		return BuildingMutationDurableCommitPlan{}, err
	}
	if err := validateBuildingMutationDurableCommitLedger(clonedReference, clonedLedger); err != nil {
		return BuildingMutationDurableCommitPlan{}, err
	}
	if !buildingMutationOutboxRecordsEqual(clonedReference.Result.OutboxRecords, clonedOutbox) {
		return BuildingMutationDurableCommitPlan{}, fmt.Errorf("outbox.result: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	if !buildingMutationMaterialLedgerEqual(clonedReference.Result.MaterialLedger, clonedLedger) {
		return BuildingMutationDurableCommitPlan{}, fmt.Errorf("material_ledger.result: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	return BuildingMutationDurableCommitPlan{
		Reference:      clonedReference,
		OutboxRecords:  clonedOutbox,
		MaterialLedger: clonedLedger,
	}, nil
}

func validateBuildingMutationDurableCommitReference(record BuildingMutationReferenceRecord) error {
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("reference_key: %w", err)
	}
	if err := record.Operation.Validate(); err != nil {
		return fmt.Errorf("operation: %w", err)
	}
	if err := record.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := record.BuildingID.Validate(); err != nil {
		return fmt.Errorf("building_id: %w", err)
	}
	if record.RecordedAt.IsZero() {
		return fmt.Errorf("recorded_at: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	result := record.Result
	if result.Duplicate ||
		result.ReferenceKey != record.ReferenceKey ||
		result.Operation != record.Operation ||
		result.Building.PlanetID != record.PlanetID ||
		result.Building.BuildingID != record.BuildingID {
		return fmt.Errorf("result: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	if result.Definition.DefinitionID != result.Building.Source.DefinitionID ||
		result.Definition.BuildingType != result.Building.BuildingType ||
		result.Definition.Level != result.Building.Level {
		return fmt.Errorf("result.definition: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	return nil
}

func validateBuildingMutationDurableCommitOutbox(
	reference BuildingMutationReferenceRecord,
	records []ProductionOutboxRecord,
) error {
	if len(records) == 0 {
		return fmt.Errorf("outbox: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	hasBuildingUpdate := false
	for index, record := range records {
		if err := validateBuildingMutationDurableCommitOutboxRecord(reference, record); err != nil {
			return fmt.Errorf("outbox[%d]: %w", index, err)
		}
		hasBuildingUpdate = hasBuildingUpdate || record.Event.Type == EventPlanetBuildingUpdated
	}
	if !hasBuildingUpdate {
		return fmt.Errorf("outbox.building_update: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	return nil
}

func validateBuildingMutationDurableCommitOutboxRecord(
	reference BuildingMutationReferenceRecord,
	record ProductionOutboxRecord,
) error {
	if record.OutboxID == "" || record.Sequence == 0 {
		return ErrInvalidBuildingMutationDurableCommit
	}
	if record.Status != ProductionOutboxStatusPending {
		return fmt.Errorf("status %q: %w", record.Status, ErrInvalidBuildingMutationDurableCommit)
	}
	if record.ReferenceKey != reference.ReferenceKey || record.SettlementWindow != "" {
		return fmt.Errorf("evidence: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	if record.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	if record.Event.Type == "" || len(record.Event.Payload) == 0 {
		return fmt.Errorf("event: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	if !record.ClaimedAt.IsZero() || record.ClaimToken != "" || !record.PublishedAt.IsZero() || !record.FailedAt.IsZero() {
		return fmt.Errorf("delivery_state: %w", ErrInvalidBuildingMutationDurableCommit)
	}
	return nil
}

func validateBuildingMutationDurableCommitLedger(
	reference BuildingMutationReferenceRecord,
	rows []BuildingMaterialLedgerEntry,
) error {
	for index, row := range rows {
		if err := row.Validate(); err != nil {
			return fmt.Errorf("material_ledger[%d]: %w: %v", index, ErrInvalidBuildingMutationDurableCommit, err)
		}
		if row.ReferenceKey != reference.ReferenceKey ||
			row.Operation != reference.Operation ||
			row.PlanetID != reference.PlanetID ||
			row.BuildingID != reference.BuildingID {
			return fmt.Errorf("material_ledger[%d]: %w", index, ErrInvalidBuildingMutationDurableCommit)
		}
	}
	return nil
}

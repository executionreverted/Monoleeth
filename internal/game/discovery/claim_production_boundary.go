package discovery

import (
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
)

// ClaimProductionInitializationRecord is the process-local boundary marker for
// production rows initialized by a planet claim. Future durable adapters should
// store this evidence with the same claim reference used by owner-CAS.
type ClaimProductionInitializationRecord struct {
	ClaimReference     PlanetClaimReference
	ReferenceKey       foundation.IdempotencyKey
	PlayerID           foundation.PlayerID
	PlanetID           foundation.PlanetID
	PlanetLevel        int
	ClaimedAt          time.Time
	InitializedAt      time.Time
	Created            bool
	AlreadyInitialized bool
}

// ClaimProductionInitializationDurablePlan validates the production-init row a
// future durable claim/production adapter must persist after owner-CAS.
type ClaimProductionInitializationDurablePlan struct {
	Initialization ClaimProductionInitializationRecord
	Boundary       ClaimBoundaryRecord
}

// DurablePlan validates this production-init record against an optional claim
// boundary. Boundary evidence may be pending or complete because retries can
// read the initialization row before or after side-effect completion.
func (record ClaimProductionInitializationRecord) DurablePlan(
	boundary *ClaimBoundaryRecord,
) (ClaimProductionInitializationDurablePlan, error) {
	return NewClaimProductionInitializationDurablePlan(&record, boundary)
}

// NewClaimProductionInitializationDurablePlan validates one claim-production
// initialization evidence row. Empty input is a no-op plan.
func NewClaimProductionInitializationDurablePlan(
	record *ClaimProductionInitializationRecord,
	boundary *ClaimBoundaryRecord,
) (ClaimProductionInitializationDurablePlan, error) {
	if record == nil {
		if boundary == nil {
			return ClaimProductionInitializationDurablePlan{}, nil
		}
		return ClaimProductionInitializationDurablePlan{}, fmt.Errorf("production_initialization: %w", ErrInvalidClaimDurableCommit)
	}
	clonedRecord := cloneClaimProductionInitializationRecord(*record)
	if err := validateClaimProductionInitializationDurableRecord(clonedRecord); err != nil {
		return ClaimProductionInitializationDurablePlan{}, err
	}
	plan := ClaimProductionInitializationDurablePlan{Initialization: clonedRecord}
	if boundary != nil {
		clonedBoundary := cloneClaimBoundaryRecord(*boundary)
		if err := validateClaimProductionInitializationDurableBoundary(clonedRecord, clonedBoundary); err != nil {
			return ClaimProductionInitializationDurablePlan{}, err
		}
		plan.Boundary = clonedBoundary
	}
	return plan, nil
}

// ClaimProductionInitializations returns production-init evidence in
// deterministic claim-reference order.
func (service *ClaimService) ClaimProductionInitializations() []ClaimProductionInitializationRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	if len(service.productionInitializations) == 0 {
		return nil
	}
	refs := make([]PlanetClaimReference, 0, len(service.productionInitializations))
	for ref := range service.productionInitializations {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	records := make([]ClaimProductionInitializationRecord, 0, len(refs))
	for _, ref := range refs {
		records = append(records, cloneClaimProductionInitializationRecord(service.productionInitializations[ref]))
	}
	return records
}

// ClaimProductionInitializationDurablePlan returns one validated
// production-initialization durable row for recovery workers. Boundary evidence
// is included when available and may still be pending if later claim
// side-effects failed.
func (service *ClaimService) ClaimProductionInitializationDurablePlan(
	reference PlanetClaimReference,
) (ClaimProductionInitializationDurablePlan, bool, error) {
	if service == nil {
		return ClaimProductionInitializationDurablePlan{}, false, ErrInvalidClaimConfig
	}
	if err := reference.Validate(); err != nil {
		return ClaimProductionInitializationDurablePlan{}, false, err
	}
	record, ok, err := service.claimProductionInitialization(reference)
	if err != nil || !ok {
		return ClaimProductionInitializationDurablePlan{}, ok, err
	}
	var boundary *ClaimBoundaryRecord
	if service.claimBoundaries != nil {
		claimBoundary, hasBoundary, err := service.claimBoundaries.ClaimBoundary(reference)
		if err != nil {
			return ClaimProductionInitializationDurablePlan{}, false, err
		}
		if hasBoundary {
			boundary = &claimBoundary
		}
	}
	plan, err := record.DurablePlan(boundary)
	if err != nil {
		return ClaimProductionInitializationDurablePlan{}, false, err
	}
	return plan, true, nil
}

func (service *ClaimService) claimProductionAlreadyInitializedLocked(input ClaimPlanetInput) (bool, error) {
	record, ok := service.productionInitializations[input.ClaimReference]
	if !ok {
		return false, nil
	}
	if record.PlayerID != input.PlayerID || record.PlanetID != input.PlanetID {
		return false, ErrPlanetClaimReferenceConflict
	}
	return true, nil
}

func (service *ClaimService) recordClaimProductionInitializationLocked(input ClaimProductionInitializeInput, result ClaimProductionInitializeResult, initializedAt time.Time) {
	referenceKey, _ := input.ClaimReference.IdempotencyKey(input.PlayerID, input.PlanetID)
	service.productionInitializations[input.ClaimReference] = cloneClaimProductionInitializationRecord(ClaimProductionInitializationRecord{
		ClaimReference:     input.ClaimReference,
		ReferenceKey:       referenceKey,
		PlayerID:           input.PlayerID,
		PlanetID:           input.PlanetID,
		PlanetLevel:        input.PlanetLevel,
		ClaimedAt:          input.ClaimedAt.UTC(),
		InitializedAt:      initializedAt.UTC(),
		Created:            result.Created,
		AlreadyInitialized: result.AlreadyInitialized,
	})
}

func cloneClaimProductionInitializationRecord(record ClaimProductionInitializationRecord) ClaimProductionInitializationRecord {
	record.ClaimedAt = record.ClaimedAt.UTC()
	record.InitializedAt = record.InitializedAt.UTC()
	return record
}

func validateClaimProductionInitializationDurableRecord(record ClaimProductionInitializationRecord) error {
	if err := record.ClaimReference.Validate(); err != nil {
		return fmt.Errorf("production_initialization.claim_reference: %w", err)
	}
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("production_initialization.reference_key: %w", err)
	}
	if err := record.PlayerID.Validate(); err != nil {
		return fmt.Errorf("production_initialization.player_id: %w", err)
	}
	if err := record.PlanetID.Validate(); err != nil {
		return fmt.Errorf("production_initialization.planet_id: %w", err)
	}
	if record.PlanetLevel <= 0 {
		return fmt.Errorf("production_initialization.planet_level %d: %w", record.PlanetLevel, ErrInvalidClaimDurableCommit)
	}
	if record.ClaimedAt.IsZero() || record.InitializedAt.IsZero() {
		return fmt.Errorf("production_initialization.timestamps: %w", ErrInvalidClaimDurableCommit)
	}
	if record.Created == record.AlreadyInitialized {
		return fmt.Errorf("production_initialization.result: %w", ErrInvalidClaimDurableCommit)
	}
	if record.InitializedAt.Before(record.ClaimedAt) {
		return fmt.Errorf("production_initialization.initialized_at: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimDurableReferenceKey(record.ClaimReference, record.ReferenceKey, record.PlayerID, record.PlanetID); err != nil {
		return fmt.Errorf("production_initialization.reference_key: %w", err)
	}
	return nil
}

func validateClaimProductionInitializationDurableBoundary(
	record ClaimProductionInitializationRecord,
	boundary ClaimBoundaryRecord,
) error {
	switch boundary.Status {
	case ClaimBoundaryStatusPendingSideEffects:
		if err := validateClaimDurableBeginBoundary(boundary); err != nil {
			return fmt.Errorf("boundary: %w", err)
		}
	case ClaimBoundaryStatusComplete:
		if err := validateClaimDurableCommitBoundary(boundary); err != nil {
			return fmt.Errorf("boundary: %w", err)
		}
	default:
		return fmt.Errorf("boundary.status %q: %w", boundary.Status, ErrInvalidClaimDurableCommit)
	}
	if boundary.ClaimReference != record.ClaimReference ||
		boundary.ReferenceKey != record.ReferenceKey ||
		boundary.PlayerID != record.PlayerID ||
		boundary.PlanetID != record.PlanetID ||
		!boundary.ClaimedAt.Equal(record.ClaimedAt) {
		return fmt.Errorf("boundary: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimDurableReferenceKey(boundary.ClaimReference, boundary.ReferenceKey, boundary.PlayerID, boundary.PlanetID); err != nil {
		return fmt.Errorf("boundary.reference_key: %w", err)
	}
	return nil
}

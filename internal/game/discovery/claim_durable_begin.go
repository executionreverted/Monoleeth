package discovery

import "fmt"

// ClaimDurableBeginPlan validates the row bundle a future durable claim DB
// transaction must commit when the dangerous begin step runs: X Core debit
// evidence, and when owner-CAS succeeds, pending owner boundary rows.
type ClaimDurableBeginPlan struct {
	XCoreConsumption        ClaimXCoreConsumptionRecord
	XCoreStorageMutation    ClaimXCoreStorageMutationPlan
	HasXCoreStorageMutation bool
	Planet                  Planet
	Boundary                ClaimBoundaryRecord
	StaleIntel              []PlayerPlanetIntel
}

// DurableBeginPlan returns the validated row bundle this X Core + owner-CAS
// begin operation committed. Debit-only failures return X Core recovery
// evidence without pretending the owner transition committed.
func (result BeginPlanetClaimWithXCoreResult) DurableBeginPlan() (ClaimDurableBeginPlan, error) {
	if result.Boundary.Boundary.ClaimReference == "" && result.Boundary.Planet.ID == "" && len(result.Boundary.StaleIntel) == 0 {
		storagePlan, err := NewClaimXCoreStorageMutationPlan(result.XCoreInput, result.XCoreResult, result.XCoreConsumption, nil)
		if err != nil {
			return ClaimDurableBeginPlan{}, err
		}
		return NewClaimDurableBeginPlan(&storagePlan, nil, nil, nil)
	}
	storagePlan, err := NewClaimXCoreStorageMutationPlan(
		result.XCoreInput,
		result.XCoreResult,
		result.XCoreConsumption,
		&result.Boundary.Boundary,
	)
	if err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	return NewClaimDurableBeginPlan(
		&storagePlan,
		&result.Boundary.Planet,
		&result.Boundary.Boundary,
		result.Boundary.StaleIntel,
	)
}

// NewClaimDurableBeginPlan validates one claim-begin row bundle. Empty input
// is a no-op plan; storage-only input is retry evidence for a failed owner-CAS
// begin.
func NewClaimDurableBeginPlan(
	xcoreStorage *ClaimXCoreStorageMutationPlan,
	planet *Planet,
	boundary *ClaimBoundaryRecord,
	staleIntel []PlayerPlanetIntel,
) (ClaimDurableBeginPlan, error) {
	if xcoreStorage == nil {
		if planet == nil && boundary == nil && len(staleIntel) == 0 {
			return ClaimDurableBeginPlan{}, nil
		}
		return ClaimDurableBeginPlan{}, fmt.Errorf("x_core_storage: %w", ErrInvalidClaimDurableCommit)
	}
	clonedStorage := cloneClaimXCoreStorageMutationPlan(*xcoreStorage)
	if err := validateClaimXCoreStorageMutationPlan(clonedStorage); err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	clonedXCore := cloneClaimXCoreConsumptionRecord(clonedStorage.Consumption)
	if planet == nil && boundary == nil {
		if len(staleIntel) > 0 {
			return ClaimDurableBeginPlan{}, fmt.Errorf("stale_intel: %w", ErrInvalidClaimDurableCommit)
		}
		if clonedStorage.HasBoundary {
			return ClaimDurableBeginPlan{}, fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
		}
		return ClaimDurableBeginPlan{
			XCoreConsumption:        clonedXCore,
			XCoreStorageMutation:    clonedStorage,
			HasXCoreStorageMutation: true,
		}, nil
	}
	if planet == nil || boundary == nil {
		return ClaimDurableBeginPlan{}, fmt.Errorf("owner_boundary: %w", ErrInvalidClaimDurableCommit)
	}
	if !clonedStorage.HasBoundary {
		return ClaimDurableBeginPlan{}, fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
	}

	clonedPlanet := clonePlanet(*planet)
	clonedBoundary := cloneClaimBoundaryRecord(*boundary)
	clonedStaleIntel := cloneClaimBoundaryStaleIntel(staleIntel)

	if err := validateClaimDurableBeginBoundary(clonedBoundary); err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	if err := validateClaimDurableCommitXCore(clonedBoundary, clonedXCore); err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	if err := validateClaimDurableBeginPlanet(clonedBoundary, clonedPlanet); err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	if err := validateClaimDurableBeginStaleIntel(clonedBoundary, clonedStaleIntel); err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	if !claimDurableBeginStorageBoundaryMatches(clonedStorage.Boundary, clonedBoundary) {
		return ClaimDurableBeginPlan{}, fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
	}
	return ClaimDurableBeginPlan{
		XCoreConsumption:        clonedXCore,
		XCoreStorageMutation:    clonedStorage,
		HasXCoreStorageMutation: true,
		Planet:                  clonedPlanet,
		Boundary:                clonedBoundary,
		StaleIntel:              clonedStaleIntel,
	}, nil
}

func claimDurableBeginStorageBoundaryMatches(left ClaimBoundaryRecord, right ClaimBoundaryRecord) bool {
	return left.ClaimReference == right.ClaimReference &&
		left.ReferenceKey == right.ReferenceKey &&
		left.PlayerID == right.PlayerID &&
		left.PlanetID == right.PlanetID &&
		left.Status == right.Status &&
		left.EventID == right.EventID &&
		left.ClaimedAt.Equal(right.ClaimedAt) &&
		left.RecordedAt.Equal(right.RecordedAt) &&
		left.CompletedAt.Equal(right.CompletedAt) &&
		left.StaleIntelCount == right.StaleIntelCount &&
		left.StaleListingCount == right.StaleListingCount
}

func validateClaimDurableBeginPlan(plan ClaimDurableBeginPlan) error {
	if !plan.HasXCoreStorageMutation {
		return nil
	}
	if err := validateClaimXCoreStorageMutationPlan(plan.XCoreStorageMutation); err != nil {
		return fmt.Errorf("x_core_storage: %w: %v", ErrInvalidClaimDurableCommit, err)
	}
	if !claimLifecycleXCoreMatches(plan.XCoreConsumption, plan.XCoreStorageMutation.Consumption) {
		return fmt.Errorf("x_core_storage.consumption: %w", ErrInvalidClaimDurableCommit)
	}
	if plan.Planet.ID == "" && plan.Boundary.ClaimReference == "" {
		if len(plan.StaleIntel) > 0 {
			return fmt.Errorf("stale_intel: %w", ErrInvalidClaimDurableCommit)
		}
		if plan.XCoreStorageMutation.HasBoundary {
			return fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
		}
		return nil
	}
	if plan.Planet.ID == "" || plan.Boundary.ClaimReference == "" {
		return fmt.Errorf("owner_boundary: %w", ErrInvalidClaimDurableCommit)
	}
	if !plan.XCoreStorageMutation.HasBoundary {
		return fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimDurableBeginBoundary(plan.Boundary); err != nil {
		return err
	}
	if err := validateClaimDurableCommitXCore(plan.Boundary, plan.XCoreConsumption); err != nil {
		return err
	}
	if err := validateClaimDurableBeginPlanet(plan.Boundary, plan.Planet); err != nil {
		return err
	}
	if err := validateClaimDurableBeginStaleIntel(plan.Boundary, plan.StaleIntel); err != nil {
		return err
	}
	if !claimDurableBeginStorageBoundaryMatches(plan.XCoreStorageMutation.Boundary, plan.Boundary) {
		return fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimDurableBeginBoundary(record ClaimBoundaryRecord) error {
	if err := record.ClaimReference.Validate(); err != nil {
		return fmt.Errorf("boundary.claim_reference: %w", err)
	}
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("boundary.reference_key: %w", err)
	}
	if err := record.PlayerID.Validate(); err != nil {
		return fmt.Errorf("boundary.player_id: %w", err)
	}
	if err := record.PlanetID.Validate(); err != nil {
		return fmt.Errorf("boundary.planet_id: %w", err)
	}
	if record.Status != ClaimBoundaryStatusPendingSideEffects {
		return fmt.Errorf("boundary.status %q: %w", record.Status, ErrInvalidClaimDurableCommit)
	}
	if err := record.EventID.Validate(); err != nil {
		return fmt.Errorf("boundary.event_id: %w", err)
	}
	if record.ClaimedAt.IsZero() || record.RecordedAt.IsZero() || !record.CompletedAt.IsZero() {
		return fmt.Errorf("boundary.timestamps: %w", ErrInvalidClaimDurableCommit)
	}
	if record.StaleIntelCount < 0 || record.StaleListingCount != 0 {
		return fmt.Errorf("boundary.stale_counts: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimDurableReferenceKey(record.ClaimReference, record.ReferenceKey, record.PlayerID, record.PlanetID); err != nil {
		return fmt.Errorf("boundary.reference_key: %w", err)
	}
	return nil
}

func validateClaimDurableBeginXCore(record ClaimXCoreConsumptionRecord) error {
	if err := record.ClaimReference.Validate(); err != nil {
		return fmt.Errorf("x_core.claim_reference: %w", err)
	}
	if err := record.ReferenceKey.Validate(); err != nil {
		return fmt.Errorf("x_core.reference_key: %w", err)
	}
	if err := record.PlayerID.Validate(); err != nil {
		return fmt.Errorf("x_core.player_id: %w", err)
	}
	if err := record.PlanetID.Validate(); err != nil {
		return fmt.Errorf("x_core.planet_id: %w", err)
	}
	if err := record.SourceLocation.Validate(); err != nil {
		return fmt.Errorf("x_core.source_location: %w", err)
	}
	if record.Quantity != defaultClaimXCoreQuantity {
		return fmt.Errorf("x_core.quantity %d: %w", record.Quantity, ErrInvalidClaimDurableCommit)
	}
	if err := record.Reason.Validate(); err != nil {
		return fmt.Errorf("x_core.reason: %w", err)
	}
	if record.ConsumedAt.IsZero() {
		return fmt.Errorf("x_core.consumed_at: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimDurableReferenceKey(record.ClaimReference, record.ReferenceKey, record.PlayerID, record.PlanetID); err != nil {
		return fmt.Errorf("x_core.reference_key: %w", err)
	}
	return nil
}

func validateClaimDurableBeginPlanet(boundary ClaimBoundaryRecord, planet Planet) error {
	if err := planet.Validate(); err != nil {
		return fmt.Errorf("planet: %w", err)
	}
	if planet.ID != boundary.PlanetID ||
		planet.OwnerPlayerID != boundary.PlayerID ||
		planet.OwnerChangedAt == nil ||
		!planet.OwnerChangedAt.Equal(boundary.ClaimedAt) {
		return fmt.Errorf("planet.owner: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimDurableBeginStaleIntel(boundary ClaimBoundaryRecord, rows []PlayerPlanetIntel) error {
	if len(rows) != boundary.StaleIntelCount {
		return fmt.Errorf("stale_intel.count: %w", ErrInvalidClaimDurableCommit)
	}
	sourceReference := "planet.claimed:" + boundary.EventID.String()
	for index, row := range rows {
		if err := row.Validate(); err != nil {
			return fmt.Errorf("stale_intel[%d]: %w", index, err)
		}
		if row.PlanetID != boundary.PlanetID ||
			row.State != IntelStateStale ||
			!row.LastSeenAt.Equal(boundary.ClaimedAt) ||
			row.SourceType != IntelSourcePlanetOwnerChanged ||
			row.SourceReference != sourceReference {
			return fmt.Errorf("stale_intel[%d]: %w", index, ErrInvalidClaimDurableCommit)
		}
	}
	return nil
}

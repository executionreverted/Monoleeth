package discovery

import "fmt"

// ClaimDurableLifecyclePlan validates the completed claim row bundle a future
// durable adapter should recover as one lifecycle: begin, optional production
// initialization, and completion/outbox evidence.
type ClaimDurableLifecyclePlan struct {
	Begin                 ClaimDurableBeginPlan
	ProductionInitialized ClaimProductionInitializationDurablePlan
	Commit                ClaimDurableCommitPlan
	HasProductionInit     bool
}

// NewClaimDurableLifecyclePlan validates that independently validated claim
// begin/init/commit plans all describe the same completed planet claim. Empty
// input is a no-op plan.
func NewClaimDurableLifecyclePlan(
	begin *ClaimDurableBeginPlan,
	productionInit *ClaimProductionInitializationDurablePlan,
	commit *ClaimDurableCommitPlan,
) (ClaimDurableLifecyclePlan, error) {
	if begin == nil {
		if productionInit == nil && commit == nil {
			return ClaimDurableLifecyclePlan{}, nil
		}
		return ClaimDurableLifecyclePlan{}, fmt.Errorf("begin: %w", ErrInvalidClaimDurableCommit)
	}
	if commit == nil {
		return ClaimDurableLifecyclePlan{}, fmt.Errorf("commit: %w", ErrInvalidClaimDurableCommit)
	}
	clonedBegin := cloneClaimDurableBeginPlan(*begin)
	clonedCommit := cloneClaimDurableCommitPlan(*commit)
	if err := validateClaimDurableBeginPlan(clonedBegin); err != nil {
		return ClaimDurableLifecyclePlan{}, err
	}
	if err := validateClaimDurableLifecycleBeginCommit(clonedBegin, clonedCommit); err != nil {
		return ClaimDurableLifecyclePlan{}, err
	}

	plan := ClaimDurableLifecyclePlan{
		Begin:  clonedBegin,
		Commit: clonedCommit,
	}
	if productionInit != nil {
		clonedInit := cloneClaimProductionInitializationDurablePlan(*productionInit)
		if err := validateClaimDurableLifecycleProductionInit(clonedBegin, clonedCommit, clonedInit); err != nil {
			return ClaimDurableLifecyclePlan{}, err
		}
		plan.ProductionInitialized = clonedInit
		plan.HasProductionInit = true
	}
	return plan, nil
}

func validateClaimDurableLifecycleBeginCommit(begin ClaimDurableBeginPlan, commit ClaimDurableCommitPlan) error {
	if begin.Boundary.Status != ClaimBoundaryStatusPendingSideEffects || begin.Boundary.ClaimReference == "" {
		return fmt.Errorf("begin: %w", ErrInvalidClaimDurableCommit)
	}
	if !begin.HasXCoreStorageMutation {
		return fmt.Errorf("begin.x_core_storage: %w", ErrInvalidClaimDurableCommit)
	}
	if commit.Boundary.Status != ClaimBoundaryStatusComplete {
		return fmt.Errorf("commit: %w", ErrInvalidClaimDurableCommit)
	}
	if begin.Boundary.ClaimReference != commit.Boundary.ClaimReference ||
		begin.Boundary.ReferenceKey != commit.Boundary.ReferenceKey ||
		begin.Boundary.PlayerID != commit.Boundary.PlayerID ||
		begin.Boundary.PlanetID != commit.Boundary.PlanetID ||
		begin.Boundary.EventID != commit.Boundary.EventID ||
		begin.Boundary.StaleIntelCount != commit.Boundary.StaleIntelCount ||
		!begin.Boundary.ClaimedAt.Equal(commit.Boundary.ClaimedAt) ||
		!begin.Boundary.RecordedAt.Equal(commit.Boundary.RecordedAt) {
		return fmt.Errorf("claim_lifecycle: %w", ErrInvalidClaimDurableCommit)
	}
	if commit.Boundary.CompletedAt.Before(begin.Boundary.ClaimedAt) {
		return fmt.Errorf("claim_lifecycle.completed_at: %w", ErrInvalidClaimDurableCommit)
	}
	if begin.Planet.ID != commit.Boundary.PlanetID || begin.Planet.OwnerPlayerID != commit.Boundary.PlayerID {
		return fmt.Errorf("claim_lifecycle.planet: %w", ErrInvalidClaimDurableCommit)
	}
	if begin.XCoreConsumption.ClaimReference != commit.Boundary.ClaimReference ||
		begin.XCoreConsumption.ReferenceKey != commit.Boundary.ReferenceKey ||
		begin.XCoreConsumption.PlayerID != commit.Boundary.PlayerID ||
		begin.XCoreConsumption.PlanetID != commit.Boundary.PlanetID {
		return fmt.Errorf("claim_lifecycle.x_core: %w", ErrInvalidClaimDurableCommit)
	}
	if claimLifecycleXCorePresent(commit.XCoreConsumption) &&
		!claimLifecycleXCoreMatches(begin.XCoreConsumption, commit.XCoreConsumption) {
		return fmt.Errorf("claim_lifecycle.x_core_commit: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimDurableLifecycleProductionInit(
	begin ClaimDurableBeginPlan,
	commit ClaimDurableCommitPlan,
	productionInit ClaimProductionInitializationDurablePlan,
) error {
	init := productionInit.Initialization
	if init.ClaimReference != commit.Boundary.ClaimReference ||
		init.ReferenceKey != commit.Boundary.ReferenceKey ||
		init.PlayerID != commit.Boundary.PlayerID ||
		init.PlanetID != commit.Boundary.PlanetID ||
		init.PlanetLevel != begin.Planet.Level ||
		!init.ClaimedAt.Equal(commit.Boundary.ClaimedAt) {
		return fmt.Errorf("production_initialization: %w", ErrInvalidClaimDurableCommit)
	}
	if productionInit.Boundary.ClaimReference != "" &&
		(productionInit.Boundary.ClaimReference != begin.Boundary.ClaimReference ||
			productionInit.Boundary.ReferenceKey != begin.Boundary.ReferenceKey ||
			productionInit.Boundary.PlayerID != begin.Boundary.PlayerID ||
			productionInit.Boundary.PlanetID != begin.Boundary.PlanetID ||
			productionInit.Boundary.EventID != begin.Boundary.EventID ||
			!productionInit.Boundary.ClaimedAt.Equal(begin.Boundary.ClaimedAt)) {
		return fmt.Errorf("production_initialization.boundary: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func claimLifecycleXCorePresent(record ClaimXCoreConsumptionRecord) bool {
	return record.ClaimReference != "" ||
		record.ReferenceKey != "" ||
		!record.PlayerID.IsZero() ||
		!record.PlanetID.IsZero() ||
		record.SourceLocation.Kind != "" ||
		record.SourceLocation.ID != "" ||
		record.Quantity != 0 ||
		record.Reason != "" ||
		!record.ConsumedAt.IsZero()
}

func claimLifecycleXCoreMatches(left ClaimXCoreConsumptionRecord, right ClaimXCoreConsumptionRecord) bool {
	return left.ClaimReference == right.ClaimReference &&
		left.ReferenceKey == right.ReferenceKey &&
		left.PlayerID == right.PlayerID &&
		left.PlanetID == right.PlanetID &&
		left.SourceLocation == right.SourceLocation &&
		left.Quantity == right.Quantity &&
		left.Reason == right.Reason &&
		left.ConsumedAt.Equal(right.ConsumedAt) &&
		left.Duplicate == right.Duplicate
}

func cloneClaimDurableBeginPlan(plan ClaimDurableBeginPlan) ClaimDurableBeginPlan {
	plan.XCoreConsumption = cloneClaimXCoreConsumptionRecord(plan.XCoreConsumption)
	plan.XCoreStorageMutation = cloneClaimXCoreStorageMutationPlan(plan.XCoreStorageMutation)
	plan.Planet = clonePlanet(plan.Planet)
	plan.Boundary = cloneClaimBoundaryRecord(plan.Boundary)
	plan.StaleIntel = cloneClaimBoundaryStaleIntel(plan.StaleIntel)
	return plan
}

func cloneClaimDurableCommitPlan(plan ClaimDurableCommitPlan) ClaimDurableCommitPlan {
	plan.Boundary = cloneClaimBoundaryRecord(plan.Boundary)
	plan.Reference = cloneClaimReferenceRecord(plan.Reference)
	plan.Event = cloneClaimEventRecord(plan.Event)
	plan.Outbox = cloneClaimOutboxRecord(plan.Outbox)
	plan.XCoreConsumption = cloneClaimXCoreConsumptionRecord(plan.XCoreConsumption)
	return plan
}

func cloneClaimProductionInitializationDurablePlan(
	plan ClaimProductionInitializationDurablePlan,
) ClaimProductionInitializationDurablePlan {
	plan.Initialization = cloneClaimProductionInitializationRecord(plan.Initialization)
	plan.Boundary = cloneClaimBoundaryRecord(plan.Boundary)
	return plan
}

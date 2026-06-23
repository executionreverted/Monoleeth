package discovery

import "fmt"

// ClaimProductionInitializationRecoveryStore is the durable production-init
// adapter shape needed by a recovery worker: scan pending rows and advance them
// through the same validated apply path used by live claims.
type ClaimProductionInitializationRecoveryStore interface {
	ClaimProductionInitializationDurableReader
	ClaimProductionInitializationDurableStore
}

// ClaimDurableLifecyclePlanReader is the narrow lifecycle readback dependency
// needed to prove a pending production-init row belongs to a completed claim.
type ClaimDurableLifecyclePlanReader interface {
	CommittedClaimDurableLifecyclePlan(PlanetClaimReference) (ClaimDurableLifecyclePlan, bool, error)
}

// ClaimProductionInitializationRecoveryInput describes one bounded recovery
// pass. It intentionally has no timers or goroutines so callers can schedule it
// with their own DB lease policy.
type ClaimProductionInitializationRecoveryInput struct {
	ProductionInitializations ClaimProductionInitializationRecoveryStore
	Lifecycles                ClaimDurableLifecyclePlanReader
	Limit                     int
}

// ClaimProductionInitializationRecoveryResult reports deterministic progress
// from one bounded recovery pass.
type ClaimProductionInitializationRecoveryResult struct {
	Scanned             int
	Completed           int
	SkippedMissingClaim int
	References          []PlanetClaimReference
}

// RecoverPendingClaimProductionInitializations advances pending production-init
// durable rows once their completed claim lifecycle bundle is available.
func RecoverPendingClaimProductionInitializations(
	input ClaimProductionInitializationRecoveryInput,
) (ClaimProductionInitializationRecoveryResult, error) {
	if input.ProductionInitializations == nil || input.Lifecycles == nil {
		return ClaimProductionInitializationRecoveryResult{}, ErrInvalidClaimDurableCommit
	}
	if input.Limit <= 0 {
		return ClaimProductionInitializationRecoveryResult{}, nil
	}

	pending, err := input.ProductionInitializations.PendingClaimProductionInitializationDurablePlans(input.Limit)
	if err != nil {
		return ClaimProductionInitializationRecoveryResult{}, err
	}
	result := ClaimProductionInitializationRecoveryResult{
		Scanned:    len(pending),
		References: make([]PlanetClaimReference, 0, len(pending)),
	}
	for _, plan := range pending {
		reference := plan.Initialization.ClaimReference
		lifecycle, ok, err := input.Lifecycles.CommittedClaimDurableLifecyclePlan(reference)
		if err != nil {
			return ClaimProductionInitializationRecoveryResult{}, err
		}
		if !ok {
			result.SkippedMissingClaim++
			continue
		}
		if !lifecycle.HasProductionInit {
			return ClaimProductionInitializationRecoveryResult{}, fmt.Errorf("production_initialization: %w", ErrInvalidClaimDurableCommit)
		}
		completedInit, err := lifecycle.ProductionInitialized.Initialization.DurablePlan(&lifecycle.Commit.Boundary)
		if err != nil {
			return ClaimProductionInitializationRecoveryResult{}, err
		}
		applied, err := input.ProductionInitializations.ApplyClaimProductionInitializationDurablePlan(completedInit)
		if err != nil {
			return ClaimProductionInitializationRecoveryResult{}, err
		}
		result.Completed++
		result.References = append(result.References, applied.Plan.Initialization.ClaimReference)
	}
	return result, nil
}

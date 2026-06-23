package discovery

import (
	"fmt"
	"time"
)

// ClaimXCoreOwnerBoundary is the DB-adapter-ready boundary for the dangerous
// part of planet claim: X Core debit plus unowned-planet owner CAS. Durable
// implementations must couple the idempotency key, inventory ledger decrease,
// owner transition, stale-intel mutation, and pending claim boundary row in one
// transaction or recoverable state machine.
type ClaimXCoreOwnerBoundary interface {
	BeginPlanetClaimWithXCore(BeginPlanetClaimWithXCoreInput) (BeginPlanetClaimWithXCoreResult, error)
}

// BeginPlanetClaimWithXCoreInput carries the server-derived debit and owner
// transition inputs for one claim reference.
type BeginPlanetClaimWithXCoreInput struct {
	XCore      ClaimXCoreConsumeInput
	Boundary   BeginPlanetClaimBoundaryInput
	ConsumedAt time.Time
}

// BeginPlanetClaimWithXCoreResult reports both sides of the coupled claim begin.
// XCoreConsumption may be populated when the debit committed but the owner
// boundary failed, letting retry paths avoid a second debit call.
type BeginPlanetClaimWithXCoreResult struct {
	XCoreResult      ClaimXCoreConsumeResult
	XCoreConsumption ClaimXCoreConsumptionRecord
	Boundary         BeginPlanetClaimBoundaryResult
}

type composedClaimXCoreOwnerBoundary struct {
	Consumer   ClaimXCoreConsumer
	Boundaries ClaimBoundaryStore
}

func (boundary composedClaimXCoreOwnerBoundary) BeginPlanetClaimWithXCore(
	input BeginPlanetClaimWithXCoreInput,
) (BeginPlanetClaimWithXCoreResult, error) {
	if boundary.Consumer == nil || boundary.Boundaries == nil {
		return BeginPlanetClaimWithXCoreResult{}, ErrInvalidClaimConfig
	}
	if err := input.Validate(); err != nil {
		return BeginPlanetClaimWithXCoreResult{}, err
	}

	xcore, err := boundary.Consumer.ConsumeClaimXCore(input.XCore)
	if err != nil {
		return BeginPlanetClaimWithXCoreResult{}, err
	}
	consumption := newClaimXCoreConsumptionRecord(input.XCore, xcore, input.ConsumedAt)
	claimBoundary, err := boundary.Boundaries.BeginPlanetClaimBoundary(input.Boundary)
	return BeginPlanetClaimWithXCoreResult{
		XCoreResult:      xcore,
		XCoreConsumption: consumption,
		Boundary:         claimBoundary,
	}, err
}

func (input BeginPlanetClaimWithXCoreInput) Validate() error {
	if err := input.XCore.Validate(); err != nil {
		return err
	}
	if err := input.Boundary.Validate(); err != nil {
		return err
	}
	if input.XCore.Reference != input.Boundary.ClaimReference ||
		input.XCore.PlayerID != input.Boundary.PlayerID ||
		input.XCore.PlanetID != input.Boundary.PlanetID {
		return ErrPlanetClaimReferenceConflict
	}
	if input.ConsumedAt.IsZero() {
		return fmt.Errorf("consumed_at: %w", ErrInvalidClaimXCoreConsume)
	}
	return nil
}

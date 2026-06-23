package server

import (
	"errors"
	"time"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/production"
)

var errInvalidRuntimeDurableOutbox = errors.New("invalid runtime durable outbox")

type RuntimeDurableOutboxDrainInput struct {
	// Limit is applied independently to each durable outbox store.
	Limit int

	Now                                   time.Time
	ReleaseExpiredLeases                  bool
	LeaseTimeout                          time.Duration
	RetryFailedOutboxes                   bool
	RecoverClaimProductionInitializations bool

	PublishClaim            discovery.ClaimOutboxPublishFunc
	PublishSettlement       production.ProductionOutboxPublishFunc
	PublishBuildingMutation production.ProductionOutboxPublishFunc
}

type RuntimeDurableOutboxDrainResult struct {
	ReleasedClaims            []discovery.ClaimOutboxRecord
	ReleasedSettlements       []production.ProductionOutboxRecord
	ReleasedBuildingMutations []production.ProductionOutboxRecord

	RetriedClaims            []discovery.ClaimOutboxRecord
	RetriedSettlements       []production.ProductionOutboxRecord
	RetriedBuildingMutations []production.ProductionOutboxRecord

	RecoveredClaimProductionInitializations discovery.ClaimProductionInitializationRecoveryResult

	Claims            []discovery.ClaimOutboxPublishResult
	Settlements       []production.ProductionOutboxPublishResult
	BuildingMutations []production.ProductionOutboxPublishResult
}

type runtimeDurableOutboxLeaseReleaseInput struct {
	Limit         int
	ClaimedBefore time.Time
	ReleasedAt    time.Time
}

type runtimeDurableOutboxLeaseReleaseResult struct {
	Claim            []discovery.ClaimOutboxRecord
	Settlement       []production.ProductionOutboxRecord
	BuildingMutation []production.ProductionOutboxRecord
}

type runtimeDurableOutboxRetryResult struct {
	Claim            []discovery.ClaimOutboxRecord
	Settlement       []production.ProductionOutboxRecord
	BuildingMutation []production.ProductionOutboxRecord
}

// DrainDurableOutboxes drains committed runtime-owned durable outbox stores
// through their publisher contracts. The publish callbacks are infrastructure
// hooks; gameplay state has already been committed by the authoritative domain
// services.
func (runtime *Runtime) DrainDurableOutboxes(
	input RuntimeDurableOutboxDrainInput,
) (RuntimeDurableOutboxDrainResult, error) {
	if runtime == nil {
		return RuntimeDurableOutboxDrainResult{}, errInvalidRuntimeDurableOutbox
	}
	now := input.Now.UTC()
	if now.IsZero() && runtime.clock != nil {
		now = runtime.clock.Now().UTC()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	var result RuntimeDurableOutboxDrainResult
	if input.RecoverClaimProductionInitializations {
		recovered, err := runtime.recoverPendingClaimProductionInitializations(input.Limit)
		if err != nil {
			return result, err
		}
		result.RecoveredClaimProductionInitializations = recovered
	}
	if input.ReleaseExpiredLeases && input.LeaseTimeout > 0 {
		released, err := runtime.releaseExpiredDurableOutboxLeases(runtimeDurableOutboxLeaseReleaseInput{
			Limit:         input.Limit,
			ClaimedBefore: now.Add(-input.LeaseTimeout),
			ReleasedAt:    now,
		})
		if err != nil {
			return result, err
		}
		result.ReleasedClaims = released.Claim
		result.ReleasedSettlements = released.Settlement
		result.ReleasedBuildingMutations = released.BuildingMutation
	}
	if input.RetryFailedOutboxes {
		retried, err := runtime.retryFailedDurableOutboxes(input.Limit, now)
		if err != nil {
			return result, err
		}
		result.RetriedClaims = retried.Claim
		result.RetriedSettlements = retried.Settlement
		result.RetriedBuildingMutations = retried.BuildingMutation
	}

	if input.PublishClaim != nil {
		claim, err := runtime.publishPendingClaimDurableOutbox(input.Limit, now, now, input.PublishClaim)
		if err != nil {
			return result, err
		}
		result.Claims = claim
	}
	if input.PublishSettlement != nil {
		settlement, err := runtime.publishPendingSettlementDurableOutbox(input.Limit, now, now, input.PublishSettlement)
		if err != nil {
			return result, err
		}
		result.Settlements = settlement
	}
	if input.PublishBuildingMutation != nil {
		building, err := runtime.publishPendingBuildingMutationDurableOutbox(input.Limit, now, now, input.PublishBuildingMutation)
		if err != nil {
			return result, err
		}
		result.BuildingMutations = building
	}
	return result, nil
}

func (runtime *Runtime) recoverPendingClaimProductionInitializations(
	limit int,
) (discovery.ClaimProductionInitializationRecoveryResult, error) {
	if runtime == nil {
		return discovery.ClaimProductionInitializationRecoveryResult{}, errInvalidRuntimeDurableOutbox
	}
	if runtime.ClaimProductionInitializations == nil || runtime.ClaimLifecycles == nil {
		return discovery.ClaimProductionInitializationRecoveryResult{}, nil
	}
	return discovery.RecoverPendingClaimProductionInitializations(discovery.ClaimProductionInitializationRecoveryInput{
		ProductionInitializations: runtime.ClaimProductionInitializations,
		Lifecycles:                runtime.ClaimLifecycles,
		Limit:                     limit,
	})
}

func (runtime *Runtime) retryFailedDurableOutboxes(
	limit int,
	retriedAt time.Time,
) (runtimeDurableOutboxRetryResult, error) {
	if runtime == nil {
		return runtimeDurableOutboxRetryResult{}, errInvalidRuntimeDurableOutbox
	}
	claim, err := runtime.retryFailedClaimDurableOutbox(limit, retriedAt)
	if err != nil {
		return runtimeDurableOutboxRetryResult{}, err
	}
	settlement, err := runtime.retryFailedSettlementDurableOutbox(limit, retriedAt)
	if err != nil {
		return runtimeDurableOutboxRetryResult{Claim: claim}, err
	}
	building, err := runtime.retryFailedBuildingMutationDurableOutbox(limit, retriedAt)
	if err != nil {
		return runtimeDurableOutboxRetryResult{Claim: claim, Settlement: settlement}, err
	}
	return runtimeDurableOutboxRetryResult{
		Claim:            claim,
		Settlement:       settlement,
		BuildingMutation: building,
	}, nil
}

func (runtime *Runtime) publishPendingClaimDurableOutbox(
	limit int,
	claimedAt time.Time,
	completedAt time.Time,
	publish discovery.ClaimOutboxPublishFunc,
) ([]discovery.ClaimOutboxPublishResult, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.ClaimLifecycles == nil {
		return nil, nil
	}
	return discovery.PublishPendingClaimOutbox(discovery.ClaimOutboxPublishInput{
		Store:       runtime.ClaimLifecycles,
		Limit:       limit,
		ClaimedAt:   claimedAt,
		CompletedAt: completedAt,
		Publish:     publish,
	})
}

func (runtime *Runtime) publishPendingSettlementDurableOutbox(
	limit int,
	claimedAt time.Time,
	completedAt time.Time,
	publish production.ProductionOutboxPublishFunc,
) ([]production.ProductionOutboxPublishResult, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.Settlements == nil {
		return nil, nil
	}
	return production.PublishPendingProductionOutbox(production.ProductionOutboxPublishInput{
		Store:       runtime.Settlements,
		Limit:       limit,
		ClaimedAt:   claimedAt,
		CompletedAt: completedAt,
		Publish:     publish,
	})
}

func (runtime *Runtime) publishPendingBuildingMutationDurableOutbox(
	limit int,
	claimedAt time.Time,
	completedAt time.Time,
	publish production.ProductionOutboxPublishFunc,
) ([]production.ProductionOutboxPublishResult, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.BuildingMutations == nil {
		return nil, nil
	}
	return production.PublishPendingProductionOutbox(production.ProductionOutboxPublishInput{
		Store:       runtime.BuildingMutations,
		Limit:       limit,
		ClaimedAt:   claimedAt,
		CompletedAt: completedAt,
		Publish:     publish,
	})
}

func (runtime *Runtime) releaseExpiredDurableOutboxLeases(
	input runtimeDurableOutboxLeaseReleaseInput,
) (runtimeDurableOutboxLeaseReleaseResult, error) {
	if runtime == nil {
		return runtimeDurableOutboxLeaseReleaseResult{}, errInvalidRuntimeDurableOutbox
	}
	claim, err := runtime.releaseExpiredClaimDurableOutboxLeases(input.Limit, input.ClaimedBefore, input.ReleasedAt)
	if err != nil {
		return runtimeDurableOutboxLeaseReleaseResult{}, err
	}
	settlement, err := runtime.releaseExpiredSettlementDurableOutboxLeases(input.Limit, input.ClaimedBefore, input.ReleasedAt)
	if err != nil {
		return runtimeDurableOutboxLeaseReleaseResult{Claim: claim}, err
	}
	building, err := runtime.releaseExpiredBuildingMutationDurableOutboxLeases(input.Limit, input.ClaimedBefore, input.ReleasedAt)
	if err != nil {
		return runtimeDurableOutboxLeaseReleaseResult{Claim: claim, Settlement: settlement}, err
	}
	return runtimeDurableOutboxLeaseReleaseResult{
		Claim:            claim,
		Settlement:       settlement,
		BuildingMutation: building,
	}, nil
}

func (runtime *Runtime) releaseExpiredClaimDurableOutboxLeases(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]discovery.ClaimOutboxRecord, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.ClaimLifecycles == nil {
		return nil, nil
	}
	return discovery.ReleaseExpiredClaimOutboxLeases(discovery.ClaimOutboxLeaseReleaseInput{
		Store:         runtime.ClaimLifecycles,
		Limit:         limit,
		ClaimedBefore: claimedBefore,
		ReleasedAt:    releasedAt,
	})
}

func (runtime *Runtime) releaseExpiredSettlementDurableOutboxLeases(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.Settlements == nil {
		return nil, nil
	}
	return production.ReleaseExpiredProductionOutboxLeases(production.ProductionOutboxLeaseReleaseInput{
		Store:         runtime.Settlements,
		Limit:         limit,
		ClaimedBefore: claimedBefore,
		ReleasedAt:    releasedAt,
	})
}

func (runtime *Runtime) releaseExpiredBuildingMutationDurableOutboxLeases(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.BuildingMutations == nil {
		return nil, nil
	}
	return production.ReleaseExpiredProductionOutboxLeases(production.ProductionOutboxLeaseReleaseInput{
		Store:         runtime.BuildingMutations,
		Limit:         limit,
		ClaimedBefore: claimedBefore,
		ReleasedAt:    releasedAt,
	})
}

func (runtime *Runtime) retryFailedClaimDurableOutbox(
	limit int,
	retriedAt time.Time,
) ([]discovery.ClaimOutboxRecord, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.ClaimLifecycles == nil {
		return nil, nil
	}
	return discovery.RetryFailedClaimOutboxRows(discovery.ClaimOutboxRetryInput{
		Store:     runtime.ClaimLifecycles,
		Limit:     limit,
		RetriedAt: retriedAt,
	})
}

func (runtime *Runtime) retryFailedSettlementDurableOutbox(
	limit int,
	retriedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.Settlements == nil {
		return nil, nil
	}
	return production.RetryFailedProductionOutboxRows(production.ProductionOutboxRetryInput{
		Store:     runtime.Settlements,
		Limit:     limit,
		RetriedAt: retriedAt,
	})
}

func (runtime *Runtime) retryFailedBuildingMutationDurableOutbox(
	limit int,
	retriedAt time.Time,
) ([]production.ProductionOutboxRecord, error) {
	if runtime == nil {
		return nil, errInvalidRuntimeDurableOutbox
	}
	if runtime.BuildingMutations == nil {
		return nil, nil
	}
	return production.RetryFailedProductionOutboxRows(production.ProductionOutboxRetryInput{
		Store:     runtime.BuildingMutations,
		Limit:     limit,
		RetriedAt: retriedAt,
	})
}

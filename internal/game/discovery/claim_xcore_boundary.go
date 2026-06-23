package discovery

import (
	"sort"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// ClaimXCoreConsumptionRecord is the process-local boundary marker for the X
// Core debit that authorizes a planet claim. Future durable adapters should
// store this evidence with the same claim reference used by owner-CAS.
type ClaimXCoreConsumptionRecord struct {
	ClaimReference PlanetClaimReference
	ReferenceKey   foundation.IdempotencyKey
	PlayerID       foundation.PlayerID
	PlanetID       foundation.PlanetID
	SourceLocation economy.ItemLocation
	Quantity       int64
	Reason         economy.LedgerReason
	ConsumedAt     time.Time
	Duplicate      bool
}

// ClaimXCoreConsumptions returns recorded X Core consumption evidence in
// deterministic claim-reference order.
func (service *ClaimService) ClaimXCoreConsumptions() []ClaimXCoreConsumptionRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	if len(service.xCoreConsumptions) == 0 {
		return nil
	}
	refs := make([]PlanetClaimReference, 0, len(service.xCoreConsumptions))
	for ref := range service.xCoreConsumptions {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i] < refs[j]
	})
	records := make([]ClaimXCoreConsumptionRecord, 0, len(refs))
	for _, ref := range refs {
		records = append(records, cloneClaimXCoreConsumptionRecord(service.xCoreConsumptions[ref]))
	}
	return records
}

func (service *ClaimService) claimXCoreAlreadyConsumedLocked(input ClaimPlanetInput) (bool, error) {
	record, ok := service.xCoreConsumptions[input.ClaimReference]
	if !ok {
		return false, nil
	}
	if record.PlayerID != input.PlayerID || record.PlanetID != input.PlanetID {
		return false, ErrPlanetClaimReferenceConflict
	}
	return true, nil
}

func (service *ClaimService) recordClaimXCoreConsumptionRecordLocked(record ClaimXCoreConsumptionRecord) {
	service.xCoreConsumptions[record.ClaimReference] = cloneClaimXCoreConsumptionRecord(record)
}

func newClaimXCoreConsumptionRecord(input ClaimXCoreConsumeInput, result ClaimXCoreConsumeResult, consumedAt time.Time) ClaimXCoreConsumptionRecord {
	referenceKey, _ := input.Reference.IdempotencyKey(input.PlayerID, input.PlanetID)
	return ClaimXCoreConsumptionRecord{
		ClaimReference: input.Reference,
		ReferenceKey:   referenceKey,
		PlayerID:       input.PlayerID,
		PlanetID:       input.PlanetID,
		SourceLocation: input.SourceLocation,
		Quantity:       input.Quantity,
		Reason:         input.Reason,
		ConsumedAt:     consumedAt.UTC(),
		Duplicate:      result.Duplicate,
	}
}

func cloneClaimXCoreConsumptionRecord(record ClaimXCoreConsumptionRecord) ClaimXCoreConsumptionRecord {
	record.ConsumedAt = record.ConsumedAt.UTC()
	return record
}

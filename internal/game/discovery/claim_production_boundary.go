package discovery

import (
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

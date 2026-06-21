package server

import (
	"errors"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world/worker"
)

const (
	runtimePlanetClaimRange               = 300.0
	runtimeClaimProductionStorageCapacity = 250
	runtimeClaimProductionEnergyCapacity  = 40
)

type runtimeClaimRankProvider struct {
	progression *progression.ProgressionService
}

func (provider runtimeClaimRankProvider) PlayerClaimRank(input discovery.ClaimRankInput) (discovery.ClaimRankResult, error) {
	if err := input.Validate(); err != nil {
		return discovery.ClaimRankResult{}, err
	}
	if provider.progression == nil {
		return discovery.ClaimRankResult{}, errors.New("nil progression claim rank provider")
	}
	snapshot, err := provider.progression.GetProgressionSnapshot(input.PlayerID)
	if err != nil {
		return discovery.ClaimRankResult{}, err
	}
	return discovery.ClaimRankResult{Rank: snapshot.Player.Rank}, nil
}

type runtimeClaimProximityProvider struct {
	runtime *Runtime
}

func (provider runtimeClaimProximityProvider) PlayerCanClaimPlanet(input discovery.ClaimProximityInput) (discovery.ClaimProximityResult, error) {
	if err := input.Validate(); err != nil {
		return discovery.ClaimProximityResult{}, err
	}
	if provider.runtime == nil {
		return discovery.ClaimProximityResult{}, errors.New("nil runtime claim proximity provider")
	}

	provider.runtime.mu.Lock()
	defer provider.runtime.mu.Unlock()

	if err := provider.runtime.validateNoActiveTransferLocked(input.PlayerID); err != nil {
		return discovery.ClaimProximityResult{}, err
	}
	location, err := provider.runtime.mapRouter.ActiveLocation(input.PlayerID)
	if err != nil {
		return discovery.ClaimProximityResult{}, err
	}
	if location.WorldID != input.WorldID || location.ZoneID != input.ZoneID {
		return discovery.ClaimProximityResult{WithinRange: false}, nil
	}
	instance, err := provider.runtime.mapInstanceForLocationLocked(location)
	if err != nil {
		return discovery.ClaimProximityResult{}, err
	}
	entity, ok := instance.Worker.PlayerEntity(input.PlayerID)
	if !ok {
		return discovery.ClaimProximityResult{}, worker.ErrUnknownPlayer
	}
	if entity.WorldID != input.WorldID || entity.ZoneID != input.ZoneID {
		return discovery.ClaimProximityResult{WithinRange: false}, nil
	}
	return discovery.ClaimProximityResult{
		WithinRange: entity.Position.Distance(input.PlanetCoordinates) <= runtimePlanetClaimRange,
	}, nil
}

type runtimeClaimXCoreConsumer struct {
	inventory *economy.InventoryService
}

func (consumer runtimeClaimXCoreConsumer) ConsumeClaimXCore(input discovery.ClaimXCoreConsumeInput) (discovery.ClaimXCoreConsumeResult, error) {
	if err := input.Validate(); err != nil {
		return discovery.ClaimXCoreConsumeResult{}, err
	}
	if consumer.inventory == nil {
		return discovery.ClaimXCoreConsumeResult{}, errors.New("nil claim x core inventory consumer")
	}
	referenceKey, err := foundation.ParseIdempotencyKey(string(input.Reference))
	if err != nil {
		return discovery.ClaimXCoreConsumeResult{}, err
	}
	result, err := consumer.inventory.SystemRemoveItem(economy.RemoveItemInput{
		PlayerID:       input.PlayerID,
		ItemRef:        input.ItemRef,
		SourceLocation: input.SourceLocation,
		Quantity:       input.Quantity,
		Reason:         input.Reason,
		ReferenceKey:   referenceKey,
	})
	if err != nil {
		return discovery.ClaimXCoreConsumeResult{}, err
	}
	return discovery.ClaimXCoreConsumeResult{Duplicate: result.Duplicate}, nil
}

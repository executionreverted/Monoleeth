package server

import (
	"encoding/json"
	"errors"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

type planetClaimedPayload struct {
	Accepted           bool               `json:"accepted"`
	Duplicate          bool               `json:"duplicate,omitempty"`
	AlreadyOwned       bool               `json:"already_owned,omitempty"`
	Planet             knownPlanetPayload `json:"planet"`
	ClaimedAt          int64              `json:"claimed_at,omitempty"`
	StaleIntelCount    int                `json:"stale_intel_count,omitempty"`
	StaleListingCount  int                `json:"stale_listing_count,omitempty"`
	ProductionIncluded bool               `json:"production_included"`
}

type planetClaimResponsePayload struct {
	Claim        planetClaimedPayload              `json:"claim"`
	KnownPlanets knownPlanetsPayload               `json:"known_planets"`
	PlanetDetail planetDetailPayload               `json:"planet_detail"`
	Production   planetProductionCollectionPayload `json:"production"`
	Inventory    inventorySnapshotPayload          `json:"inventory"`
}

func (runtime *Runtime) handleClaimPlanet(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"position",
		"coordinates",
		"owner",
		"owner_player_id",
		"x_core",
		"production",
		"inventory",
		"storage",
		"claim_reference",
	); err != nil {
		return nil, err
	}
	var payload struct {
		PlanetID string `json:"planet_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	planetID, err := foundation.ParsePlanetID(payload.PlanetID)
	if err != nil {
		return nil, invalidPayload("Planet is invalid.", err)
	}
	if runtime.Claim == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Planet claim service is unavailable.")
	}
	claimReference, err := planetClaimReference(ctx.PlayerID, planetID)
	if err != nil {
		return nil, err
	}
	runtime.beginPlanetClaimMarketGuard(planetID)
	defer runtime.endPlanetClaimMarketGuard(planetID)

	result, err := runtime.Claim.ClaimPlanet(discovery.ClaimPlanetInput{
		PlayerID:       ctx.PlayerID,
		PlanetID:       planetID,
		ClaimReference: claimReference,
	})
	if err != nil {
		if applyErr := runtime.applyClaimProductionInitializationDurablePlan(claimReference); applyErr != nil {
			return nil, domainErrorForClaim(applyErr)
		}
		return nil, domainErrorForClaim(err)
	}
	if err := runtime.applyClaimDurableLifecycle(claimReference); err != nil {
		return nil, domainErrorForClaim(err)
	}

	knownPlanets, err := runtime.knownPlanetsPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForClaim(err)
	}
	detail, err := runtime.planetDetailPayload(ctx.PlayerID, planetID)
	if err != nil {
		return nil, domainErrorForClaim(err)
	}
	productionPayload, err := runtime.productionSummaryPayload(ctx.PlayerID, planetID)
	if err != nil {
		return nil, domainErrorForClaim(err)
	}
	claimPayload := planetClaimedPayloadFromResult(result, detail.knownPlanetPayload, len(productionPayload.Planets) > 0)

	runtime.mu.Lock()
	inventory := runtime.inventorySnapshotLocked(ctx.PlayerID)
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventPlanetClaimed, claimPayload)
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventKnownPlanets, knownPlanets)
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventPlanetDetail, detail)
	if len(productionPayload.Planets) > 0 {
		runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventProductionSummary, productionPayload)
	}
	if result.StaleListingCount > 0 && !result.Duplicate {
		runtime.queueClaimStaleMarketListingEventsLocked(planetID)
	}
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventInventorySnapshot, inventory)
	runtime.mu.Unlock()

	return marshalPayload(planetClaimResponsePayload{
		Claim:        claimPayload,
		KnownPlanets: knownPlanets,
		PlanetDetail: detail,
		Production:   productionPayload,
		Inventory:    inventory,
	})
}

func (runtime *Runtime) applyClaimProductionInitializationDurablePlan(reference discovery.PlanetClaimReference) error {
	if runtime.Claim == nil || runtime.ClaimProductionInitializations == nil {
		return nil
	}
	plan, ok, err := runtime.Claim.ClaimProductionInitializationDurablePlan(reference)
	if err != nil || !ok {
		return err
	}
	_, err = plan.ApplyDurableProductionInitialization(runtime.ClaimProductionInitializations)
	return err
}

func (runtime *Runtime) applyClaimDurableLifecycle(reference discovery.PlanetClaimReference) error {
	if runtime.Claim == nil || runtime.ClaimLifecycles == nil {
		return nil
	}
	plan, ok, err := runtime.Claim.ClaimDurableLifecyclePlan(reference)
	if err != nil || !ok {
		return err
	}
	if plan.HasProductionInit {
		if err := runtime.applyClaimProductionInitializationDurablePlan(reference); err != nil {
			return err
		}
	}
	_, err = plan.ApplyDurableLifecycle(runtime.ClaimLifecycles)
	return err
}

func planetClaimReference(playerID foundation.PlayerID, planetID foundation.PlanetID) (discovery.PlanetClaimReference, error) {
	key, err := foundation.PlanetClaimIdempotencyKey(playerID, planetID)
	if err != nil {
		return "", err
	}
	return discovery.PlanetClaimReference(key.String()), nil
}

func (runtime *Runtime) beginPlanetClaimMarketGuard(planetID foundation.PlanetID) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.activePlanetClaims[planetID]++
}

func (runtime *Runtime) endPlanetClaimMarketGuard(planetID foundation.PlanetID) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.activePlanetClaims[planetID] <= 1 {
		delete(runtime.activePlanetClaims, planetID)
		return
	}
	runtime.activePlanetClaims[planetID]--
}

func planetClaimedPayloadFromResult(result discovery.ClaimPlanetResult, planet knownPlanetPayload, productionIncluded bool) planetClaimedPayload {
	payload := planetClaimedPayload{
		Accepted:           result.Claimed,
		Duplicate:          result.Duplicate,
		AlreadyOwned:       result.AlreadyOwned,
		Planet:             planet,
		StaleIntelCount:    result.StaleIntelCount,
		StaleListingCount:  result.StaleListingCount,
		ProductionIncluded: productionIncluded,
	}
	if result.Planet.OwnerChangedAt != nil {
		payload.ClaimedAt = result.Planet.OwnerChangedAt.UTC().UnixMilli()
	}
	return payload
}

func domainErrorForClaim(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, discovery.ErrPlanetClaimRequiresIntel),
		errors.Is(err, discovery.ErrPlanetClaimIntelInvalidated),
		errors.Is(err, discovery.ErrUnknownPlanet):
		return foundation.NewDomainError(foundation.CodeNotFound, "Planet was not found.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrPlanetAlreadyOwned):
		return foundation.NewDomainError(foundation.CodeForbidden, "Planet is already claimed.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrPlanetClaimProximity):
		return foundation.NewDomainError(foundation.CodeOutOfRange, "Planet is out of claim range.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrPlanetClaimRankTooLow):
		return foundation.NewDomainError(foundation.CodeRankTooLow, "Pilot rank is too low.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrItemNotOwned), errors.Is(err, economy.ErrInsufficientItemQuantity):
		return foundation.NewDomainError(foundation.CodeForbidden, "Required X Core is unavailable.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrPlanetClaimReferenceConflict),
		errors.Is(err, discovery.ErrInvalidPlanetClaim),
		errors.Is(err, discovery.ErrInvalidClaimXCoreConsume),
		errors.Is(err, discovery.ErrInvalidClaimXCoreSource):
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Planet claim request is invalid.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Planet claim failed.", foundation.WithCause(err))
	}
}

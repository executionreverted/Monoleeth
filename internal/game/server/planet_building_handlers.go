package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
)

type planetBuildingBuildIntent struct {
	PlanetID     string `json:"planet_id"`
	BuildingType string `json:"building_type"`
	Slot         string `json:"slot"`
}

type planetBuildingUpgradeIntent struct {
	PlanetID    string `json:"planet_id"`
	BuildingID  string `json:"building_id"`
	TargetLevel int    `json:"target_level,omitempty"`
	NextLevel   int    `json:"next_level,omitempty"`
}

func (runtime *Runtime) handlePlanetBuildingBuild(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"owner",
		"owner_player_id",
		"building_id",
		"definition_id",
		"level",
		"catalog",
		"source",
		"production",
		"storage",
		"materials",
		"material",
		"wallet",
		"cost",
		"server_cost",
		"truth",
		"position",
		"coordinates",
	); err != nil {
		return nil, err
	}

	var intent planetBuildingBuildIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	planetID, err := foundation.ParsePlanetID(intent.PlanetID)
	if err != nil {
		return nil, invalidPayload("Planet is invalid.", err)
	}
	buildingType := production.BuildingType(strings.TrimSpace(intent.BuildingType))
	if err := buildingType.Validate(); err != nil {
		return nil, invalidPayload("Building type is invalid.", err)
	}
	slot, err := validatePlanetBuildingSlot(intent.Slot)
	if err != nil {
		return nil, invalidPayload("Building slot is invalid.", err)
	}
	buildingID, err := deterministicPlanetBuildingID(planetID, buildingType, slot)
	if err != nil {
		return nil, invalidPayload("Building slot is invalid.", err)
	}
	referenceKey, err := foundation.PlanetBuildingBuildIdempotencyKey(planetID, buildingID.String())
	if err != nil {
		return nil, invalidPayload("Building reference is invalid.", err)
	}

	now := runtime.clock.Now().UTC()
	runtime.buildingMutationMu.Lock()
	defer runtime.buildingMutationMu.Unlock()

	if err := runtime.validateOwnedActiveProductionPlanet(ctx.PlayerID, planetID); err != nil {
		return nil, err
	}
	if _, _, _, err := runtime.settleOwnedProductionForSummary(ctx.PlayerID, planetID, now); err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}

	service, err := runtime.newPlanetBuildingMutationService(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	result, err := service.BuildPlanetBuilding(production.BuildPlanetBuildingInput{
		PlanetID:     planetID,
		BuildingID:   buildingID,
		BuildingType: buildingType,
		Level:        1,
		RequestedAt:  now,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	if err := runtime.applyBuildingMutationDurableCommit(result); err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	questUpdates, err := runtime.consumeBuildingQuestProgress(ctx.PlayerID, request.RequestID, result)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	response, err := runtime.planetBuildingMutationResponse(ctx.PlayerID, planetID, result)
	if err != nil {
		return nil, err
	}
	runtime.queueQuestProgressEvents(authSessionID(ctx.SessionID), questUpdates)
	return response, nil
}

func (runtime *Runtime) handlePlanetBuildingUpgrade(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated player is required.", foundation.WithCause(err))
	}
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"owner",
		"owner_player_id",
		"building_type",
		"slot",
		"definition_id",
		"level",
		"catalog",
		"source",
		"production",
		"storage",
		"materials",
		"material",
		"wallet",
		"cost",
		"server_cost",
		"truth",
		"position",
		"coordinates",
	); err != nil {
		return nil, err
	}

	var intent planetBuildingUpgradeIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	planetID, err := foundation.ParsePlanetID(intent.PlanetID)
	if err != nil {
		return nil, invalidPayload("Planet is invalid.", err)
	}
	buildingID := production.BuildingID(strings.TrimSpace(intent.BuildingID))
	if buildingID.String() != intent.BuildingID {
		return nil, invalidPayload("Building is invalid.", production.ErrInvalidBuildingID)
	}
	if err := buildingID.Validate(); err != nil {
		return nil, invalidPayload("Building is invalid.", err)
	}
	targetLevel, err := buildingUpgradeTargetLevel(intent)
	if err != nil {
		return nil, invalidPayload("Building target level is invalid.", err)
	}
	referenceKey, err := foundation.PlanetBuildingUpgradeIdempotencyKey(planetID, buildingID.String(), targetLevel)
	if err != nil {
		return nil, invalidPayload("Building reference is invalid.", err)
	}

	now := runtime.clock.Now().UTC()
	runtime.buildingMutationMu.Lock()
	defer runtime.buildingMutationMu.Unlock()

	if err := runtime.validateOwnedActiveProductionPlanet(ctx.PlayerID, planetID); err != nil {
		return nil, err
	}
	if _, _, _, err := runtime.settleOwnedProductionForSummary(ctx.PlayerID, planetID, now); err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}

	service, err := runtime.newPlanetBuildingMutationService(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	result, err := service.UpgradePlanetBuilding(production.UpgradePlanetBuildingInput{
		PlanetID:     planetID,
		BuildingID:   buildingID,
		NextLevel:    targetLevel,
		RequestedAt:  now,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	if err := runtime.applyBuildingMutationDurableCommit(result); err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	return runtime.planetBuildingMutationResponse(ctx.PlayerID, planetID, result)
}

func (runtime *Runtime) newPlanetBuildingMutationService(playerID foundation.PlayerID) (*production.BuildingMutationService, error) {
	if runtime == nil || runtime.Production == nil {
		return nil, production.ErrInvalidBuildingMutationConfig
	}
	if len(runtime.ProductionCatalog.Definitions()) == 0 {
		return nil, production.ErrInvalidProductionCatalog
	}
	return production.NewBuildingMutationService(production.BuildingMutationServiceConfig{
		Store:   runtime.Production,
		Catalog: runtime.ProductionCatalog,
		Costs:   runtimePlanetBuildingCostProvider{playerID: playerID, rules: runtime.productionRules},
		Wallet:  runtime.Wallet,
	})
}

func (runtime *Runtime) applyBuildingMutationDurableCommit(result production.BuildingMutationResult) error {
	if runtime.Production == nil || runtime.BuildingMutations == nil || result.ReferenceKey.IsZero() {
		return nil
	}
	reference, ok, err := runtime.Production.BuildingMutationReference(result.ReferenceKey)
	if err != nil {
		return err
	}
	if !ok {
		return production.ErrInvalidBuildingMutationDurableCommit
	}
	if len(reference.Result.OutboxRecords) == 0 {
		return nil
	}
	plan, err := production.NewBuildingMutationDurableCommitPlan(
		&reference,
		reference.Result.OutboxRecords,
		reference.Result.MaterialLedger,
	)
	if err != nil {
		return err
	}
	_, err = plan.ApplyDurableCommit(runtime.BuildingMutations)
	return err
}

func (runtime *Runtime) consumeBuildingQuestProgress(
	playerID foundation.PlayerID,
	requestID foundation.RequestID,
	result production.BuildingMutationResult,
) ([]quests.PlayerQuest, error) {
	if result.Operation != production.BuildingMutationBuild {
		return nil, nil
	}
	if result.ReferenceKey.IsZero() {
		return nil, nil
	}
	return runtime.Quest.ConsumeBuildingCompleted(quests.BuildingCompletedInput{
		EventID:          foundation.EventID("event-building-build-" + requestID.String()),
		ProgressEventKey: quests.QuestProgressEventKey("building.completed:" + result.ReferenceKey.String()),
		PlayerID:         playerID,
		BuildingType:     result.Building.BuildingType.String(),
	})
}

func (runtime *Runtime) validateOwnedActiveProductionPlanet(playerID foundation.PlayerID, planetID foundation.PlanetID) error {
	scope, err := runtime.knownPlanetMapScope(playerID)
	if err != nil {
		return domainErrorForRuntime(err)
	}
	planet, ok, err := runtime.Discovery.Planet(planetID)
	if err != nil {
		return domainErrorForBuildingMutation(err)
	}
	if !ok || planet.OwnerPlayerID != playerID {
		return foundation.NewDomainError(foundation.CodeNotFound, "Planet was not found.")
	}
	if planet.WorldID != scope.worldID || planet.ZoneID != scope.zoneID {
		return foundation.NewDomainError(foundation.CodeNotFound, "Planet was not found.")
	}
	if _, ok, err := runtime.Production.Snapshot(planetID); err != nil {
		return domainErrorForBuildingMutation(err)
	} else if !ok {
		return foundation.NewDomainError(foundation.CodeNotFound, "Planet production was not found.")
	}
	return nil
}

func (runtime *Runtime) planetBuildingMutationResponse(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	result production.BuildingMutationResult,
) (json.RawMessage, error) {
	productionPayload, err := runtime.productionSummaryPayload(playerID, planetID)
	if err != nil {
		return nil, domainErrorForBuildingMutation(err)
	}
	storagePayload := storageSummaryPayloadFromProduction(productionPayload)
	buildingPayload, ok := buildingPayloadFromProductionSummary(productionPayload, result.Building.BuildingID)
	if !ok {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Building mutation response is unavailable.")
	}

	runtime.mu.Lock()
	wallet := runtime.walletSnapshotLocked(playerID)
	runtime.updatePlayerWalletCacheLocked(playerID, wallet)
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventProductionSummary, productionPayload)
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventPlanetStorage, storagePayload)
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventWalletSnapshot, wallet)
	runtime.mu.Unlock()

	return marshalPayload(map[string]any{
		"building":       buildingPayload,
		"production":     productionPayload,
		"planet_storage": storagePayload,
		"wallet":         wallet,
		"duplicate":      result.Duplicate,
	})
}

type runtimePlanetBuildingCostProvider struct {
	playerID foundation.PlayerID
	rules    gamecontent.ProductionRulesContent
}

func (provider runtimePlanetBuildingCostProvider) BuildingMutationCost(input production.BuildingMutationCostInput) (production.BuildingMutationCost, error) {
	if err := provider.playerID.Validate(); err != nil {
		return production.BuildingMutationCost{}, err
	}
	if err := input.PlanetID.Validate(); err != nil {
		return production.BuildingMutationCost{}, err
	}
	if err := input.BuildingID.Validate(); err != nil {
		return production.BuildingMutationCost{}, err
	}
	if err := input.Definition.Validate(); err != nil {
		return production.BuildingMutationCost{}, err
	}

	switch input.Operation {
	case production.BuildingMutationBuild:
		return provider.mutationCost(input)
	case production.BuildingMutationUpgrade:
		return provider.mutationCost(input)
	default:
		return production.BuildingMutationCost{}, production.ErrInvalidBuildingMutationCost
	}
}

func (provider runtimePlanetBuildingCostProvider) mutationCost(input production.BuildingMutationCostInput) (production.BuildingMutationCost, error) {
	cost, ok := provider.rules.BuildingMutationCost(provider.playerID, input)
	if !ok {
		return production.BuildingMutationCost{}, production.ErrInvalidBuildingMutationCost
	}
	return cost, nil
}

func validatePlanetBuildingSlot(value string) (string, error) {
	slot := strings.TrimSpace(value)
	if slot == "" || slot != value || strings.Contains(slot, ":") || strings.ContainsAny(slot, " \t\r\n") {
		return "", production.ErrInvalidBuildingID
	}
	for _, r := range slot {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return "", production.ErrInvalidBuildingID
	}
	return slot, nil
}

func deterministicPlanetBuildingID(planetID foundation.PlanetID, buildingType production.BuildingType, slot string) (production.BuildingID, error) {
	buildingID := production.BuildingID(fmt.Sprintf("%s-building-%s-%s", planetID, buildingType, slot))
	if err := buildingID.Validate(); err != nil {
		return "", err
	}
	if _, err := foundation.PlanetBuildingBuildIdempotencyKey(planetID, buildingID.String()); err != nil {
		return "", err
	}
	return buildingID, nil
}

func buildingUpgradeTargetLevel(intent planetBuildingUpgradeIntent) (int, error) {
	if intent.TargetLevel > 0 && intent.NextLevel > 0 && intent.TargetLevel != intent.NextLevel {
		return 0, production.ErrInvalidBuildingLevel
	}
	targetLevel := intent.TargetLevel
	if targetLevel == 0 {
		targetLevel = intent.NextLevel
	}
	if targetLevel <= 0 {
		return 0, production.ErrInvalidBuildingLevel
	}
	return targetLevel, nil
}

func buildingPayloadFromProductionSummary(payload planetProductionCollectionPayload, buildingID production.BuildingID) (planetBuildingPayload, bool) {
	for _, planet := range payload.Planets {
		for _, building := range planet.Buildings {
			if building.BuildingID == buildingID.String() {
				return building, true
			}
		}
	}
	return planetBuildingPayload{}, false
}

func domainErrorForBuildingMutation(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, economy.ErrInsufficientWalletFunds):
		return foundation.NewDomainError(foundation.CodeNotEnoughFunds, "Not enough credits.", foundation.WithCause(err))
	case errors.Is(err, production.ErrInsufficientBuildingMaterials):
		return foundation.NewDomainError(foundation.CodeForbidden, "Required planet materials are unavailable.", foundation.WithCause(err))
	case errors.Is(err, production.ErrBuildingNotFound),
		errors.Is(err, production.ErrProductionSnapshotIncomplete):
		return foundation.NewDomainError(foundation.CodeNotFound, "Planet building was not found.", foundation.WithCause(err))
	case errors.Is(err, production.ErrDuplicateBuilding),
		errors.Is(err, production.ErrStaleBuildingMutation):
		return foundation.NewDomainError(foundation.CodeForbidden, "Planet building mutation is not available.", foundation.WithCause(err))
	case errors.Is(err, production.ErrInvalidBuildingID),
		errors.Is(err, production.ErrInvalidBuildingType),
		errors.Is(err, production.ErrInvalidBuildingLevel),
		errors.Is(err, production.ErrInvalidBuildingState),
		errors.Is(err, production.ErrInvalidBuildingMutationCost),
		errors.Is(err, production.ErrInvalidBuildingMutationReference),
		errors.Is(err, production.ErrUnknownBuildingDefinition),
		errors.Is(err, production.ErrBuildingSourceMismatch),
		errors.Is(err, foundation.ErrEmptyID),
		errors.Is(err, foundation.ErrInvalidID),
		errors.Is(err, foundation.ErrEmptyIdempotencyKey),
		errors.Is(err, foundation.ErrInvalidIdempotencyKey),
		errors.Is(err, foundation.ErrEmptyIdempotencyPart),
		errors.Is(err, foundation.ErrInvalidIdempotencyPart):
		return invalidPayload("Planet building payload is invalid.", err)
	case errors.Is(err, production.ErrInvalidBuildingMutationConfig),
		errors.Is(err, production.ErrInvalidBuildingMutationDurableCommit):
		return foundation.NewDomainError(foundation.CodeInternal, "Planet building service is unavailable.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Planet building mutation failed.", foundation.WithCause(err))
	}
}

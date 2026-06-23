package server

import (
	"encoding/json"
	"errors"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

type craftingStartIntent struct {
	RecipeID     string `json:"recipe_id"`
	LocationType string `json:"location_type,omitempty"`
	LocationID   string `json:"location_id,omitempty"`
}

type craftingCompleteIntent struct {
	JobID string `json:"job_id"`
}

func (runtime *Runtime) handleCraftingStart(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"job_id",
		"reference_id",
		"reservation_id",
		"reservation",
		"wallet_debit",
		"source_location",
		"output_location",
		"started_at",
		"completes_at",
		"completed_at",
		"duration",
		"inputs",
		"output",
		"materials",
		"cost",
		"credits",
		"quantity",
		"duplicate",
	); err != nil {
		return nil, err
	}
	var intent craftingStartIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	input, err := runtime.craftingStartInput(ctx.PlayerID, request.RequestID, intent)
	if err != nil {
		return nil, err
	}
	result, err := runtime.Crafting.StartCraft(input)
	if err != nil {
		return nil, domainErrorForCrafting(err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return marshalPayload(map[string]any{
		"crafting":  runtime.craftingSnapshot(ctx.PlayerID),
		"job":       craftingJob(result.Job),
		"wallet":    runtime.walletSnapshotLocked(ctx.PlayerID),
		"inventory": runtime.inventorySnapshotLocked(ctx.PlayerID),
		"duplicate": result.Duplicate,
	})
}

func (runtime *Runtime) handleCraftingComplete(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if runtime == nil || runtime.Crafting == nil {
		return nil, domainErrorForRuntime(crafting.ErrMissingRecipeCatalog)
	}
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"player_id",
		"recipe_id",
		"reservation_id",
		"reservation",
		"reservation_commit",
		"item_output",
		"ship_unlock",
		"xp_grant",
		"reference_id",
		"output_location",
		"started_at",
		"completes_at",
		"completed_at",
		"state",
		"output",
		"quantity",
		"duplicate",
	); err != nil {
		return nil, err
	}
	var intent craftingCompleteIntent
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, err
	}
	jobID := crafting.CraftJobID(intent.JobID)
	if err := jobID.Validate(); err != nil {
		return nil, invalidPayload("Craft job is invalid.", err)
	}
	result, err := runtime.Crafting.CompleteCraft(crafting.CompleteCraftInput{
		PlayerID: ctx.PlayerID,
		JobID:    jobID,
	})
	if err != nil {
		return nil, domainErrorForCrafting(err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return marshalPayload(map[string]any{
		"crafting":    runtime.craftingSnapshot(ctx.PlayerID),
		"job":         craftingJob(result.Job),
		"inventory":   runtime.inventorySnapshotLocked(ctx.PlayerID),
		"progression": progressionPayload(result.XPGrant.Snapshot),
		"duplicate":   result.Duplicate,
	})
}

func (runtime *Runtime) craftingStartInput(playerID foundation.PlayerID, requestID foundation.RequestID, intent craftingStartIntent) (crafting.StartCraftInput, error) {
	if runtime == nil || runtime.Crafting == nil {
		return crafting.StartCraftInput{}, domainErrorForRuntime(crafting.ErrMissingRecipeCatalog)
	}
	if err := playerID.Validate(); err != nil {
		return crafting.StartCraftInput{}, domainErrorForRuntime(err)
	}
	if err := requestID.Validate(); err != nil {
		return crafting.StartCraftInput{}, invalidPayload("Craft request is invalid.", err)
	}
	recipeID, err := catalog.ParseDefinitionID(intent.RecipeID)
	if err != nil {
		return crafting.StartCraftInput{}, invalidPayload("Craft recipe is invalid.", err)
	}
	location, err := craftLocationFromIntent(intent)
	if err != nil {
		return crafting.StartCraftInput{}, err
	}
	referenceKey, err := foundation.CraftStartIdempotencyKey(playerID.String() + "-" + requestID.String())
	if err != nil {
		return crafting.StartCraftInput{}, invalidPayload("Craft request is invalid.", err)
	}
	return crafting.StartCraftInput{
		PlayerID:     playerID,
		RecipeID:     recipeID,
		Location:     location,
		ReferenceKey: referenceKey,
	}, nil
}

func craftLocationFromIntent(intent craftingStartIntent) (crafting.CraftLocation, error) {
	locationType := crafting.CraftLocationType(intent.LocationType)
	if locationType == "" {
		locationType = crafting.CraftLocationStation
	}
	locationID := intent.LocationID
	if locationID == "" && locationType == crafting.CraftLocationStation {
		locationID = "station"
	}
	location := crafting.CraftLocation{
		Type: locationType,
		ID:   locationID,
	}
	if err := location.Validate(); err != nil {
		return crafting.CraftLocation{}, invalidPayload("Craft location is invalid.", err)
	}
	return location, nil
}

func domainErrorForCrafting(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, economy.ErrInsufficientWalletFunds):
		return foundation.NewDomainError(foundation.CodeNotEnoughFunds, "Not enough funds.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrInsufficientItemQuantity), errors.Is(err, economy.ErrItemNotOwned):
		return foundation.NewDomainError(foundation.CodeNotEnoughCargo, "Not enough item quantity.", foundation.WithCause(err))
	case errors.Is(err, crafting.ErrUnknownRecipeDefinition), errors.Is(err, crafting.ErrLocationRequirementNotMet), errors.Is(err, crafting.ErrRankRequirementNotMet), errors.Is(err, crafting.ErrRoleRequirementNotMet), errors.Is(err, crafting.ErrCraftStartReferenceMismatch), errors.Is(err, crafting.ErrMissingLocationAuthorizer):
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Craft request is invalid.", foundation.WithCause(err))
	case errors.Is(err, crafting.ErrCraftNotReady):
		return foundation.NewDomainError(foundation.CodeCooldown, "Craft job is not ready.", foundation.WithCause(err))
	case errors.Is(err, crafting.ErrCraftJobNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Craft job was not found.", foundation.WithCause(err))
	case errors.Is(err, crafting.ErrCraftJobPlayerMismatch):
		return foundation.NewDomainError(foundation.CodeForbidden, "Craft job is not owned by this player.", foundation.WithCause(err))
	case errors.Is(err, crafting.ErrInvalidCraftJobState):
		return foundation.NewDomainError(foundation.CodeForbidden, "Craft job is not active.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Crafting command failed.", foundation.WithCause(err))
	}
}

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/intel"
	"gameproject/internal/game/realtime"
)

type intelShareRequest struct {
	ToPlayerID string `json:"to_player_id"`
	PlanetID   string `json:"planet_id"`
}

type intelCoordinateItemCreateRequest struct {
	PlanetID string `json:"planet_id"`
}

type intelCoordinateItemUseRequest struct {
	ItemInstanceID string `json:"item_instance_id"`
}

type intelSharePayload struct {
	PlanetID        string `json:"planet_id"`
	ToPlayerID      string `json:"to_player_id"`
	Shared          bool   `json:"shared"`
	ReceiverUpdated bool   `json:"receiver_updated"`
	Duplicate       bool   `json:"duplicate"`
}

type intelCoordinateItemPayload struct {
	ItemInstanceID string `json:"item_instance_id"`
	PlanetID       string `json:"planet_id"`
	State          string `json:"state"`
	Confidence     int    `json:"confidence"`
	LastVerifiedAt int64  `json:"last_verified_at"`
	CreatedAt      int64  `json:"created_at"`
	Used           bool   `json:"used"`
	UsedAt         *int64 `json:"used_at,omitempty"`
}

func (runtime *Runtime) handleIntelShare(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"from_player_id",
		"owner_player_id",
		"coordinates",
		"position",
		"x",
		"y",
		"state",
		"intel_state",
		"confidence",
		"last_seen_at",
		"source_type",
		"source_reference",
	); err != nil {
		return nil, err
	}
	var intent intelShareRequest
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, invalidPayload("Intel share intent is invalid.", err)
	}
	toPlayerID := foundation.PlayerID(strings.TrimSpace(intent.ToPlayerID))
	planetID := foundation.PlanetID(strings.TrimSpace(intent.PlanetID))
	if toPlayerID.IsZero() || toPlayerID == ctx.PlayerID {
		return nil, invalidPayload("Intel share target is invalid.", nil)
	}
	if err := toPlayerID.Validate(); err != nil {
		return nil, invalidPayload("Intel share target is invalid.", err)
	}
	if err := planetID.Validate(); err != nil {
		return nil, invalidPayload("Planet id is invalid.", err)
	}
	reference, err := foundation.IntelShareIdempotencyKey(ctx.PlayerID, toPlayerID, planetID, request.RequestID.String())
	if err != nil {
		return nil, invalidPayload("Intel share reference is invalid.", err)
	}

	if _, err := runtime.syncIntelFromDiscovery(ctx.PlayerID, planetID); err != nil {
		return nil, intelDomainError(err)
	}
	result, err := runtime.Intel.SharePlanetIntel(intel.SharePlanetIntelInput{
		FromPlayerID: ctx.PlayerID,
		ToPlayerID:   toPlayerID,
		PlanetID:     planetID,
		Reference:    reference,
	})
	if err != nil {
		return nil, intelDomainError(err)
	}
	if _, _, err := runtime.Discovery.UpsertPlayerPlanetIntel(discoveryIntelFromIntel(result.ReceiverIntel)); err != nil {
		return nil, err
	}
	if result.ReceiverUpdated {
		known, err := runtime.knownPlanetsPayload(toPlayerID)
		if err != nil {
			return nil, err
		}
		runtime.mu.Lock()
		runtime.queueEventToPlayerSessionsLocked(toPlayerID, realtime.EventKnownPlanets, known)
		runtime.mu.Unlock()
	}

	payload := map[string]any{
		"share": intelSharePayload{
			PlanetID:        result.ReceiverIntel.PlanetID.String(),
			ToPlayerID:      toPlayerID.String(),
			Shared:          result.Shared,
			ReceiverUpdated: result.ReceiverUpdated,
			Duplicate:       result.Duplicate,
		},
	}
	return marshalPayload(payload)
}

func (runtime *Runtime) handleIntelCoordinateItemCreate(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"item_instance_id",
		"owner_player_id",
		"created_by",
		"coordinates",
		"position",
		"x",
		"y",
		"state",
		"intel_state",
		"confidence",
		"last_seen_at",
		"source_type",
		"source_reference",
		"last_verified_at",
		"created_at",
		"used_at",
		"used_by",
		"inventory",
	); err != nil {
		return nil, err
	}
	var intent intelCoordinateItemCreateRequest
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, invalidPayload("Coordinate item create intent is invalid.", err)
	}
	planetID := foundation.PlanetID(strings.TrimSpace(intent.PlanetID))
	if err := planetID.Validate(); err != nil {
		return nil, invalidPayload("Planet id is invalid.", err)
	}
	itemID := deterministicCoordinateItemID(ctx.PlayerID, planetID, request.RequestID)
	reference, err := foundation.CoordinateItemCreateIdempotencyKey(ctx.PlayerID, planetID, itemID)
	if err != nil {
		return nil, invalidPayload("Coordinate item create reference is invalid.", err)
	}

	if _, err := runtime.syncIntelFromDiscovery(ctx.PlayerID, planetID); err != nil {
		return nil, intelDomainError(err)
	}
	result, err := runtime.Intel.CreateCoordinateItem(intel.CreateCoordinateItemInput{
		PlayerID:       ctx.PlayerID,
		PlanetID:       planetID,
		ItemInstanceID: itemID,
		Reference:      reference,
	})
	if err != nil {
		return nil, intelDomainError(err)
	}
	payload := map[string]any{
		"coordinate_item": intelCoordinateItemPayloadFromDomain(result.Item),
		"created":         result.Created,
		"duplicate":       result.Duplicate,
	}
	return marshalPayload(payload)
}

func (runtime *Runtime) handleIntelCoordinateItemUse(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayloadWithAdditional(
		request.Payload,
		"planet_id",
		"owner_player_id",
		"created_by",
		"coordinates",
		"position",
		"x",
		"y",
		"state",
		"intel_state",
		"confidence",
		"last_seen_at",
		"source_type",
		"source_reference",
		"last_verified_at",
		"created_at",
		"used_at",
		"used_by",
		"inventory",
	); err != nil {
		return nil, err
	}
	var intent intelCoordinateItemUseRequest
	if err := decodeStrict(request.Payload, &intent); err != nil {
		return nil, invalidPayload("Coordinate item use intent is invalid.", err)
	}
	itemID := foundation.ItemID(strings.TrimSpace(intent.ItemInstanceID))
	if err := itemID.Validate(); err != nil {
		return nil, invalidPayload("Coordinate item id is invalid.", err)
	}
	reference, err := foundation.CoordinateItemUseIdempotencyKey(ctx.PlayerID, itemID, request.RequestID.String())
	if err != nil {
		return nil, invalidPayload("Coordinate item use reference is invalid.", err)
	}

	result, err := runtime.Intel.UseCoordinateItem(intel.UseCoordinateItemInput{
		PlayerID:       ctx.PlayerID,
		ItemInstanceID: itemID,
		Reference:      reference,
	})
	if err != nil {
		return nil, intelDomainError(err)
	}
	if _, _, err := runtime.Discovery.UpsertPlayerPlanetIntel(discoveryIntelFromIntel(result.Intel)); err != nil {
		return nil, err
	}
	known, err := runtime.knownPlanetsPayload(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	detail, err := runtime.planetDetailPayload(ctx.PlayerID, result.Intel.PlanetID)
	if err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventKnownPlanets, known)
	runtime.queueEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventPlanetDetail, detail)
	runtime.mu.Unlock()

	payload := map[string]any{
		"coordinate_item": intelCoordinateItemPayloadFromDomain(result.Item),
		"known_planets":   known,
		"planet_detail":   detail,
		"used":            result.Used,
		"intel_updated":   result.IntelUpdated,
		"duplicate":       result.Duplicate,
	}
	return marshalPayload(payload)
}

func (runtime *Runtime) syncIntelFromDiscovery(playerID foundation.PlayerID, planetID foundation.PlanetID) (intel.PlayerPlanetIntel, error) {
	row, ok, err := runtime.Discovery.PlayerPlanetIntel(playerID, planetID)
	if err != nil {
		return intel.PlayerPlanetIntel{}, err
	}
	if !ok {
		return intel.PlayerPlanetIntel{}, intel.ErrPlanetIntelNotKnown
	}
	converted := intelFromDiscovery(row)
	if _, _, err := runtime.Intel.UpsertPlayerPlanetIntel(converted); err != nil {
		return intel.PlayerPlanetIntel{}, err
	}
	return converted, nil
}

func deterministicCoordinateItemID(playerID foundation.PlayerID, planetID foundation.PlanetID, requestID foundation.RequestID) foundation.ItemID {
	return foundation.ItemID(fmt.Sprintf("coord_%s_%s_%s", playerID, planetID, requestID))
}

func intelCoordinateItemPayloadFromDomain(item intel.CoordinateItem) intelCoordinateItemPayload {
	payload := intelCoordinateItemPayload{
		ItemInstanceID: item.ItemInstanceID.String(),
		PlanetID:       item.PlanetID.String(),
		State:          string(item.State),
		Confidence:     item.Confidence,
		LastVerifiedAt: item.LastVerifiedAt.UTC().UnixMilli(),
		CreatedAt:      item.CreatedAt.UTC().UnixMilli(),
		Used:           item.UsedAt != nil,
	}
	if item.UsedAt != nil {
		usedAt := item.UsedAt.UTC().UnixMilli()
		payload.UsedAt = &usedAt
	}
	return payload
}

func intelFromDiscovery(row discovery.PlayerPlanetIntel) intel.PlayerPlanetIntel {
	return intel.PlayerPlanetIntel{
		PlayerID:        row.PlayerID,
		PlanetID:        row.PlanetID,
		WorldID:         row.WorldID,
		ZoneID:          row.ZoneID,
		Coordinates:     row.Coordinates,
		State:           intel.IntelState(row.State),
		Confidence:      row.Confidence,
		LastSeenAt:      row.LastSeenAt,
		SourceType:      intelSourceFromDiscovery(row.SourceType),
		SourceReference: row.SourceReference,
	}
}

func discoveryIntelFromIntel(row intel.PlayerPlanetIntel) discovery.PlayerPlanetIntel {
	return discovery.PlayerPlanetIntel{
		PlayerID:        row.PlayerID,
		PlanetID:        row.PlanetID,
		WorldID:         row.WorldID,
		ZoneID:          row.ZoneID,
		Coordinates:     row.Coordinates,
		State:           discovery.IntelState(row.State),
		Confidence:      row.Confidence,
		LastSeenAt:      row.LastSeenAt,
		SourceType:      discoverySourceFromIntel(row.SourceType),
		SourceReference: row.SourceReference,
	}
}

func intelSourceFromDiscovery(source discovery.IntelSourceType) intel.IntelSourceType {
	switch source {
	case discovery.IntelSourceShareReceived:
		return intel.IntelSourceShareReceived
	case discovery.IntelSourceCoordinateScrollUsed:
		return intel.IntelSourceCoordinateItemUsed
	case discovery.IntelSourceQuestReward:
		return intel.IntelSourceQuestReward
	case discovery.IntelSourceMarketPurchase:
		return intel.IntelSourceMarketPurchase
	case discovery.IntelSourceAdmin, discovery.IntelSourcePlanetOwnerChanged:
		return intel.IntelSourceAdmin
	default:
		return intel.IntelSourceScanSuccess
	}
}

func discoverySourceFromIntel(source intel.IntelSourceType) discovery.IntelSourceType {
	switch source {
	case intel.IntelSourceShareReceived:
		return discovery.IntelSourceShareReceived
	case intel.IntelSourceCoordinateItemUsed:
		return discovery.IntelSourceCoordinateScrollUsed
	case intel.IntelSourceQuestReward:
		return discovery.IntelSourceQuestReward
	case intel.IntelSourceMarketPurchase:
		return discovery.IntelSourceMarketPurchase
	case intel.IntelSourceAdmin:
		return discovery.IntelSourceAdmin
	default:
		return discovery.IntelSourceScanSuccess
	}
}

func intelDomainError(err error) error {
	switch {
	case errors.Is(err, intel.ErrPlanetIntelNotKnown), errors.Is(err, intel.ErrPlanetIntelInvalidated), errors.Is(err, intel.ErrCoordinateItemNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Intel was not found.")
	case errors.Is(err, intel.ErrCoordinateItemNotOwned), errors.Is(err, intel.ErrCoordinateItemAlreadyUsed):
		return foundation.NewDomainError(foundation.CodeForbidden, "Coordinate item cannot be used.")
	case errors.Is(err, intel.ErrReferenceConflict),
		errors.Is(err, intel.ErrInvalidReference),
		errors.Is(err, intel.ErrInvalidIntel),
		errors.Is(err, intel.ErrInvalidCoordinateItem),
		errors.Is(err, intel.ErrInvalidIntelState),
		errors.Is(err, intel.ErrInvalidIntelSource),
		errors.Is(err, intel.ErrInvalidIntelConfidence):
		return invalidPayload("Intel intent is invalid.", err)
	default:
		return err
	}
}

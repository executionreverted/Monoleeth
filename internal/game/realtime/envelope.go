package realtime

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"gameproject/internal/game/contracts"
	"gameproject/internal/game/foundation"
)

// CurrentVersion is the JSON realtime protocol version.
const CurrentVersion = contracts.CurrentVersion

// Operation is a client request operation name.
type Operation string

const (
	OperationSessionSnapshot      Operation = "session.snapshot"
	OperationWorldSnapshot        Operation = "world.snapshot"
	OperationMoveTo               Operation = "move_to"
	OperationStop                 Operation = "stop"
	OperationDebugSpawnNPC        Operation = "debug_spawn_npc"
	OperationDebugSnapshot        Operation = "debug_snapshot"
	OperationCombatUseSkill       Operation = "combat.use_skill"
	OperationLootPickup           Operation = "loot.pickup"
	OperationDeathRepairQuote     Operation = "death.repair_quote"
	OperationDeathRepairShip      Operation = "death.repair_ship"
	OperationProgressionSnapshot  Operation = "progression.snapshot"
	OperationInventorySnapshot    Operation = "inventory.snapshot"
	OperationHangarSnapshot       Operation = "hangar.snapshot"
	OperationHangarActivateShip   Operation = "hangar.activate_ship"
	OperationLoadoutSnapshot      Operation = "loadout.snapshot"
	OperationLoadoutEquipModule   Operation = "loadout.equip_module"
	OperationLoadoutUnequipModule Operation = "loadout.unequip_module"
	OperationStatsSnapshot        Operation = "stats.snapshot"
	OperationStealthToggle        Operation = "stealth.toggle"
	OperationCraftingRecipes      Operation = "crafting.recipes"
	OperationScanPulse            Operation = "scan.pulse"
	OperationKnownPlanets         Operation = "discovery.known_planets"
	OperationPlanetDetail         Operation = "discovery.planet_detail"
	OperationProductionSummary    Operation = "planet.production_summary"
	OperationPlanetStorage        Operation = "planet.storage_summary"
	OperationRouteList            Operation = "route.list"
	OperationRouteSnapshot        Operation = "route.snapshot"
	OperationWalletSnapshot       Operation = "wallet.snapshot"
	OperationShopCatalog          Operation = "shop.catalog"
	OperationMarketSearch         Operation = "market.search"
	OperationMarketCreateListing  Operation = "market.create_listing"
	OperationMarketBuy            Operation = "market.buy"
	OperationMarketCancel         Operation = "market.cancel"
	OperationAuctionSearch        Operation = "auction.search"
	OperationAuctionBid           Operation = "auction.bid"
	OperationAuctionBuyNow        Operation = "auction.buy_now"
	OperationAuctionGrants        Operation = "auction.grants"
	OperationPremiumEntitlements  Operation = "premium.entitlements"
	OperationPremiumClaim         Operation = "premium.claim"
	OperationPremiumWeeklyXCore   Operation = "premium.purchase_weekly_xcore"
	OperationQuestBoard           Operation = "quest.board"
	OperationQuestAccept          Operation = "quest.accept"
	OperationQuestProgress        Operation = "quest.progress"
	OperationQuestClaimReward     Operation = "quest.claim_reward"
	OperationQuestReroll          Operation = "quest.reroll"
	OperationAdminInspectPlayer   Operation = "admin.inspect_player"
	OperationAdminRepairCraftJob  Operation = "admin.repair_craft_job"
	OperationAdminEconomyDash     Operation = "admin.economy_dashboard"
	OperationObservabilityLog     Operation = "observability.command_log"
	OperationObservabilityMetric  Operation = "observability.metrics"
	OperationObservabilityGate    Operation = "observability.release_gate"
	OperationObservabilityAbuse   Operation = "observability.abuse_coverage"
)

// ClientEventType is an event name that may be sent to a client after filtering.
type ClientEventType string

const (
	EventSessionReady          ClientEventType = "session.ready"
	EventPlayerSnapshot        ClientEventType = "player.snapshot"
	EventShipSnapshot          ClientEventType = "ship.snapshot"
	EventStatsUpdated          ClientEventType = "stats.updated"
	EventWalletSnapshot        ClientEventType = "wallet.snapshot"
	EventCargoSnapshot         ClientEventType = "cargo.snapshot"
	EventWorldSnapshot         ClientEventType = "world.snapshot"
	EventAOIEntityEntered      ClientEventType = "aoi.entity_entered"
	EventAOIEntityUpdated      ClientEventType = "aoi.entity_updated"
	EventAOIEntityLeft         ClientEventType = "aoi.entity_left"
	EventPositionCorrected     ClientEventType = "position.corrected"
	EventMovementStopped       ClientEventType = "movement.stopped"
	EventServerNotice          ClientEventType = "server.notice"
	EventTargetUpdated         ClientEventType = "target.updated"
	EventCombatDamage          ClientEventType = "combat.damage"
	EventCombatMiss            ClientEventType = "combat.miss"
	EventCombatCooldownStarted ClientEventType = "combat.cooldown_started"
	EventCombatNPCKilled       ClientEventType = "combat.npc_killed"
	EventLootCreated           ClientEventType = "loot.created"
	EventLootUpdated           ClientEventType = "loot.updated"
	EventLootRemoved           ClientEventType = "loot.removed"
	EventLootPickedUp          ClientEventType = "loot.picked_up"
	EventProgressionSnapshot   ClientEventType = "progression.snapshot"
	EventInventorySnapshot     ClientEventType = "inventory.snapshot"
	EventHangarSnapshot        ClientEventType = "hangar.snapshot"
	EventLoadoutSnapshot       ClientEventType = "loadout.snapshot"
	EventCraftingRecipes       ClientEventType = "crafting.recipes"
	EventScanPulseStarted      ClientEventType = "scan.pulse_started"
	EventScanPulseResolved     ClientEventType = "scan.pulse_resolved"
	EventScanPlanetDiscovered  ClientEventType = "scan.planet_discovered"
	EventKnownPlanets          ClientEventType = "discovery.known_planets"
	EventPlanetDetail          ClientEventType = "discovery.planet_detail"
	EventProductionSummary     ClientEventType = "planet.production_summary"
	EventPlanetStorage         ClientEventType = "planet.storage_summary"
	EventRouteList             ClientEventType = "route.list"
	EventRouteSnapshot         ClientEventType = "route.snapshot"
	EventMarketListingCreated  ClientEventType = "market.listing_created"
	EventMarketListingUpdated  ClientEventType = "market.listing_updated"
	EventMarketSaleCompleted   ClientEventType = "market.sale_completed"
	EventMarketListingCanceled ClientEventType = "market.listing_cancelled"
	EventAuctionLotUpdated     ClientEventType = "auction.lot_updated"
	EventAuctionBidPlaced      ClientEventType = "auction.bid_placed"
	EventAuctionClosed         ClientEventType = "auction.closed"
	EventPremiumEntitlement    ClientEventType = "premium.entitlement_created"
	EventPremiumClaimed        ClientEventType = "premium.entitlement_claimed"
	EventPremiumStockConsumed  ClientEventType = "premium.stock_consumed"
	EventQuestBoardGenerated   ClientEventType = "quest.board_generated"
	EventQuestAccepted         ClientEventType = "quest.accepted"
	EventQuestProgressed       ClientEventType = "quest.progressed"
	EventQuestCompleted        ClientEventType = "quest.completed"
	EventQuestRewardClaimed    ClientEventType = "quest.reward_claimed"
	EventQuestBoardRerolled    ClientEventType = "quest.board_rerolled"
	EventQuestAbandoned        ClientEventType = "quest.abandoned"
	EventAdminActionCompleted  ClientEventType = "admin.action_completed"
	EventObservabilityMetric   ClientEventType = "observability.metric_updated"
	EventReleaseGateUpdated    ClientEventType = "release_gate.updated"
	EventEconomyFlowUpdated    ClientEventType = "economy.flow_updated"
	EventDeathShipDisabled     ClientEventType = "death.ship_disabled"
	EventDeathRepaired         ClientEventType = "death.repaired"
)

// RateLimitPosture names the future per-operation abuse posture.
//
// This is intentionally metadata only. It does not enforce limits or affect
// gameplay truth.
type RateLimitPosture string

const (
	RateLimitPostureUnspecified RateLimitPosture = "unspecified"
	RateLimitPostureIntentBurst RateLimitPosture = "intent_burst"
	RateLimitPostureDebugOnly   RateLimitPosture = "debug_only"
)

// OperationSpec describes one registered realtime operation.
type OperationSpec struct {
	Operation        Operation
	RateLimitPosture RateLimitPosture
}

var registeredOperations = map[Operation]OperationSpec{
	OperationSessionSnapshot: {
		Operation:        OperationSessionSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationWorldSnapshot: {
		Operation:        OperationWorldSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationMoveTo: {
		Operation:        OperationMoveTo,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationStop: {
		Operation:        OperationStop,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationDebugSpawnNPC: {
		Operation:        OperationDebugSpawnNPC,
		RateLimitPosture: RateLimitPostureDebugOnly,
	},
	OperationDebugSnapshot: {
		Operation:        OperationDebugSnapshot,
		RateLimitPosture: RateLimitPostureDebugOnly,
	},
	OperationCombatUseSkill: {
		Operation:        OperationCombatUseSkill,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationLootPickup: {
		Operation:        OperationLootPickup,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationDeathRepairQuote: {
		Operation:        OperationDeathRepairQuote,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationDeathRepairShip: {
		Operation:        OperationDeathRepairShip,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationProgressionSnapshot: {
		Operation:        OperationProgressionSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationInventorySnapshot: {
		Operation:        OperationInventorySnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationHangarSnapshot: {
		Operation:        OperationHangarSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationHangarActivateShip: {
		Operation:        OperationHangarActivateShip,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationLoadoutSnapshot: {
		Operation:        OperationLoadoutSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationLoadoutEquipModule: {
		Operation:        OperationLoadoutEquipModule,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationLoadoutUnequipModule: {
		Operation:        OperationLoadoutUnequipModule,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationStatsSnapshot: {
		Operation:        OperationStatsSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationStealthToggle: {
		Operation:        OperationStealthToggle,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationCraftingRecipes: {
		Operation:        OperationCraftingRecipes,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationScanPulse: {
		Operation:        OperationScanPulse,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationKnownPlanets: {
		Operation:        OperationKnownPlanets,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationPlanetDetail: {
		Operation:        OperationPlanetDetail,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationProductionSummary: {
		Operation:        OperationProductionSummary,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationPlanetStorage: {
		Operation:        OperationPlanetStorage,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationRouteList: {
		Operation:        OperationRouteList,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationRouteSnapshot: {
		Operation:        OperationRouteSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationWalletSnapshot: {
		Operation:        OperationWalletSnapshot,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationShopCatalog: {
		Operation:        OperationShopCatalog,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationMarketSearch: {
		Operation:        OperationMarketSearch,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationMarketCreateListing: {
		Operation:        OperationMarketCreateListing,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationMarketBuy: {
		Operation:        OperationMarketBuy,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationMarketCancel: {
		Operation:        OperationMarketCancel,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAuctionSearch: {
		Operation:        OperationAuctionSearch,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAuctionBid: {
		Operation:        OperationAuctionBid,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAuctionBuyNow: {
		Operation:        OperationAuctionBuyNow,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAuctionGrants: {
		Operation:        OperationAuctionGrants,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationPremiumEntitlements: {
		Operation:        OperationPremiumEntitlements,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationPremiumClaim: {
		Operation:        OperationPremiumClaim,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationPremiumWeeklyXCore: {
		Operation:        OperationPremiumWeeklyXCore,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationQuestBoard: {
		Operation:        OperationQuestBoard,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationQuestAccept: {
		Operation:        OperationQuestAccept,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationQuestProgress: {
		Operation:        OperationQuestProgress,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationQuestClaimReward: {
		Operation:        OperationQuestClaimReward,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationQuestReroll: {
		Operation:        OperationQuestReroll,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAdminInspectPlayer: {
		Operation:        OperationAdminInspectPlayer,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAdminRepairCraftJob: {
		Operation:        OperationAdminRepairCraftJob,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationAdminEconomyDash: {
		Operation:        OperationAdminEconomyDash,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationObservabilityLog: {
		Operation:        OperationObservabilityLog,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationObservabilityMetric: {
		Operation:        OperationObservabilityMetric,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationObservabilityGate: {
		Operation:        OperationObservabilityGate,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
	OperationObservabilityAbuse: {
		Operation:        OperationObservabilityAbuse,
		RateLimitPosture: RateLimitPostureIntentBurst,
	},
}

// LookupOperation returns the registered realtime operation spec for op.
func LookupOperation(op Operation) (OperationSpec, bool) {
	spec, ok := registeredOperations[op]
	return spec, ok
}

// OperationRegistry returns a copy of the realtime operation registry.
func OperationRegistry() map[Operation]OperationSpec {
	registry := make(map[Operation]OperationSpec, len(registeredOperations))
	for op, spec := range registeredOperations {
		registry[op] = spec
	}
	return registry
}

// RequestEnvelope is the client-to-server command/query envelope.
type RequestEnvelope struct {
	RequestID foundation.RequestID `json:"request_id"`
	Op        Operation            `json:"op"`
	Payload   json.RawMessage      `json:"payload"`
	ClientSeq uint64               `json:"client_seq"`
	Version   int                  `json:"v"`
}

// ResponseEnvelope is the successful server response envelope for a request.
type ResponseEnvelope struct {
	RequestID  foundation.RequestID `json:"request_id"`
	OK         bool                 `json:"ok"`
	Payload    json.RawMessage      `json:"payload"`
	ServerTime int64                `json:"server_time"`
	Version    int                  `json:"v"`
}

// ErrorPayload is the client-safe error body carried by an ErrorEnvelope.
type ErrorPayload struct {
	foundation.PublicError
	Retryable bool `json:"retryable"`
}

// ErrorEnvelope is the failed server response envelope for a request.
type ErrorEnvelope struct {
	RequestID  foundation.RequestID `json:"request_id"`
	OK         bool                 `json:"ok"`
	Error      ErrorPayload         `json:"error"`
	ServerTime int64                `json:"server_time"`
	Version    int                  `json:"v"`
}

// EventEnvelope is the server-to-client realtime event envelope.
type EventEnvelope struct {
	EventID    foundation.EventID `json:"event_id"`
	Type       ClientEventType    `json:"type"`
	Payload    json.RawMessage    `json:"payload"`
	ServerTime int64              `json:"server_time"`
	Sequence   uint64             `json:"seq"`
	Version    int                `json:"v"`
}

// NewRequestEnvelope returns a request envelope using the current protocol version.
func NewRequestEnvelope(requestID foundation.RequestID, op Operation, payload json.RawMessage, clientSeq uint64) RequestEnvelope {
	return RequestEnvelope{
		RequestID: requestID,
		Op:        op,
		Payload:   cloneRawMessage(payload),
		ClientSeq: clientSeq,
		Version:   CurrentVersion,
	}
}

// DecodeRequestEnvelope decodes and validates a request envelope.
func DecodeRequestEnvelope(data []byte) (RequestEnvelope, error) {
	var envelope RequestEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return RequestEnvelope{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid request envelope.",
			foundation.WithCause(err),
		)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return RequestEnvelope{}, foundation.NewDomainError(
			foundation.CodeInvalidPayload,
			"Invalid request envelope.",
		)
	}
	if err := envelope.Validate(); err != nil {
		return RequestEnvelope{}, err
	}
	return envelope, nil
}

// Validate checks envelope-level fields common to every realtime request.
func (envelope RequestEnvelope) Validate() error {
	if err := envelope.RequestID.Validate(); err != nil {
		return invalidRequest("request_id is required.", err)
	}
	if envelope.Version != 0 && envelope.Version != CurrentVersion {
		return invalidRequest("protocol version is not supported.", nil)
	}
	if strings.TrimSpace(string(envelope.Op)) == "" {
		return invalidRequest("op is required.", nil)
	}
	if _, ok := LookupOperation(envelope.Op); !ok {
		return invalidRequest("op is not registered.", nil)
	}
	if err := validateRequestPayload(envelope.Payload); err != nil {
		return err
	}
	return nil
}

// NewResponseEnvelope returns a successful response envelope.
func NewResponseEnvelope(requestID foundation.RequestID, payload json.RawMessage, serverTime int64) ResponseEnvelope {
	return ResponseEnvelope{
		RequestID:  requestID,
		OK:         true,
		Payload:    cloneRawMessage(payload),
		ServerTime: serverTime,
		Version:    CurrentVersion,
	}
}

// NewErrorEnvelope returns a failed response envelope from a domain error.
func NewErrorEnvelope(requestID foundation.RequestID, domainErr *foundation.DomainError, retryable bool, serverTime int64) ErrorEnvelope {
	publicErr := foundation.PublicError{
		Code:    foundation.CodeInternal,
		Message: "Request failed.",
	}
	if domainErr != nil {
		publicErr = domainErr.Public()
	}

	return ErrorEnvelope{
		RequestID: requestID,
		OK:        false,
		Error: ErrorPayload{
			PublicError: publicErr,
			Retryable:   retryable,
		},
		ServerTime: serverTime,
		Version:    CurrentVersion,
	}
}

// NewEventEnvelope returns a filtered server-to-client realtime event envelope.
func NewEventEnvelope(eventID foundation.EventID, eventType ClientEventType, payload json.RawMessage, serverTime int64, sequence uint64) EventEnvelope {
	return EventEnvelope{
		EventID:    eventID,
		Type:       eventType,
		Payload:    cloneRawMessage(payload),
		ServerTime: serverTime,
		Sequence:   sequence,
		Version:    CurrentVersion,
	}
}

func validateRequestPayload(payload json.RawMessage) error {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return invalidRequest("payload is required.", nil)
	}
	if !json.Valid(trimmed) {
		return invalidRequest("payload must be valid JSON.", nil)
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return invalidRequest("payload is required.", nil)
	}
	if trimmed[0] != '{' {
		return invalidRequest("payload must be a JSON object.", nil)
	}
	return nil
}

func invalidRequest(message string, cause error) *foundation.DomainError {
	opts := make([]foundation.DomainErrorOption, 0, 1)
	if cause != nil {
		opts = append(opts, foundation.WithCause(cause))
	}
	return foundation.NewDomainError(foundation.CodeInvalidPayload, message, opts...)
}

func cloneRawMessage(payload json.RawMessage) json.RawMessage {
	if payload == nil {
		return nil
	}
	return append(json.RawMessage(nil), payload...)
}

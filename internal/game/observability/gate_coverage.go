package observability

// GateStatus names whether one release/security gate is satisfied, missing, or
// intentionally not applicable for the covered module or command.
type GateStatus string

const (
	GateStatusSatisfied     GateStatus = "satisfied"
	GateStatusMissing       GateStatus = "missing"
	GateStatusNotApplicable GateStatus = "not_applicable"
)

// GateEvidence points at durable proof for a satisfied gate. Command evidence
// records a required operator validation command rather than a historical result.
type GateEvidence struct {
	Package  string `json:"package,omitempty"`
	TestName string `json:"test_name,omitempty"`
	Document string `json:"document,omitempty"`
	Command  string `json:"command,omitempty"`
	Note     string `json:"note"`
}

// ReleaseGateCoverage records one Phase 12 release-gate item for one module.
type ReleaseGateCoverage struct {
	Module   string           `json:"module"`
	Check    ReleaseGateCheck `json:"check"`
	Status   GateStatus       `json:"status"`
	Evidence []GateEvidence   `json:"evidence,omitempty"`
	Note     string           `json:"note,omitempty"`
}

// ReleaseGateCoverageMissing names an uncovered or failing module release gate.
type ReleaseGateCoverageMissing struct {
	Module string           `json:"module"`
	Check  ReleaseGateCheck `json:"check"`
}

// ReleaseGateCoverageReport reports whether every required module/check pair is covered.
type ReleaseGateCoverageReport struct {
	Covered bool                         `json:"covered"`
	Passed  bool                         `json:"passed"`
	Missing []ReleaseGateCoverageMissing `json:"missing,omitempty"`
}

// CommandSecurityCoverage records one security-review item for one command.
type CommandSecurityCoverage struct {
	Command  string               `json:"command"`
	Check    CommandSecurityCheck `json:"check"`
	Status   GateStatus           `json:"status"`
	Evidence []GateEvidence       `json:"evidence,omitempty"`
	Note     string               `json:"note,omitempty"`
}

// CommandSecurityCoverageMissing names an uncovered or failing command gate.
type CommandSecurityCoverageMissing struct {
	Command string               `json:"command"`
	Check   CommandSecurityCheck `json:"check"`
}

// CommandSecurityCoverageReport reports whether every command/check pair is covered.
type CommandSecurityCoverageReport struct {
	Covered bool                             `json:"covered"`
	Passed  bool                             `json:"passed"`
	Missing []CommandSecurityCoverageMissing `json:"missing,omitempty"`
}

type gateCoverageSource struct {
	Status   GateStatus
	Evidence []GateEvidence
	Note     string
}

type releaseModuleProfile struct {
	Module string
	Checks map[ReleaseGateCheck]gateCoverageSource
}

type commandSecurityProfile struct {
	Command string
	Checks  map[CommandSecurityCheck]gateCoverageSource
}

var requiredReleaseGateModules = []string{
	"01-player-progression-rank-role-skills",
	"02-inventory-cargo-wallet-ledger",
	"03-ship-hangar-loadout",
	"04-module-stat-aggregation",
	"05-combat-damage-targeting",
	"06-loot-drop-ownership",
	"07-death-repair-respawn",
	"08-crafting-recipes-materials",
	"09-market-auction-premium",
	"10-quest-board-generation",
	"11-planet-production-offline-settlement",
	"12-automation-routes",
	"13-intel-coordinate-trading",
	"14-world-aoi-fog-security",
	"15-api-events-errors",
	"16-testing-observability-balancing",
}

var requiredCommandSecurityOperations = []string{
	"session.snapshot",
	"world.snapshot",
	"move_to",
	"stop",
	"portal.enter",
	"debug_spawn_npc",
	"debug_snapshot",
	"combat.use_skill",
	"loot.pickup",
	"repair.shield_tick",
	"death.repair_quote",
	"death.repair_ship",
	"progression.snapshot",
	"progression.unlock_skill",
	"inventory.snapshot",
	"hangar.snapshot",
	"hangar.activate_ship",
	"loadout.snapshot",
	"loadout.equip_module",
	"loadout.unequip_module",
	"stats.snapshot",
	"stealth.toggle",
	"crafting.recipes",
	"crafting.start",
	"crafting.complete",
	"crafting.cancel",
	"scan.pulse",
	"discovery.claim_planet",
	"discovery.known_planets",
	"discovery.planet_detail",
	"intel.share",
	"intel.coordinate_item.create",
	"intel.coordinate_item.use",
	"planet.production_summary",
	"planet.storage_summary",
	"planet.building_build",
	"planet.building_upgrade",
	"route.create",
	"route.update",
	"route.enable",
	"route.disable",
	"route.settle",
	"route.list",
	"route.snapshot",
	"wallet.snapshot",
	"content.catalog",
	"shop.catalog",
	"shop.buy_product",
	"market.search",
	"market.create_listing",
	"market.buy",
	"market.cancel",
	"auction.search",
	"auction.bid",
	"auction.buy_now",
	"auction.grants",
	"premium.entitlements",
	"premium.claim",
	"premium.purchase_weekly_xcore",
	"quest.board",
	"quest.accept",
	"quest.progress",
	"quest.claim_reward",
	"quest.reroll",
	"admin.inspect_player",
	"admin.repair_craft_job",
	"admin.economy_dashboard",
	"admin.content.list",
	"admin.content.get",
	"admin.content.update_draft",
	"admin.content.validate_draft",
	"admin.content.publish",
	"admin.content.rollback",
	"admin.content.audit_log",
	"admin.content.versions",
	"observability.command_log",
	"observability.metrics",
	"observability.release_gate",
	"observability.abuse_coverage",
}

var phase12LoadTestEvidence = GateEvidence{
	Package:  "gameproject/internal/game/observability",
	TestName: "TestPhase12WorldRealtimeLoadSmokeCoversExpectedThroughput",
	Note:     "local load-smoke coverage executes the Phase 12 minimum player, visibility, snapshot, AOI, and metric envelope",
}

var phase12GoTestAllEvidence = GateEvidence{
	Command: "go test ./...",
	Note:    "required final repository validation command before handoff",
}

var phase12GitDiffCheckEvidence = GateEvidence{
	Command: "git diff --check",
	Note:    "required final whitespace validation command before handoff",
}

var phase12ReleaseModuleProfiles = []releaseModuleProfile{
	releaseModuleProfileFor(
		"01-player-progression-rank-role-skills",
		evidence("gameproject/internal/game/progression", "TestTryRankUpGrantsHistorySkillPointAndInvalidationOnce", "rank progression grants history and skill points idempotently"),
		satisfied(evidence("gameproject/internal/game/progression", "TestGrantXPAppliesMainAndRoleXPOncePerSource", "XP grants are source-idempotent and mutate progression state once")),
		satisfied(evidence("gameproject/internal/game/progression", "TestUnlockPilotSkillValidatesLockedNodesAndConsumesPointOnce", "locked skill unlock abuse is rejected before spending more than one point")),
		notApplicable("progression has no value-changing admin inspection surface beyond shared account support views"),
		notApplicable("progression changes are not item/currency ledger mutations"),
	),
	releaseModuleProfileFor(
		"02-inventory-cargo-wallet-ledger",
		evidence("gameproject/internal/game/economy", "TestAddItemWritesItemLedgerEntryWithReasonAndReference", "inventory service writes item ledgers with reasons and references"),
		satisfied(evidence("gameproject/internal/game/economy", "TestTransferCurrencyMovesFundsOnceAndWritesDebitAndCreditLedgerEntries", "wallet transfer moves value once with debit and credit ledger rows")),
		satisfied(evidence("gameproject/internal/game/economy", "TestPhase02SafetyReserveItemsRejectsNegativeQuantityWithoutLedger", "negative item reservation abuse fails before ledger mutation")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminInspectsPlayerInventoryAndLedgers", "admin inspection exposes inventory, wallet ledger, and item ledger snapshots")),
		satisfied(evidence("gameproject/internal/game/economy", "TestLedgerEntriesRequireReasonAndIdempotencyReference", "ledger entries require reasons and idempotency references")),
	),
	releaseModuleProfileFor(
		"03-ship-hangar-loadout",
		evidence("gameproject/internal/game/ships", "TestEnsureStarterShipCreatesAndActivatesStarterWhenNoActiveShip", "starter ship and hangar state unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/ships", "TestUnlockShipIsIdempotentByPlayerAndShip", "ship unlock is idempotent by player and ship")),
		satisfied(evidence("gameproject/internal/game/modules", "TestSaveLoadoutIgnoresSpoofedRankAndRoleInput", "loadout save ignores spoofed client rank and role inputs")),
		notApplicable("ship inspection is covered through gameplay snapshots and does not expose direct value repair tools yet"),
		notApplicable("ship and loadout state changes do not write item/currency ledgers directly"),
	),
	releaseModuleProfileFor(
		"04-module-stat-aggregation",
		evidence("gameproject/internal/game/stats", "TestAggregateStatsAppliesDocumentedOrder", "stat aggregation unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/stats", "TestStatServiceGetEffectiveStatsAggregatesProviderInputs", "stat service recalculates from server-side providers and snapshots")),
		satisfied(evidence("gameproject/internal/game/stats", "TestStatServiceExcludesBrokenModuleModifiers", "broken modules are excluded from effective stat snapshots")),
		notApplicable("stat aggregation is recomputable from authoritative service state"),
		notApplicable("stat aggregation does not mutate item/currency ledgers"),
	),
	releaseModuleProfileFor(
		"05-combat-damage-targeting",
		evidence("gameproject/internal/game/combat", "TestExecuteBasicAttackSpendsExactEnergyAndStartsCooldown", "combat attack unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/combat", "TestSimultaneousLethalDamageProcessesNPCDeathOnce", "simultaneous lethal damage resolves one NPC death")),
		satisfied(evidence("gameproject/internal/game/combat", "TestExecuteBasicAttackRejectsHiddenTarget", "hidden target attack abuse is rejected")),
		notApplicable("combat does not have value-changing admin repair separate from death/loot/economy services"),
		notApplicable("combat energy/cooldown state is not an item/currency ledger mutation"),
	),
	releaseModuleProfileFor(
		"06-loot-drop-ownership",
		evidence("gameproject/internal/game/loot", "TestCreateDropsForNPCKillRollsServerSideAndIsIdempotent", "loot drops are rolled server-side and source-idempotent"),
		satisfied(evidence("gameproject/internal/game/loot", "TestPickupDropRecordsDuplicateLootXPReconciliationWithoutUndoingClaimOrCargo", "loot pickup coordinates claim, cargo, and XP reconciliation")),
		satisfied(evidence("gameproject/internal/game/loot", "TestPickupDropRejectsFarHiddenAndCargoFullWithoutClaim", "out-of-range and hidden pickup abuse fails before claim/cargo mutation")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminInspectsPlayerInventoryAndLedgers", "admin ledger inspection can audit loot-driven inventory changes")),
		satisfied(evidence("gameproject/internal/game/economy", "TestAddItemWritesItemLedgerEntryWithReasonAndReference", "loot pickup item movement flows through item ledger references")),
	),
	releaseModuleProfileFor(
		"07-death-repair-respawn",
		evidence("gameproject/internal/game/death", "TestDeathServiceProcessDeathDropsCargoCreatesLootDisablesShipRecordsRespawnAndCallsHook", "death service vertical behavior is covered"),
		satisfied(evidence("gameproject/internal/game/death", "TestDeathServiceProcessDeathDuplicateLethalEventDoesNotMutateTwice", "duplicate lethal event does not duplicate death value movement")),
		satisfied(evidence("gameproject/internal/game/death", "TestRepairServiceDuplicateReferenceDoesNotDoubleCharge", "duplicate repair reference does not double-charge")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminWritesCompensatingCurrencyAndItemEntries", "admin repair uses compensating entries for bad value movement")),
		satisfied(evidence("gameproject/internal/game/death", "TestRepairServiceRejectsNonDisabledShipWithoutWalletLedger", "repair write paths avoid wallet ledger mutation on invalid state")),
	),
	releaseModuleProfileFor(
		"08-crafting-recipes-materials",
		evidence("gameproject/internal/game/crafting", "TestMVPRecipeCatalogValidates", "MVP recipe catalog unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/crafting", "TestStartCraftReservesMaterialsAndDebitsFee", "craft start reserves material and debits fee through services")),
		satisfied(evidence("gameproject/internal/game/crafting", "TestCompleteCraftAfterTimeCreatesItemOnceForDuplicateCompletion", "duplicate craft completion creates output once")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminRepairsReadyCraftJobThroughCompletion", "admin can repair a ready craft job through normal completion")),
		satisfied(evidence("gameproject/internal/game/economy", "TestCommitReservationCraftMovesReservedItemsToSystemSinkAndWritesLedger", "craft reservation commit writes item ledger sink rows")),
	),
	releaseModuleProfileFor(
		"09-market-auction-premium",
		evidence("gameproject/internal/game/market", "TestCreateListingDuplicateReferenceReturnsCachedResultWithoutMutation", "market listing unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/auction", "TestPlaceBidDuplicateRetryDoesNotDebitOrRefundTwice", "auction bidding is duplicate-safe and refund-aware")),
		satisfied(evidence("gameproject/internal/game/observability/simulations", "TestMarketBuyCancelRaceSimulationConservesItems", "market buy/cancel race simulation conserves value")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminDisablesSuspiciousListingAndMarksIntelStale", "admin can disable suspicious listings and mark stale intel")),
		satisfied(evidence("gameproject/internal/game/market", "TestBuyListingDuplicateRetryReturnsPreviousResultWithoutDuplication", "market buy writes ledger-backed value movement once")),
	),
	releaseModuleProfileFor(
		"10-quest-board-generation",
		evidence("gameproject/internal/game/quests", "TestGenerateBoardReturnsExactlyTenOffers", "quest board generation unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/quests", "TestClaimRewardGrantsAllRewardsOnceWithQuestRewardReference", "quest reward claim grants through stable reward reference")),
		satisfied(evidence("gameproject/internal/game/quests", "TestClaimRewardDuplicateReturnsClaimedResultWithoutDuplicateGrants", "duplicate quest reward claim does not duplicate grants")),
		notApplicable("quest admin repair is not required beyond reward ledger inspection for current MVP"),
		satisfied(evidence("gameproject/internal/game/quests", "TestClaimRewardGrantsAllRewardsOnceWithQuestRewardReference", "quest rewards use explicit quest reward references for value grants")),
	),
	releaseModuleProfileFor(
		"11-planet-production-offline-settlement",
		evidence("gameproject/internal/game/production", "TestSettlePlanetProductionOneHourOutput", "planet production settlement unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/production", "TestSettlePlanetProductionDoubleSettlementDoesNotDuplicateOutput", "repeated offline settlement does not duplicate output")),
		satisfied(evidence("gameproject/internal/game/observability/simulations", "TestPlanetSettlementSimulationTracksOfflineProductionAndDuplicateNoOps", "planet settlement simulation tracks duplicate no-op behavior")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminDryRunsProductionAndRouteSettlementWithoutMutatingStore", "admin dry-run inspects production without mutating store")),
		notApplicable("production storage is local production state; external inventory ledger integration is not in this MVP slice"),
	),
	releaseModuleProfileFor(
		"12-automation-routes",
		evidence("gameproject/internal/game/production", "TestCreateRouteStoresDetachedEnabledRoute", "automation route unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/production", "TestSettleRouteDoubleSettlementDoesNotDuplicateTransfer", "route settlement double-run does not duplicate transfer")),
		satisfied(evidence("gameproject/internal/game/production", "TestDisableRouteSettlesOldRouteBeforeDisabling", "route toggle around settlement is covered")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminDryRunsProductionAndRouteSettlementWithoutMutatingStore", "admin dry-run inspects route settlement without mutating store")),
		notApplicable("route settlement moves production storage in-memory; external item ledger integration is not in this MVP slice"),
	),
	releaseModuleProfileFor(
		"13-intel-coordinate-trading",
		evidence("gameproject/internal/game/discovery", "TestCreateCoordinateScrollStoresServerAuthoredMetadataFromKnownIntel", "coordinate scroll unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/discovery", "TestUseCoordinateScrollConsumesOnceAndWritesIntel", "coordinate scroll use consumes once and writes intel")),
		satisfied(evidence("gameproject/internal/game/discovery", "TestCreateCoordinateScrollDuplicateReferenceDoesNotMintTwice", "duplicate coordinate scroll creation does not mint twice")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminDisablesSuspiciousListingAndMarksIntelStale", "admin can mark stale listed intel")),
		satisfied(evidence("gameproject/internal/game/discovery", "TestUseCoordinateScrollDuplicateReferenceDoesNotConsumeTwice", "coordinate scroll consumption uses idempotent references")),
	),
	releaseModuleProfileFor(
		"14-world-aoi-fog-security",
		evidence("gameproject/internal/game/world/visibility", "TestCanSendEntityToClientRejectsHiddenEntity", "world visibility unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/world/worker", "TestCommandDrainOrderIsDeterministic", "world worker drains commands through a single owner in deterministic order")),
		satisfied(evidence("gameproject/internal/game/world/visibility", "TestCanInteractRejectsHiddenEntityWithGenericError", "hidden interaction fails with generic error")),
		notApplicable("world AOI/fog has no value-changing admin repair surface"),
		notApplicable("world AOI/fog visibility does not mutate item/currency ledgers"),
	),
	releaseModuleProfileFor(
		"15-api-events-errors",
		evidence("gameproject/internal/game/contracts", "TestRequestEnvelopeModelsRequestIDSeparatelyFromDomainIdempotency", "request envelope unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/realtime", "TestRequestCacheCoordinatesInFlightDuplicateRequestID", "realtime request cache coordinates duplicate in-flight requests")),
		satisfied(evidence("gameproject/internal/game/realtime", "TestEventEnvelopeMarshalsWithoutHiddenInternalFields", "client event envelope omits hidden internal fields")),
		notApplicable("API/event contracts have no value-changing admin repair surface"),
		notApplicable("API/event contracts do not mutate item/currency ledgers"),
	),
	releaseModuleProfileFor(
		"16-testing-observability-balancing",
		evidence("gameproject/internal/game/observability", "TestMetricHelpersRecordPhase12Series", "observability metric helper unit coverage exists"),
		satisfied(evidence("gameproject/internal/game/observability/simulations", "TestRouteSettlementSimulationTracksLossAndDuplicateNoOps", "simulation layer covers duplicate-safe route settlement accounting")),
		satisfied(evidence("gameproject/internal/game/observability", "TestPhase12AbuseTestCoverageCoversRequiredCases", "abuse coverage report covers every required Phase 12 abuse case")),
		notApplicable("observability repair is delivered through the admin module"),
		notApplicable("observability stores reports and metrics, not player item/currency value"),
	),
}

var phase12CommandSecurityProfiles = []commandSecurityProfile{
	realtimeCommandSecurityProfile("session.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestServerAuthRoutesAndWebSocketBootstrap", "session snapshot data is bootstrapped from an authenticated server session")),
		notApplicable("session.snapshot uses the server-resolved session subject instead of a client-owned entity id"),
		notApplicable("session.snapshot has no item/currency amount"),
		notApplicable("session.snapshot has no hidden target interaction"),
		notApplicable("session.snapshot has no item/currency mutation ledger"),
		notApplicable("session.snapshot is a read operation without mutation commit semantics"),
	),
	realtimeCommandSecurityProfile("world.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestBadPayloadReturnsSafeErrorAndLogoutRejectsFurtherCommands", "world snapshot rejects trusted client-owned identity and server-owned payload fields through the socket path")),
		notApplicable("world.snapshot uses the server-resolved session subject instead of a client-owned entity id"),
		notApplicable("world.snapshot has no item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestServerAuthRoutesAndWebSocketBootstrap", "world snapshot bootstrap filters hidden worker entities before serialization")),
		notApplicable("world.snapshot has no item/currency mutation ledger"),
		notApplicable("world.snapshot is a read operation without mutation commit semantics"),
	),
	realtimeCommandSecurityProfile("move_to",
		satisfied(evidence("gameproject/internal/game/world/worker", "TestMoveToCommandDoesNotExposeClientFinalPosition", "move command accepts intent and server computes final position")),
		notApplicable("move_to uses the server-resolved session subject instead of a client-owned entity id"),
		notApplicable("move_to has no item/currency amount"),
		notApplicable("move_to movement is not a hidden target interaction in the current worker slice"),
		notApplicable("move_to has no item/currency mutation ledger"),
		notApplicable("move_to has no durable value commit or realtime transport broadcast in the current in-process worker slice"),
	),
	realtimeCommandSecurityProfile("stop",
		satisfied(evidence("gameproject/internal/game/world/worker", "TestStopCommandClearsMovementTarget", "stop command sends intent to clear the server-owned movement target")),
		notApplicable("stop uses the server-resolved session subject instead of a client-owned entity id"),
		notApplicable("stop has no item/currency amount"),
		notApplicable("stop has no hidden target interaction"),
		notApplicable("stop has no item/currency mutation ledger"),
		notApplicable("stop has no durable value commit or realtime transport broadcast in the current in-process worker slice"),
	),
	realtimeCommandSecurityProfile("portal.enter",
		satisfied(evidence("gameproject/internal/game/server", "TestPortalEnterRejectsTrustedInternalFieldsWithoutMutation", "portal.enter accepts only portal_id intent and rejects client-authored map, worker, transfer, and destination fields")),
		satisfied(evidence("gameproject/internal/game/server", "TestPortalEnterTransfersPlayerAndAllActiveSessions", "portal.enter resolves the authenticated player, current source map, visible portal, destination spawn, and owned active sessions server-side")),
		notApplicable("portal.enter has no item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPortalEnterOutOfRangeAndCooldownAreNonMutating", "portal.enter validates proximity and cooldown before mutating map ownership or session routing")),
		notApplicable("portal.enter mutates map and session routing state, not item/currency value"),
		notApplicable("portal.enter queues in-memory transfer events after the server-owned handoff; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("debug_spawn_npc",
		satisfied(document("internal/game/realtime/envelope.go", "debug spawn is registered with debug-only rate-limit posture")),
		notApplicable("debug_spawn_npc has no player-owned id in the current operation contract"),
		notApplicable("debug_spawn_npc has no item/currency amount"),
		notApplicable("debug_spawn_npc is debug-only and not a player world interaction gate"),
		notApplicable("debug_spawn_npc has no item/currency mutation ledger"),
		notApplicable("debug_spawn_npc has no durable value commit in the current debug contract"),
	),
	realtimeCommandSecurityProfile("debug_snapshot",
		satisfied(document("internal/game/realtime/envelope.go", "debug snapshot is registered with debug-only rate-limit posture")),
		notApplicable("debug_snapshot has no player-owned id in the current operation contract"),
		notApplicable("debug_snapshot has no item/currency amount"),
		notApplicable("debug_snapshot is debug-only and not a player world interaction gate"),
		notApplicable("debug_snapshot has no item/currency mutation ledger"),
		notApplicable("debug_snapshot is a read/debug operation without commit/broadcast semantics"),
	),
	commandSecurityProfile{
		Command: "combat.use_skill",
		Checks: map[CommandSecurityCheck]gateCoverageSource{
			CommandSecurityIntentOnlyPayload: satisfied(
				evidence("gameproject/internal/game/runtime", "TestCombatUseSkillIgnoresClientTimestampForCooldown", "combat.use_skill ignores client_timestamp and uses server cooldown time"),
			),
			CommandSecurityServerPlayerSession: satisfied(
				evidence("gameproject/internal/game/realtime", "TestObservedCommandExecutorRequiresServerResolvedIdentity", "observed command executor requires server-resolved session and player identity"),
				evidence("gameproject/internal/game/realtime", "TestObservedCommandExecutorRequiresServerResolvedWorldAndZone", "observed command executor requires server-resolved world and zone identity"),
			),
			CommandSecurityOwnershipChecked: satisfied(
				evidence("gameproject/internal/game/runtime", "TestCombatUseSkillRejectsClientAuthoredAttackerID", "combat.use_skill rejects client-authored attacker ids and resolves the attacker from authenticated context"),
			),
			CommandSecurityPositiveBoundedAmounts: notApplicable("combat.use_skill carries no item/currency amount"),
			CommandSecurityVisibilityRangeChecked: satisfied(
				evidence("gameproject/internal/game/combat", "TestExecuteBasicAttackRejectsHiddenTarget", "combat service rejects hidden targets before mutation"),
				evidence("gameproject/internal/game/combat", "TestExecuteBasicAttackRejectsOutOfRangeTarget", "combat service rejects out-of-range targets before mutation"),
			),
			CommandSecurityTransactionLock: satisfied(
				evidence("gameproject/internal/game/combat", "TestSimultaneousLethalDamageProcessesNPCDeathOnce", "combat service serializes concurrent lethal damage so NPC death is processed once"),
			),
			CommandSecurityLedgerWrite: notApplicable("combat.use_skill mutates live combat resources and cooldowns, not item/currency value"),
			CommandSecurityIdempotency: satisfied(
				evidence("gameproject/internal/game/realtime", "TestRequestCacheCoordinatesInFlightDuplicateRequestID", "request cache coordinates in-flight duplicate request IDs"),
			),
			CommandSecurityLeakSafeError: satisfied(
				evidence("gameproject/internal/game/runtime", "TestCombatUseSkillIgnoresClientTimestampForCooldown", "combat.use_skill returns a stable cooldown code for rejected timestamp spoof attempts"),
			),
			CommandSecurityBroadcastAfterCommit: notApplicable("combat.use_skill currently mutates in-memory combat state; durable commit/outbox broadcast is not part of this Phase 05 gateway slice"),
		},
	},
	realtimeCommandSecurityProfile("loot.pickup",
		satisfied(evidence("gameproject/internal/game/server", "TestCombatKillCreatesLootAndPickupUpdatesCargo", "loot.pickup accepts only a drop id and derives player, cargo, and item contents server-side")),
		notApplicable("loot.pickup uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("loot.pickup does not accept client-authored quantity or item amount"),
		satisfied(evidence("gameproject/internal/game/loot", "TestPickupDropRejectsFarHiddenAndCargoFullWithoutClaim", "loot service rejects hidden, far, and cargo-full pickup attempts before claim")),
		satisfied(evidence("gameproject/internal/game/economy", "TestAddItemWritesItemLedgerEntryWithReasonAndReference", "loot pickup cargo add writes item ledger entries with reason and reference")),
		notApplicable("loot.pickup broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("repair.shield_tick",
		satisfied(evidence("gameproject/internal/game/server", "TestShieldRepairTickRejectsClientAuthoredRepairTruth", "repair.shield_tick rejects client-authored shield amount, repair rate, and elapsed time")),
		satisfied(evidence("gameproject/internal/game/server", "TestShieldRepairTickRequiresEquippedShieldRepairModule", "repair.shield_tick derives module ownership and repair ability from server-owned loadout state")),
		notApplicable("repair.shield_tick does not accept a client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestShieldRepairTickRespectsCombatLockAndRepairsOnlyShield", "repair.shield_tick rejects combat-locked repair attempts and only restores shield")),
		notApplicable("repair.shield_tick mutates live shield state without item/currency ledger entries"),
		notApplicable("repair.shield_tick broadcasts in-memory ship/player snapshots after server mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("death.repair_quote",
		satisfied(evidence("gameproject/internal/game/server", "TestRepairQuoteAndRepairUseServerOwnedActiveShip", "repair quote ignores client ship/cost data and derives the active ship server-side")),
		notApplicable("death.repair_quote uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("death.repair_quote is read-only and does not accept a client-authored amount"),
		notApplicable("death.repair_quote has no hidden target interaction"),
		notApplicable("death.repair_quote has no item/currency mutation ledger"),
		notApplicable("death.repair_quote is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("death.repair_ship",
		satisfied(evidence("gameproject/internal/game/server", "TestRepairQuoteAndRepairUseServerOwnedActiveShip", "repair command derives active ship, cost, and wallet mutation from server state")),
		notApplicable("death.repair_ship uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("death.repair_ship does not accept a client-authored repair cost"),
		notApplicable("death.repair_ship has no hidden target interaction"),
		satisfied(evidence("gameproject/internal/game/death", "TestRepairServiceDebitsServerCalculatedCostAndRestoresShipAvailable", "death repair service debits the server-calculated credit cost through wallet ledger entries")),
		notApplicable("death.repair_ship broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("progression.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase06SnapshotQueriesUseServerResolvedState", "progression.snapshot is read-only and rejects client-authored progression truth")),
		notApplicable("progression.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("progression.snapshot has no client-authored item/currency amount"),
		notApplicable("progression.snapshot has no hidden target interaction"),
		notApplicable("progression.snapshot has no item/currency mutation ledger"),
		notApplicable("progression.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("progression.unlock_skill",
		satisfied(evidence("gameproject/internal/game/server", "TestProgressionUnlockSkillConsumesExactlyOnePoint", "progression.unlock_skill accepts only a node_id intent and resolves the player from the authenticated session subject")),
		satisfied(evidence("gameproject/internal/game/progression", "TestUnlockPilotSkillValidatesLockedNodesAndConsumesPointOnce", "skill node cost, prerequisites, rank, role, and available skill points are validated server-side before spending")),
		notApplicable("progression.unlock_skill does not accept a client-authored item quantity or currency amount; the node cost is server-owned"),
		notApplicable("progression.unlock_skill has no hidden target interaction"),
		satisfied(evidence("gameproject/internal/game/progression", "TestUnlockPilotSkillValidatesLockedNodesAndConsumesPointOnce", "skill point spend is a server-owned progression store mutation that is idempotent on duplicate unlock")),
		satisfied(evidence("gameproject/internal/game/server", "TestProgressionUnlockSkillConsumesExactlyOnePoint", "progression snapshot is queued to the session after the server-owned mutation commits")),
	),
	realtimeCommandSecurityProfile("inventory.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase06SnapshotQueriesUseServerResolvedState", "inventory.snapshot derives owned item rows from server inventory state")),
		notApplicable("inventory.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("inventory.snapshot has no client-authored item/currency amount"),
		notApplicable("inventory.snapshot has no hidden target interaction"),
		notApplicable("inventory.snapshot has no item/currency mutation ledger"),
		notApplicable("inventory.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("hangar.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase06SnapshotQueriesUseServerResolvedState", "hangar.snapshot derives the active ship from server runtime state")),
		notApplicable("hangar.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("hangar.snapshot has no client-authored item/currency amount"),
		notApplicable("hangar.snapshot has no hidden target interaction"),
		notApplicable("hangar.snapshot has no item/currency mutation ledger"),
		notApplicable("hangar.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("hangar.activate_ship",
		satisfied(evidence("gameproject/internal/game/server", "TestHangarActivateShipUsesServerOwnedHangarState", "hangar.activate_ship accepts only a ship id intent and rejects trusted payload fields")),
		satisfied(
			evidence("gameproject/internal/game/server", "TestHangarActivateShipUsesServerOwnedHangarState", "hangar.activate_ship resolves the player and owned active hangar state server-side"),
			evidence("gameproject/internal/game/ships", "TestSetActiveShipRejectsCombatUnsafeCargoAndDisabledTargets", "ship activation validation rejects combat, unsafe area, cargo overflow, disabled, and repairing ships"),
		),
		notApplicable("hangar.activate_ship does not accept a client-authored item quantity or currency amount"),
		notApplicable("hangar.activate_ship has no hidden target interaction; safe area is derived server-side"),
		notApplicable("hangar.activate_ship mutates hangar/ship state, not item/currency ledger value"),
		notApplicable("hangar.activate_ship queues in-memory hangar, ship, stats, cargo, and loadout events after mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("loadout.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase06SnapshotQueriesUseServerResolvedState", "loadout.snapshot derives empty slot state from the server-owned active ship")),
		notApplicable("loadout.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("loadout.snapshot has no client-authored item/currency amount"),
		notApplicable("loadout.snapshot has no hidden target interaction"),
		notApplicable("loadout.snapshot has no item/currency mutation ledger"),
		notApplicable("loadout.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("loadout.equip_module",
		satisfied(evidence("gameproject/internal/game/server", "TestLoadoutEquipAndUnequipMutateServerOwnedInventory", "loadout.equip_module accepts only slot and item intent and rejects trusted payload fields")),
		satisfied(
			evidence("gameproject/internal/game/server", "TestLoadoutEquipAndUnequipMutateServerOwnedInventory", "loadout.equip_module resolves the player, active ship, owned item, slot, and module type server-side"),
			evidence("gameproject/internal/game/modules", "TestSaveLoadoutRejectsInvalidModuleAssignments", "module assignment validation rejects wrong slots, broken modules, duplicate module use, bad owner, and bad item location"),
		),
		notApplicable("loadout.equip_module does not accept a client-authored item quantity or currency amount"),
		notApplicable("loadout.equip_module has no hidden target interaction; module ownership and location are validated server-side"),
		satisfied(evidence("gameproject/internal/game/server", "TestLoadoutEquipAndUnequipMutateServerOwnedInventory", "equip moves the module instance through the server inventory location path before snapshot reconciliation")),
		notApplicable("loadout.equip_module queues in-memory inventory, loadout, and stats events after mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("loadout.unequip_module",
		satisfied(evidence("gameproject/internal/game/server", "TestLoadoutEquipAndUnequipMutateServerOwnedInventory", "loadout.unequip_module accepts only a slot intent and rejects trusted payload fields through the same strict decode path")),
		satisfied(evidence("gameproject/internal/game/server", "TestLoadoutEquipAndUnequipMutateServerOwnedInventory", "loadout.unequip_module resolves the player, active ship, slot layout, and equipped module server-side")),
		notApplicable("loadout.unequip_module does not accept a client-authored item quantity or currency amount"),
		notApplicable("loadout.unequip_module has no hidden target interaction; equipped module state is validated server-side"),
		satisfied(evidence("gameproject/internal/game/server", "TestLoadoutEquipAndUnequipMutateServerOwnedInventory", "unequip returns the module instance through the server inventory location path before snapshot reconciliation")),
		notApplicable("loadout.unequip_module queues in-memory inventory, loadout, and stats events after mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("stats.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase06SnapshotQueriesUseServerResolvedState", "stats.snapshot derives effective display stats from server runtime state")),
		notApplicable("stats.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("stats.snapshot has no client-authored item/currency amount"),
		notApplicable("stats.snapshot has no hidden target interaction"),
		notApplicable("stats.snapshot has no item/currency mutation ledger"),
		notApplicable("stats.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("stealth.toggle",
		satisfied(evidence("gameproject/internal/game/server", "TestStealthToggleCommandUsesServerOwnedStateAndSafePayload", "stealth.toggle accepts only an enabled intent and rejects client-authored hidden truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestStealthToggleCommandUsesServerOwnedStateAndSafePayload", "stealth.toggle resolves the player from the authenticated server session and mutates only that player")),
		notApplicable("stealth.toggle has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestStealthToggleCommandUsesServerOwnedStateAndSafePayload", "stealth.toggle emits only safe stats and self stealthed AOI state without hidden/player id leaks")),
		notApplicable("stealth.toggle mutates live visibility and movement speed, not item/currency ledger value"),
		notApplicable("stealth.toggle queues in-memory stats and AOI diff events after the worker speed mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("crafting.recipes",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase06SnapshotQueriesUseServerResolvedState", "crafting.recipes returns the server recipe catalog without accepting client-authored materials or outputs")),
		notApplicable("crafting.recipes uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("crafting.recipes has no client-authored item/currency amount"),
		notApplicable("crafting.recipes has no hidden target interaction"),
		notApplicable("crafting.recipes has no item/currency mutation ledger"),
		notApplicable("crafting.recipes is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("crafting.start",
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.start accepts only recipe intent and rejects spoofed material, wallet, and player-owned truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.start resolves player, inventory, wallet, recipe, reservation, and craft location server-side")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.start validates server-owned recipe cost and inventory quantities")),
		notApplicable("crafting.start has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.start writes server-owned reservation and wallet ledger state")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.start returns reconciled wallet, inventory, and crafting snapshots after mutation")),
	),
	realtimeCommandSecurityProfile("crafting.complete",
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.complete accepts only job_id and rejects client-authored output or reward truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.complete resolves job ownership through the authenticated player")),
		notApplicable("crafting.complete carries no client-authored item/currency amount"),
		notApplicable("crafting.complete has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.complete commits server-owned output and progression state exactly once")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingStartAndCompleteUseServerOwnedEconomyState", "crafting.complete returns reconciled inventory and progression snapshots after commit")),
	),
	realtimeCommandSecurityProfile("crafting.cancel",
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingCancelRefundsReservedServerOwnedState", "crafting.cancel accepts only job_id and rejects client-authored refund, wallet, and reservation truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingCancelRefundsReservedServerOwnedState", "crafting.cancel resolves job ownership through the authenticated player")),
		notApplicable("crafting.cancel carries no client-authored item/currency amount"),
		notApplicable("crafting.cancel has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingCancelRefundsReservedServerOwnedState", "crafting.cancel releases reservations and refunds wallet ledger state exactly once")),
		satisfied(evidence("gameproject/internal/game/server", "TestCraftingCancelRefundsReservedServerOwnedState", "crafting.cancel returns reconciled wallet, inventory, and crafting snapshots after commit")),
	),
	realtimeCommandSecurityProfile("scan.pulse",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "scan.pulse accepts no client scan result, candidate, seed, or coordinate truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "scan.pulse resolves player, ship, world, zone, stats, energy, and scanner capability server-side")),
		notApplicable("scan.pulse has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "scan.pulse returns only safe signal projections and known intel after server-side discovery")),
		notApplicable("scan.pulse grants progression XP but does not mutate item/currency ledger state"),
		notApplicable("scan.pulse broadcasts in-memory realtime events after server discovery mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("discovery.claim_planet",
		satisfied(evidence("gameproject/internal/game/server", "TestClaimPlanetRejectsTrustedAndUnknownPayloadFieldsWithoutMutation", "discovery.claim_planet accepts only planet_id and rejects client-authored player/map/coordinate/owner/X Core truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestClaimPlanetSucceedsForKnownNearbyPlanetAndEmitsSafeOwnerEvents", "discovery.claim_planet resolves the authenticated player, active map, position, rank, X Core source, and production defaults server-side")),
		satisfied(evidence("gameproject/internal/game/server", "TestClaimPlanetRejectsMissingXCoreWithoutOwnerProductionOrEvents", "discovery.claim_planet requires one server-owned X Core and rejects missing item state without owner or production mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestClaimPlanetRejectsCrossMapKnownPlanet", "discovery.claim_planet rejects known planets outside the player's active map and claim range")),
		satisfied(evidence("gameproject/internal/game/server", "TestClaimPlanetDuplicateRetryDoesNotConsumeSecondXCore", "discovery.claim_planet consumes one X Core through inventory ledger idempotency and duplicate retries do not consume a second item")),
		notApplicable("discovery.claim_planet queues in-memory owner-scoped claim, planet, production, and inventory events after the claim service mutation; durable outbox remains deferred for Phase07A"),
	),
	realtimeCommandSecurityProfile("discovery.known_planets",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "discovery.known_planets derives personal intel from server discovery state")),
		notApplicable("discovery.known_planets uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("discovery.known_planets has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "discovery.known_planets returns only planets for which the player has server-written intel")),
		notApplicable("discovery.known_planets has no item/currency mutation ledger"),
		notApplicable("discovery.known_planets is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("discovery.planet_detail",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "discovery.planet_detail accepts only a planet id and rejects hidden scan/candidate truth")),
		notApplicable("discovery.planet_detail uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("discovery.planet_detail has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "discovery.planet_detail requires existing player intel before returning coordinates or planet data")),
		notApplicable("discovery.planet_detail has no item/currency mutation ledger"),
		notApplicable("discovery.planet_detail is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("intel.share",
		satisfied(evidence("gameproject/internal/game/server", "TestIntelShareRejectsSpoofedServerOwnedFieldsBeforeMutation", "intel.share accepts only planet_id and to_player_id while rejecting client-authored source, coordinate, confidence, and ownership truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestIntelShareUsesServerOwnedKnownPlanetAndQueuesReceiverIntel", "intel.share resolves the sender from the authenticated session and copies only server-known source intel to the receiver")),
		notApplicable("intel.share has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestIntelShareUsesServerOwnedKnownPlanetAndQueuesReceiverIntel", "intel.share requires existing sender intel before the receiver can learn a planet")),
		notApplicable("intel.share mutates personal intel memory but does not mutate item/currency ledger state"),
		notApplicable("intel.share queues in-memory receiver known-planets events after the intel mutation; durable outbox remains deferred for Phase07 intel persistence hardening"),
	),
	realtimeCommandSecurityProfile("intel.coordinate_item.create",
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemCreateRejectsSpoofedServerOwnedFieldsBeforeMutation", "intel.coordinate_item.create accepts only planet_id and rejects client-authored item id, coordinates, source, state, confidence, timestamps, and inventory truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemCreateAndUseConsumeOnceAndRefreshDiscovery", "intel.coordinate_item.create derives player id and coordinate item identity server-side from authenticated context and request id")),
		notApplicable("intel.coordinate_item.create does not accept a client-authored item/currency amount in this temporary server-owned intel slice"),
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemCreateRequiresKnownPlanet", "intel.coordinate_item.create requires existing server-known player intel before creating a coordinate payload")),
		notApplicable("intel.coordinate_item.create creates temporary server-owned intel state; inventory-backed ledger consumption is tracked as remaining hardening work"),
		notApplicable("intel.coordinate_item.create is actor-scoped and returns its committed server-owned coordinate item payload without AOI broadcast"),
	),
	realtimeCommandSecurityProfile("intel.coordinate_item.use",
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemUseRejectsSpoofedServerOwnedFieldsBeforeMutation", "intel.coordinate_item.use accepts only item_instance_id and rejects client-authored planet, coordinate, source, state, confidence, timestamps, and inventory truth")),
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemUseRejectsWrongOwner", "intel.coordinate_item.use derives player id from the authenticated session and rejects item ownership mismatch")),
		notApplicable("intel.coordinate_item.use has no client-authored item/currency amount in this temporary server-owned intel slice"),
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemCreateAndUseConsumeOnceAndRefreshDiscovery", "intel.coordinate_item.use reveals only the coordinate item's server-owned planet memory to the owning player")),
		notApplicable("intel.coordinate_item.use consumes temporary server-owned intel state; inventory-backed item removal and ledger evidence remain deferred hardening work"),
		satisfied(evidence("gameproject/internal/game/server", "TestCoordinateItemCreateAndUseConsumeOnceAndRefreshDiscovery", "intel.coordinate_item.use queues owner-scoped known-planets and planet-detail refreshes after the use mutation")),
	),
	realtimeCommandSecurityProfile("planet.production_summary",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "planet.production_summary derives owned planet production snapshots server-side")),
		notApplicable("planet.production_summary uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("planet.production_summary has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "planet.production_summary filters snapshots by server-owned planet ownership")),
		notApplicable("planet.production_summary has no item/currency mutation ledger"),
		notApplicable("planet.production_summary is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("planet.storage_summary",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "planet.storage_summary derives owned planet storage snapshots server-side")),
		notApplicable("planet.storage_summary uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("planet.storage_summary has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "planet.storage_summary filters snapshots by server-owned planet ownership")),
		notApplicable("planet.storage_summary has no item/currency mutation ledger"),
		notApplicable("planet.storage_summary is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("planet.building_build",
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingBuildRejectsSpoofedServerOwnedFieldsBeforeMutation", "planet.building_build accepts only planet id, building type, and slot while rejecting client-authored owner, wallet, storage, material, definition, level, and map truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingBuildAlloyFoundryDebitsServerOwnedCostsAndQueuesOwnerEvents", "planet.building_build derives the owner from the authenticated command context and queues events only to owner sessions")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingBuildAlloyFoundryDebitsServerOwnedCostsAndQueuesOwnerEvents", "planet.building_build derives material and wallet costs from the server building catalog instead of client-authored amounts")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingBuildRejectsWrongOwnerAndOtherMapWithoutMutation", "planet.building_build rejects wrong-owner and inactive-map planets without mutation, wallet debit, or events")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingBuildAlloyFoundryDebitsServerOwnedCostsAndQueuesOwnerEvents", "planet.building_build writes production material and wallet ledgers once, and duplicate retries do not debit again")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingConcurrentBuildsSerializeBeforeWalletDebit", "planet.building_build serializes gateway mutations so a second build cannot debit wallet before the first production commit resolves")),
	),
	realtimeCommandSecurityProfile("planet.building_upgrade",
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel", "planet.building_upgrade accepts planet id, building id, and a single target level while rejecting conflicting target/next level payloads before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel", "planet.building_upgrade derives the owner from the authenticated command context and mutates only the owned active-map production planet")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel", "planet.building_upgrade derives material and wallet costs from the server building catalog instead of client-authored amounts")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel", "planet.building_upgrade validates the stored building and target level through server-owned production state")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel", "planet.building_upgrade writes production material and wallet ledgers once, and duplicate retries do not debit again")),
		satisfied(evidence("gameproject/internal/game/server", "TestPlanetBuildingUpgradeIronExtractorDebitsCostsOnceAndUpdatesLevel", "planet.building_upgrade queues owner-scoped safe production, storage, and wallet snapshots after mutation")),
	),
	realtimeCommandSecurityProfile("route.create",
		satisfied(evidence("gameproject/internal/game/server", "TestRouteCreateRejectsSpoofedServerOwnedFieldsBeforeMutation", "route.create accepts only source/destination planet ids, resource item id, and amount per hour, rejecting client-authored owner/map/route/energy/risk truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteCreateCreatesOwnedPlanetRouteThroughGateway", "route.create derives the route owner from the authenticated command context and stores that server-resolved owner on the route row")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteCreateRejectsXCoreResourceBeforeMutation", "route.create uses the server-owned routeable resource allowlist and rejects X Core without route mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteCreateCreatesOwnedPlanetRouteThroughGateway", "route.create derives endpoint ownership/access and source/destination map ids from server-owned discovery planets and map catalog rows")),
		notApplicable("route.create registers a virtual automation route but does not move item/currency ledger value until future settlement flows"),
		notApplicable("route.create queues in-memory owner-scoped route.updated, route.snapshot, and route.list events after production route creation; durable outbox remains deferred for route persistence work"),
	),
	realtimeCommandSecurityProfile("route.update",
		satisfied(evidence("gameproject/internal/game/server", "TestRouteUpdateRejectsSpoofedServerOwnedFieldsBeforeMutation", "route.update accepts only route id, destination planet id, resource item id, and amount per hour, rejecting client-authored owner/map/source/enabled/settlement/storage/energy/risk truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteUpdateChangesOwnedRouteTermsThroughGateway", "route.update derives the route owner from the authenticated command context and preserves the server-owned source/owner route facts")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteUpdateRejectsXCoreResourceBeforeMutation", "route.update uses the server-owned routeable resource allowlist and rejects X Core without route mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteUpdateRejectsWrongOwnerWithoutMutationOrEvents", "route.update filters route access by server-owned route owner and returns a safe not-found/forbidden error without mutation")),
		notApplicable("route.update may settle production storage in-memory before replacing terms; external item/currency ledger integration remains deferred with durable route settlement work"),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteUpdateSettlesElapsedStorageAndQueuesActiveMapSnapshots", "route.update queues owner-scoped safe route events and active-map production/storage snapshots after settlement touches storage")),
	),
	realtimeCommandSecurityProfile("route.enable",
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlRejectsSpoofedServerOwnedFieldsBeforeMutation", "route.enable accepts only route_id and rejects client-authored owner/map/enabled/settlement/source/destination/risk/storage truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlRejectsWrongOwnerWithoutMutationOrEvents", "route.enable derives the route owner from the authenticated command context and rejects wrong-owner routes without mutation")),
		notApplicable("route.enable has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlRejectsWrongOwnerWithoutMutationOrEvents", "route.enable filters route access by server-owned route owner and returns a safe not-found/forbidden error")),
		notApplicable("route.enable resets the settlement clock but does not move item/currency ledger value in this gateway slice"),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlDisableThenEnableThroughGatewayQueuesSafeOwnerEvents", "route.enable queues only owner-scoped safe route.updated, route.snapshot, and route.list events after the production route control mutation")),
	),
	realtimeCommandSecurityProfile("route.disable",
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlRejectsSpoofedServerOwnedFieldsBeforeMutation", "route.disable accepts only route_id and rejects client-authored owner/map/enabled/settlement/source/destination/risk/storage truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlRejectsWrongOwnerWithoutMutationOrEvents", "route.disable derives the route owner from the authenticated command context and rejects wrong-owner routes without mutation")),
		notApplicable("route.disable has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteControlRejectsWrongOwnerWithoutMutationOrEvents", "route.disable filters route access by server-owned route owner and returns a safe not-found/forbidden error")),
		notApplicable("route.disable may settle production storage in-memory before disabling; external item/currency ledger integration remains deferred with durable route settlement work"),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteDisableSettlesStorageAndQueuesSafeProductionStorageSnapshots", "route.disable queues owner-scoped safe route events and active-map production/storage snapshots after settlement touches storage")),
	),
	realtimeCommandSecurityProfile("route.settle",
		satisfied(evidence("gameproject/internal/game/server", "TestRouteSettleRejectsSpoofedServerOwnedFieldsBeforeMutation", "route.settle accepts only route_id or an empty owner reconcile intent and rejects client-authored owner/map/source/destination/settlement/storage/energy/risk/amount truth before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteSettleTransfersStorageAndQueuesSafeOwnerEvents", "route.settle derives the route owner from the authenticated command context and queues owner-scoped safe settlement events")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteSettleRejectsSpoofedServerOwnedFieldsBeforeMutation", "route.settle rejects client-authored wanted/taken/lost/delivered/added/resource/rate facts before mutation")),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteSettleRejectsWrongOwnerWithoutMutationOrEvents", "route.settle filters route access by server-owned route owner and returns a safe not-found error without mutation")),
		notApplicable("route.settle moves production storage in-memory in this MVP; external item/currency ledger integration remains deferred with durable route settlement work"),
		satisfied(evidence("gameproject/internal/game/server", "TestRouteSettleTransfersStorageAndQueuesSafeOwnerEvents", "route.settle queues owner-scoped route.settled, route.updated, route.snapshot, route.list, and active-map production/storage snapshots after settlement")),
	),
	realtimeCommandSecurityProfile("route.list",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "route.list derives automation routes from server production state")),
		notApplicable("route.list uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("route.list has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "route.list filters routes by server-owned route owner")),
		notApplicable("route.list has no item/currency mutation ledger"),
		notApplicable("route.list is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("route.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "route.snapshot accepts only a route id and rejects hidden scan/candidate truth")),
		notApplicable("route.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("route.snapshot has no client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase07DiscoveryProductionRouteQueriesUseServerState", "route.snapshot filters route access by server-owned route owner")),
		notApplicable("route.snapshot has no item/currency mutation ledger"),
		notApplicable("route.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("wallet.snapshot",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "wallet.snapshot returns balances from the server wallet service")),
		notApplicable("wallet.snapshot uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("wallet.snapshot has no client-authored amount"),
		notApplicable("wallet.snapshot has no hidden world target interaction"),
		notApplicable("wallet.snapshot has no item/currency mutation ledger"),
		notApplicable("wallet.snapshot is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("content.catalog",
		satisfied(evidence("gameproject/internal/game/server", "TestContentCatalogReturnsSafePlayerProjection", "content.catalog returns a server-owned player-safe projection and rejects client-authored player identity")),
		notApplicable("content.catalog uses the authenticated socket session and does not accept a client-owned player id"),
		notApplicable("content.catalog is read-only and does not accept a client-authored item/currency amount"),
		satisfied(evidence("gameproject/internal/game/server", "TestContentCatalogReturnsSafePlayerProjection", "content.catalog omits server-only catalog, spawn, loot, audit, and procedural fields from the player projection")),
		notApplicable("content.catalog has no item/currency mutation ledger"),
		notApplicable("content.catalog is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("shop.catalog",
		satisfied(evidence("gameproject/internal/game/server", "TestShopCatalogUsesServerOwnedGameCatalog", "shop.catalog returns server-owned categories/products and rejects client-authored catalog truth")),
		notApplicable("shop.catalog returns global server catalog data instead of a player-owned entity id"),
		notApplicable("shop.catalog is read-only and does not accept a client-authored item/currency amount"),
		notApplicable("shop.catalog has no hidden world target interaction in the current system shop catalog slice"),
		notApplicable("shop.catalog has no item/currency mutation ledger"),
		notApplicable("shop.catalog is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("shop.buy_product",
		satisfied(evidence("gameproject/internal/game/server", "TestShopBuyProductDebitsWalletAndGrantsServerCatalogProduct", "shop.buy_product accepts product id/quantity intent and rejects client-authored price/stock truth")),
		notApplicable("shop.buy_product resolves the buyer from the authenticated session rather than client player id"),
		satisfied(evidence("gameproject/internal/game/server", "TestShopBuyProductDebitsWalletAndGrantsServerCatalogProduct", "shop.buy_product derives server price and debits wallet by server-owned catalog policy")),
		notApplicable("shop.buy_product has no hidden world target interaction in the current system shop catalog slice"),
		satisfied(evidence("gameproject/internal/game/server", "TestShopBuyProductDebitsWalletAndGrantsServerCatalogProduct", "shop.buy_product writes wallet and item ledgers through wallet/inventory services")),
		notApplicable("shop.buy_product queues in-memory wallet/inventory/hangar snapshots after mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("market.search",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "market.search returns filtered listing summaries without seller/player/escrow fields")),
		notApplicable("market.search uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("market.search has no client-authored amount"),
		notApplicable("market.search has no hidden world target interaction"),
		notApplicable("market.search has no item/currency mutation ledger"),
		notApplicable("market.search is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("market.create_listing",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "market.create_listing rejects client-authored economy truth through the shared trusted-field filter")),
		satisfied(evidence("gameproject/internal/game/market", "TestCreateListingMovesStackableItemsIntoEscrow", "market create validates seller-owned quantity and moves it to escrow")),
		satisfied(evidence("gameproject/internal/game/market", "TestCreateListingRejectsInvalidInputsWithoutMutation", "market create rejects invalid quantity and unit price before mutation")),
		notApplicable("market.create_listing has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/economy", "TestSystemMoveItemMovesStackableToMarketEscrowAndBack", "market create escrow movement writes item ledger rows")),
		notApplicable("market.create_listing broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("market.buy",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "market.buy rejects forged total_amount and recalculates price server-side")),
		notApplicable("market.buy uses the server-resolved session subject instead of a client-owned buyer id"),
		satisfied(evidence("gameproject/internal/game/market", "TestBuyListingInsufficientFundsLeavesListingAndEscrowUnchanged", "market buy validates wallet funds before settlement")),
		notApplicable("market.buy has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/market", "TestBuyListingTransfersItemsCurrencyAndRecordsTotals", "market buy debits buyer, credits seller/fee sink, and moves escrowed item once")),
		notApplicable("market.buy broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("market.cancel",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "market.cancel accepts only a listing id and resolves seller from session")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "market cancel resolves seller ownership from the authenticated session")),
		notApplicable("market.cancel has no client-authored amount"),
		notApplicable("market.cancel has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/market", "TestCancelListingReturnsRemainingEscrowAndDuplicateDoesNotReturnTwice", "market cancel returns escrow through item ledger movement once")),
		notApplicable("market.cancel broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("auction.search",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "auction.search returns lot summaries without bidder/player/world ids")),
		notApplicable("auction.search uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("auction.search has no client-authored amount"),
		notApplicable("auction.search has no hidden world target interaction"),
		notApplicable("auction.search has no item/currency mutation ledger"),
		notApplicable("auction.search is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("auction.bid",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "auction.bid accepts only auction id and bid amount while deriving bidder from session")),
		notApplicable("auction.bid uses the server-resolved session subject instead of a client-owned bidder id"),
		satisfied(evidence("gameproject/internal/game/auction", "TestPlaceBidRejectsTooLowAndEndedWithoutDebit", "auction bid validates bounded bid amounts before debit")),
		notApplicable("auction.bid has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/auction", "TestPlaceBidRefundsPreviousBidder", "auction bid writes wallet debit and refund ledger rows")),
		notApplicable("auction.bid broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("auction.buy_now",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "auction.buy_now accepts only auction id and recalculates buy-now price server-side")),
		notApplicable("auction.buy_now uses the server-resolved session subject instead of a client-owned buyer id"),
		notApplicable("auction.buy_now does not accept a client-authored amount"),
		notApplicable("auction.buy_now has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/auction", "TestBuyNowDebitsRefundsGrantsAndClosesOnce", "auction buy-now debits wallet, refunds current bidder, and records one grant")),
		notApplicable("auction.buy_now broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("auction.grants",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "auction.grants returns only grants for the authenticated player")),
		notApplicable("auction.grants uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("auction.grants has no client-authored amount"),
		notApplicable("auction.grants has no hidden world target interaction"),
		notApplicable("auction.grants exposes skeleton grant snapshots; concrete item/unlock grant adapters remain future work"),
		notApplicable("auction.grants is read-only in the current skeleton grant slice"),
	),
	realtimeCommandSecurityProfile("premium.entitlements",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "premium.entitlements filters provider-created entitlements by authenticated player")),
		notApplicable("premium.entitlements uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("premium.entitlements has no client-authored amount"),
		notApplicable("premium.entitlements has no hidden world target interaction"),
		notApplicable("premium.entitlements has no item/currency mutation ledger"),
		notApplicable("premium.entitlements is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("premium.claim",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "premium.claim accepts only entitlement id and resolves player server-side")),
		satisfied(evidence("gameproject/internal/game/premium", "TestClaimRejectsWrongPlayerAndDifferentRequestAfterClaim", "premium claim checks entitlement ownership")),
		notApplicable("premium.claim does not accept a client-authored grant amount"),
		notApplicable("premium.claim has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/premium", "TestClaimPremiumCurrencyPackCreditsPremiumPaidOnce", "premium currency entitlement claim writes wallet ledger credit once")),
		notApplicable("premium.claim broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("premium.purchase_weekly_xcore",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "premium.purchase_weekly_xcore accepts no client price, stock, or limit truth")),
		notApplicable("premium.purchase_weekly_xcore uses the server-resolved session subject instead of a client-owned player id"),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "weekly X Core purchase debits the server-configured premium price and rejects a second purchase")),
		notApplicable("premium.purchase_weekly_xcore has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/premium", "TestWeeklyXCorePurchaseEnforcesOnePerPlayerPerPeriod", "weekly X Core purchase consumes stock once per player period")),
		notApplicable("premium.purchase_weekly_xcore broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("quest.board",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "quest.board generates offers from server-owned progression and discovery snapshots")),
		notApplicable("quest.board uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("quest.board has no client-authored amount"),
		notApplicable("quest.board has no hidden world target interaction"),
		notApplicable("quest.board is read/generation state without item/currency mutation ledger"),
		notApplicable("quest.board returns a server-owned board snapshot and only broadcasts generation after storing offers"),
	),
	realtimeCommandSecurityProfile("quest.accept",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "quest.accept accepts only an offer id and rejects client-authored quest truth")),
		satisfied(evidence("gameproject/internal/game/quests", "TestAcceptQuestRejectsWrongPlayerOffer", "quest acceptance validates offer ownership")),
		notApplicable("quest.accept has no client-authored amount"),
		notApplicable("quest.accept has no hidden world target interaction"),
		notApplicable("quest.accept stores accepted quest state without item/currency mutation ledger"),
		notApplicable("quest.accept broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("quest.progress",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "quest.progress is a read query and rejects authored progress payload fields")),
		notApplicable("quest.progress uses the server-resolved session subject instead of a client-owned player id"),
		notApplicable("quest.progress has no client-authored amount"),
		notApplicable("quest.progress has no hidden world target interaction"),
		notApplicable("quest.progress is read-only without item/currency mutation ledger"),
		notApplicable("quest.progress is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("quest.claim_reward",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "quest.claim_reward accepts only a quest id and resolves player ownership server-side")),
		satisfied(evidence("gameproject/internal/game/quests", "TestClaimRewardRejectsIncompleteQuestWithoutGrants", "quest reward claim validates completed state before grants")),
		notApplicable("quest.claim_reward does not accept client-authored reward amounts"),
		notApplicable("quest.claim_reward has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/quests", "TestClaimRewardGrantsAllRewardsOnceWithQuestRewardReference", "quest reward claim writes wallet, inventory, and XP grants through quest_reward references")),
		notApplicable("quest.claim_reward broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("quest.reroll",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "quest.reroll accepts no client-authored seed, cost, reward, or offer truth")),
		notApplicable("quest.reroll uses the server-resolved session subject instead of a client-owned player id"),
		satisfied(evidence("gameproject/internal/game/quests", "TestRerollBoardChargesCreditsOnce", "reroll cost is calculated from server-owned player state and debited once")),
		notApplicable("quest.reroll has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/quests", "TestRerollBoardDuplicateReferenceDoesNotDoubleCharge", "quest reroll debits wallet once by reroll idempotency key")),
		notApplicable("quest.reroll broadcasts in-memory realtime events after the service mutation; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("admin.inspect_player",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "admin.inspect_player rejects non-admin sessions and returns redacted allowlisted fields")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "admin.inspect_player resolves the admin role from the authenticated server session")),
		notApplicable("admin.inspect_player has no client-authored amount"),
		notApplicable("admin.inspect_player has no hidden world target interaction"),
		notApplicable("admin.inspect_player is an inspection read model without item/currency mutation ledger"),
		notApplicable("admin.inspect_player is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("admin.repair_craft_job",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "admin.repair_craft_job rejects non-admin sessions and accepts only a job id")),
		satisfied(evidence("gameproject/internal/game/admin", "TestAdminRepairsReadyCraftJobThroughCompletion", "admin craft repair validates job ownership and completion state through the crafting service")),
		notApplicable("admin.repair_craft_job has no client-authored item or currency amount"),
		notApplicable("admin.repair_craft_job has no hidden world target interaction"),
		satisfied(evidence("gameproject/internal/game/crafting", "TestCompleteCraftAfterTimeCreatesItemOnceForDuplicateCompletion", "craft repair completes through normal craft completion and ledger-backed output services")),
		notApplicable("admin.repair_craft_job broadcasts in-memory realtime events after the service mutation when wired; durable outbox is not part of this browser runtime slice"),
	),
	realtimeCommandSecurityProfile("admin.economy_dashboard",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "admin.economy_dashboard rejects non-admin sessions")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase08MarketAuctionPremiumUseServerEconomyState", "admin.economy_dashboard resolves admin role from the authenticated server session")),
		notApplicable("admin.economy_dashboard has no client-authored amount"),
		notApplicable("admin.economy_dashboard has no hidden world target interaction"),
		notApplicable("admin.economy_dashboard is an aggregate read model without item/currency mutation ledger"),
		notApplicable("admin.economy_dashboard is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("admin.content.list",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList", "admin.content.list rejects non-admin sessions and only accepts content list filters")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList", "admin.content.list resolves admin role from the authenticated server session")),
		notApplicable("admin.content.list has no client-authored amount"),
		notApplicable("admin.content.list has no hidden world target interaction"),
		notApplicable("admin.content.list is a CMS metadata read model without item/currency mutation ledger"),
		notApplicable("admin.content.list is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("admin.content.get",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList", "admin.content.get rejects non-admin sessions and only accepts content row identity")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList", "admin.content.get resolves admin role from the authenticated server session")),
		notApplicable("admin.content.get has no client-authored amount"),
		notApplicable("admin.content.get has no hidden world target interaction"),
		notApplicable("admin.content.get is a CMS metadata read model without item/currency mutation ledger"),
		notApplicable("admin.content.get is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("admin.content.update_draft",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentUpdateDraftAndValidateUseServerActor", "admin.content.update_draft rejects non-admin sessions and server-owned actor fields while allowing CMS stat JSON")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentUpdateDraftAndValidateUseServerActor", "admin.content.update_draft resolves updated_by from the authenticated admin account")),
		satisfied(evidence("gameproject/internal/game/admin", "TestContentServiceUpdateDraftRowRejectsInvalidContentJSON", "draft update validates content JSON and rejects forbidden script/eval/formula fields before write")),
		notApplicable("admin.content.update_draft has no hidden world target interaction"),
		notApplicable("admin.content.update_draft writes draft CMS metadata only; gameplay inventory/wallet mutations are not involved"),
		notApplicable("admin.content.update_draft is a draft-row upsert; publish/rollback idempotency is tracked by the later content version workflow"),
	),
	realtimeCommandSecurityProfile("admin.content.validate_draft",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentUpdateDraftAndValidateUseServerActor", "admin.content.validate_draft rejects non-admin sessions and returns a safe validation report")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentUpdateDraftAndValidateUseServerActor", "admin.content.validate_draft resolves admin role from the authenticated server session")),
		satisfied(evidence("gameproject/internal/game/admin", "TestContentServiceValidateDraftBuildsSnapshotAndReportsRuntimeErrors", "draft validation runs the assembled draft snapshot through runtime catalog validation and reports invalid balance/schema rows without mutating")),
		notApplicable("admin.content.validate_draft has no hidden world target interaction"),
		notApplicable("admin.content.validate_draft is a read-only CMS validation command without item/currency mutation ledger"),
		notApplicable("admin.content.validate_draft is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("admin.content.publish",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentPublishRollbackAndAuditUseServerActor", "admin.content.publish rejects non-admin sessions and client-authored actor/published fields")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentPublishRollbackAndAuditUseServerActor", "admin.content.publish resolves published_by from the authenticated admin account")),
		satisfied(evidence("gameproject/internal/game/admin", "TestContentServicePublishDraftValidatesAndWritesImmutableVersion", "publish validates the assembled draft snapshot before creating an immutable published version")),
		notApplicable("admin.content.publish has no hidden world target interaction"),
		notApplicable("admin.content.publish changes CMS definitions only; gameplay inventory/wallet mutations are not involved"),
		satisfied(evidence("gameproject/internal/game/admin", "TestContentServicePublishDraftValidatesAndWritesImmutableVersion", "publish sends a deterministic content_publish idempotency key and expected-current guard to the DB-backed publisher")),
	),
	realtimeCommandSecurityProfile("admin.content.rollback",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentPublishRollbackAndAuditUseServerActor", "admin.content.rollback rejects non-admin sessions and client-authored actor/published fields")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentPublishRollbackAndAuditUseServerActor", "admin.content.rollback resolves published_by from the authenticated admin account")),
		satisfied(evidence("gameproject/internal/game/admin", "TestContentServiceRollbackPublishesTargetSnapshotAsNewVersion", "rollback validates the target snapshot and publishes a new immutable version")),
		notApplicable("admin.content.rollback has no hidden world target interaction"),
		notApplicable("admin.content.rollback changes CMS definitions only; gameplay inventory/wallet mutations are not involved"),
		satisfied(evidence("gameproject/internal/game/admin", "TestContentServiceRollbackPublishesTargetSnapshotAsNewVersion", "rollback sends content_rollback idempotency, expected-current guard, and rolled_back_from to the DB-backed publisher")),
	),
	realtimeCommandSecurityProfile("admin.content.audit_log",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentPublishRollbackAndAuditUseServerActor", "admin.content.audit_log rejects non-admin sessions and trusted payload fields")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentPublishRollbackAndAuditUseServerActor", "admin.content.audit_log resolves admin role from the authenticated server session")),
		notApplicable("admin.content.audit_log has no client-authored amount"),
		notApplicable("admin.content.audit_log has no hidden world target interaction"),
		notApplicable("admin.content.audit_log is a read-only CMS audit query without item/currency mutation ledger"),
		notApplicable("admin.content.audit_log is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("admin.content.versions",
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList", "admin.content.versions rejects non-admin sessions and client-authored actor fields")),
		satisfied(evidence("gameproject/internal/game/server", "TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList", "admin.content.versions resolves admin role from the authenticated server session")),
		notApplicable("admin.content.versions has no client-authored amount"),
		notApplicable("admin.content.versions has no hidden world target interaction"),
		notApplicable("admin.content.versions is a CMS metadata read model without item/currency mutation ledger"),
		notApplicable("admin.content.versions is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("observability.command_log",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.command_log rejects non-admin sessions and redacts session/player identities")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.command_log resolves admin role from the authenticated server session")),
		notApplicable("observability.command_log has no client-authored amount"),
		notApplicable("observability.command_log has no hidden world target interaction"),
		notApplicable("observability.command_log is read-only without item/currency mutation ledger"),
		notApplicable("observability.command_log is read-only without commit/broadcast semantics"),
	),
	realtimeCommandSecurityProfile("observability.metrics",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.metrics rejects non-admin sessions and returns server recorder snapshots")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.metrics resolves admin role from the authenticated server session")),
		notApplicable("observability.metrics has no client-authored amount"),
		notApplicable("observability.metrics has no hidden world target interaction"),
		notApplicable("observability.metrics is read-only without item/currency mutation ledger"),
		notApplicable("observability.metrics is read-only except for a local metric-updated event broadcast"),
	),
	realtimeCommandSecurityProfile("observability.release_gate",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.release_gate rejects non-admin sessions and returns release gate evidence from server code")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.release_gate resolves admin role from the authenticated server session")),
		notApplicable("observability.release_gate has no client-authored amount"),
		notApplicable("observability.release_gate has no hidden world target interaction"),
		notApplicable("observability.release_gate is read-only without item/currency mutation ledger"),
		notApplicable("observability.release_gate is read-only except for a local release-gate event broadcast"),
	),
	realtimeCommandSecurityProfile("observability.abuse_coverage",
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.abuse_coverage rejects non-admin sessions and returns abuse coverage evidence from server code")),
		satisfied(evidence("gameproject/internal/game/server", "TestPhase09QuestAdminObservabilityUseServerState", "observability.abuse_coverage resolves admin role from the authenticated server session")),
		notApplicable("observability.abuse_coverage has no client-authored amount"),
		notApplicable("observability.abuse_coverage has no hidden world target interaction"),
		notApplicable("observability.abuse_coverage is read-only without item/currency mutation ledger"),
		notApplicable("observability.abuse_coverage is read-only without commit/broadcast semantics"),
	),
}

// RequiredReleaseGateModules returns the Phase 12 module checklist in stable order.
func RequiredReleaseGateModules() []string {
	return cloneStrings(requiredReleaseGateModules)
}

// Phase12ReleaseGateCoverage returns deterministic module-by-module release coverage.
func Phase12ReleaseGateCoverage() []ReleaseGateCoverage {
	coverage := make([]ReleaseGateCoverage, 0, len(phase12ReleaseModuleProfiles)*len(requiredReleaseGateChecks))
	for _, profile := range phase12ReleaseModuleProfiles {
		for _, check := range requiredReleaseGateChecks {
			source := profile.Checks[check]
			coverage = append(coverage, ReleaseGateCoverage{
				Module:   profile.Module,
				Check:    check,
				Status:   source.Status,
				Evidence: cloneGateEvidence(source.Evidence),
				Note:     source.Note,
			})
		}
	}
	return coverage
}

// NewReleaseGateCoverageReport fails closed unless every module/check pair is satisfied or not applicable.
func NewReleaseGateCoverageReport(coverage []ReleaseGateCoverage) ReleaseGateCoverageReport {
	covered := make(map[string]map[ReleaseGateCheck]GateStatus, len(requiredReleaseGateModules))
	for _, item := range coverage {
		if covered[item.Module] == nil {
			covered[item.Module] = make(map[ReleaseGateCheck]GateStatus)
		}
		covered[item.Module][item.Check] = item.Status
	}

	missing := make([]ReleaseGateCoverageMissing, 0)
	for _, module := range requiredReleaseGateModules {
		for _, check := range requiredReleaseGateChecks {
			if !gateStatusPasses(covered[module][check]) {
				missing = append(missing, ReleaseGateCoverageMissing{Module: module, Check: check})
			}
		}
	}
	return ReleaseGateCoverageReport{
		Covered: len(missing) == 0,
		Passed:  len(missing) == 0,
		Missing: cloneReleaseGateCoverageMissing(missing),
	}
}

// RequiredCommandSecurityOperations returns the covered command names in stable order.
func RequiredCommandSecurityOperations() []string {
	return cloneStrings(requiredCommandSecurityOperations)
}

// Phase12CommandSecurityCoverage returns deterministic command security coverage.
func Phase12CommandSecurityCoverage() []CommandSecurityCoverage {
	coverage := make([]CommandSecurityCoverage, 0, len(phase12CommandSecurityProfiles)*len(requiredCommandSecurityChecks))
	for _, profile := range phase12CommandSecurityProfiles {
		for _, check := range requiredCommandSecurityChecks {
			source := profile.Checks[check]
			coverage = append(coverage, CommandSecurityCoverage{
				Command:  profile.Command,
				Check:    check,
				Status:   source.Status,
				Evidence: cloneGateEvidence(source.Evidence),
				Note:     source.Note,
			})
		}
	}
	return coverage
}

// NewCommandSecurityCoverageReport fails closed unless every command/check pair is satisfied or not applicable.
func NewCommandSecurityCoverageReport(coverage []CommandSecurityCoverage) CommandSecurityCoverageReport {
	covered := make(map[string]map[CommandSecurityCheck]GateStatus, len(requiredCommandSecurityOperations))
	for _, item := range coverage {
		if covered[item.Command] == nil {
			covered[item.Command] = make(map[CommandSecurityCheck]GateStatus)
		}
		covered[item.Command][item.Check] = item.Status
	}

	missing := make([]CommandSecurityCoverageMissing, 0)
	for _, command := range requiredCommandSecurityOperations {
		for _, check := range requiredCommandSecurityChecks {
			if !gateStatusPasses(covered[command][check]) {
				missing = append(missing, CommandSecurityCoverageMissing{Command: command, Check: check})
			}
		}
	}
	return CommandSecurityCoverageReport{
		Covered: len(missing) == 0,
		Passed:  len(missing) == 0,
		Missing: cloneCommandSecurityCoverageMissing(missing),
	}
}

func releaseModuleProfileFor(module string, unitTest GateEvidence, integration, abuse, admin, ledger gateCoverageSource) releaseModuleProfile {
	return releaseModuleProfile{
		Module: module,
		Checks: map[ReleaseGateCheck]gateCoverageSource{
			ReleaseGateUnitTests:                   satisfied(unitTest),
			ReleaseGateIntegrationTransactionTests: integration,
			ReleaseGateAbuseTests:                  abuse,
			ReleaseGateMetrics:                     satisfied(evidence("gameproject/internal/game/observability", "TestMetricHelpersRecordPhase12Series", "Phase 12 metrics include gameplay and economy series for release dashboards")),
			ReleaseGateAdminInspection:             admin,
			ReleaseGateErrorCodes:                  satisfied(evidence("gameproject/internal/game/foundation", "TestDomainErrorPublicSerializationOmitsInternalDetails", "public errors serialize with stable codes and without internal details")),
			ReleaseGateLedgerReason:                ledger,
			ReleaseGateLoadTest:                    satisfied(phase12LoadTestEvidence),
			ReleaseGateGoTestAll:                   satisfied(phase12GoTestAllEvidence),
			ReleaseGateGitDiffCheck:                satisfied(phase12GitDiffCheckEvidence),
		},
	}
}

func realtimeCommandSecurityProfile(command string, intent, ownership, amount, visibility, ledger, broadcast gateCoverageSource) commandSecurityProfile {
	return commandSecurityProfile{
		Command: command,
		Checks: map[CommandSecurityCheck]gateCoverageSource{
			CommandSecurityIntentOnlyPayload:      intent,
			CommandSecurityServerPlayerSession:    satisfied(evidence("gameproject/internal/game/realtime", "TestObservedCommandExecutorRequiresServerResolvedIdentity", "observed command executor requires server-resolved session and player identity")),
			CommandSecurityOwnershipChecked:       ownership,
			CommandSecurityPositiveBoundedAmounts: amount,
			CommandSecurityVisibilityRangeChecked: visibility,
			CommandSecurityTransactionLock:        satisfied(evidence("gameproject/internal/game/world/worker", "TestCommandDrainOrderIsDeterministic", "zone worker is the single owner that drains intent commands deterministically")),
			CommandSecurityLedgerWrite:            ledger,
			CommandSecurityIdempotency:            satisfied(evidence("gameproject/internal/game/realtime", "TestRequestCacheCoordinatesInFlightDuplicateRequestID", "request cache coordinates in-flight duplicate request IDs")),
			CommandSecurityLeakSafeError:          satisfied(evidence("gameproject/internal/game/realtime", "TestObservedCommandExecutorRecordsErrorCodeMetricWithoutLeakingDetails", "observed command errors record safe codes without leaking details")),
			CommandSecurityBroadcastAfterCommit:   broadcast,
		},
	}
}

func satisfied(evidence ...GateEvidence) gateCoverageSource {
	return gateCoverageSource{
		Status:   GateStatusSatisfied,
		Evidence: cloneGateEvidence(evidence),
	}
}

func notApplicable(note string) gateCoverageSource {
	return gateCoverageSource{
		Status: GateStatusNotApplicable,
		Note:   note,
	}
}

func evidence(packagePath, testName, note string) GateEvidence {
	return GateEvidence{
		Package:  packagePath,
		TestName: testName,
		Note:     note,
	}
}

func document(path, note string) GateEvidence {
	return GateEvidence{
		Document: path,
		Note:     note,
	}
}

func gateStatusPasses(status GateStatus) bool {
	return status == GateStatusSatisfied || status == GateStatusNotApplicable
}

func cloneGateEvidence(evidence []GateEvidence) []GateEvidence {
	if len(evidence) == 0 {
		return nil
	}
	cloned := make([]GateEvidence, len(evidence))
	copy(cloned, evidence)
	return cloned
}

func cloneReleaseGateCoverageMissing(missing []ReleaseGateCoverageMissing) []ReleaseGateCoverageMissing {
	if len(missing) == 0 {
		return nil
	}
	cloned := make([]ReleaseGateCoverageMissing, len(missing))
	copy(cloned, missing)
	return cloned
}

func cloneCommandSecurityCoverageMissing(missing []CommandSecurityCoverageMissing) []CommandSecurityCoverageMissing {
	if len(missing) == 0 {
		return nil
	}
	cloned := make([]CommandSecurityCoverageMissing, len(missing))
	copy(cloned, missing)
	return cloned
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

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
	"move_to",
	"stop",
	"debug_spawn_npc",
	"debug_snapshot",
}

var phase12LoadTestEvidence = GateEvidence{
	Package:  "gameproject/internal/game/observability",
	TestName: "TestPhase12LoadTestTargetsCoverExpectedThroughput",
	Note:     "local load target coverage records the Phase 12 expected player, visibility, snapshot, tick, and metric envelope",
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
		evidence("gameproject/internal/game/market", "TestCreateListingRejectsDuplicateListingIDWithoutMutation", "market listing unit coverage exists"),
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

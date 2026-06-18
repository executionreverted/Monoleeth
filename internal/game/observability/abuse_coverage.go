package observability

// AbuseTestCase is a stable Phase 12 abuse-test checklist item.
type AbuseTestCase string

const (
	AbuseTestNegativeAmounts                AbuseTestCase = "negative_amounts"
	AbuseTestEnormousAmounts                AbuseTestCase = "enormous_amounts"
	AbuseTestDuplicateRequestID             AbuseTestCase = "duplicate_request_id"
	AbuseTestSameCommandDifferentRequestIDs AbuseTestCase = "same_command_different_request_ids"
	AbuseTestHiddenEntityInteraction        AbuseTestCase = "hidden_entity_interaction"
	AbuseTestOutOfRangePickup               AbuseTestCase = "out_of_range_pickup"
	AbuseTestMarketBuyCancelRace            AbuseTestCase = "market_buy_cancel_race"
	AbuseTestAuctionBidBuyNowRace           AbuseTestCase = "auction_bid_buy_now_race"
	AbuseTestPremiumWebhookReplay           AbuseTestCase = "premium_webhook_replay"
	AbuseTestOfflineSettlementRepeated      AbuseTestCase = "offline_settlement_repeated"
	AbuseTestRouteToggleAroundSettlement    AbuseTestCase = "route_toggle_around_settlement"
	AbuseTestLockedSkillUnlock              AbuseTestCase = "locked_skill_unlock"
	AbuseTestBrokenModuleStillActive        AbuseTestCase = "broken_module_still_active"
)

// AbuseTestEvidence points at executable Go test coverage for one abuse case.
type AbuseTestEvidence struct {
	Package  string `json:"package"`
	TestName string `json:"test_name"`
	Note     string `json:"note"`
}

// AbuseTestCoverage records the evidence backing one abuse checklist item.
type AbuseTestCoverage struct {
	Case     AbuseTestCase       `json:"case"`
	Evidence []AbuseTestEvidence `json:"evidence,omitempty"`
}

// AbuseTestCoverageReport reports whether every required abuse case has evidence.
type AbuseTestCoverageReport struct {
	Passed  bool            `json:"passed"`
	Missing []AbuseTestCase `json:"missing,omitempty"`
}

var requiredAbuseTestCases = []AbuseTestCase{
	AbuseTestNegativeAmounts,
	AbuseTestEnormousAmounts,
	AbuseTestDuplicateRequestID,
	AbuseTestSameCommandDifferentRequestIDs,
	AbuseTestHiddenEntityInteraction,
	AbuseTestOutOfRangePickup,
	AbuseTestMarketBuyCancelRace,
	AbuseTestAuctionBidBuyNowRace,
	AbuseTestPremiumWebhookReplay,
	AbuseTestOfflineSettlementRepeated,
	AbuseTestRouteToggleAroundSettlement,
	AbuseTestLockedSkillUnlock,
	AbuseTestBrokenModuleStillActive,
}

var phase12AbuseTestCoverage = []AbuseTestCoverage{
	{
		Case: AbuseTestNegativeAmounts,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/foundation",
				TestName: "TestValidatePositiveAmountRejectsZeroAndNegativeValues",
				Note:     "shared amount primitive rejects zero and negative value inputs",
			},
			{
				Package:  "gameproject/internal/game/economy",
				TestName: "TestPhase02SafetyReserveItemsRejectsNegativeQuantityWithoutLedger",
				Note:     "reservation write path rejects negative quantity before ledger mutation",
			},
		},
	},
	{
		Case: AbuseTestEnormousAmounts,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/foundation",
				TestName: "TestPositiveAmountTypesRejectValuesAboveSafetyBound",
				Note:     "shared amount primitive rejects values above the configured safety bound",
			},
			{
				Package:  "gameproject/internal/game/economy",
				TestName: "TestPhase02OverflowRejectedByCreditWalletAndTransferCurrency",
				Note:     "wallet credit and transfer reject balance overflow without ledger mutation",
			},
		},
	},
	{
		Case: AbuseTestDuplicateRequestID,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/realtime",
				TestName: "TestRequestCacheDuplicateRequestIDReturnsCachedResponse",
				Note:     "realtime request cache returns the original response for duplicate request IDs",
			},
		},
	},
	{
		Case: AbuseTestSameCommandDifferentRequestIDs,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/premium",
				TestName: "TestClaimRejectsWrongPlayerAndDifferentRequestAfterClaim",
				Note:     "same entitlement command with a different request reference is rejected after claim",
			},
		},
	},
	{
		Case: AbuseTestHiddenEntityInteraction,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/combat",
				TestName: "TestExecuteBasicAttackRejectsHiddenTarget",
				Note:     "authoritative combat rejects hidden target interactions at attack time",
			},
			{
				Package:  "gameproject/internal/game/world/visibility",
				TestName: "TestCanInteractRejectsHiddenEntityWithGenericError",
				Note:     "visibility primitive rejects hidden interactions without leaking hidden details",
			},
		},
	},
	{
		Case: AbuseTestOutOfRangePickup,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/loot",
				TestName: "TestPickupDropRejectsFarHiddenAndCargoFullWithoutClaim",
				Note:     "loot pickup rejects far drops before claiming or moving cargo",
			},
		},
	},
	{
		Case: AbuseTestMarketBuyCancelRace,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/observability/simulations",
				TestName: "TestMarketBuyCancelRaceSimulationConservesItems",
				Note:     "market buy/cancel race simulation conserves item and currency balances",
			},
			{
				Package:  "gameproject/internal/game/market",
				TestName: "TestBuyRacingCancelCannotDuplicateItems",
				Note:     "authoritative market service allows only one terminal buy/cancel path",
			},
		},
	},
	{
		Case: AbuseTestAuctionBidBuyNowRace,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/observability/simulations",
				TestName: "TestAuctionBidBuyNowRaceSimulationClosesOnce",
				Note:     "auction bid/buy-now race simulation closes each lot once with one grant",
			},
			{
				Package:  "gameproject/internal/game/auction",
				TestName: "TestBidRacingBuyNowCannotCreateTwoWinners",
				Note:     "authoritative auction service prevents concurrent bid and buy-now double wins",
			},
		},
	},
	{
		Case: AbuseTestPremiumWebhookReplay,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/premium",
				TestName: "TestCreateEntitlementStoresPendingAndProviderReplayReturnsOriginal",
				Note:     "provider event replay returns original entitlement by provider reference",
			},
			{
				Package:  "gameproject/internal/game/premium",
				TestName: "TestApplyProviderRiskLockRevokesEntitlementAndReplaysByReference",
				Note:     "provider risk lock replay is duplicate-safe by provider reference",
			},
		},
	},
	{
		Case: AbuseTestOfflineSettlementRepeated,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/observability/simulations",
				TestName: "TestPlanetSettlementSimulationTracksOfflineProductionAndDuplicateNoOps",
				Note:     "offline planet settlement simulation retries the same timestamp and expects no duplicate production",
			},
		},
	},
	{
		Case: AbuseTestRouteToggleAroundSettlement,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/production",
				TestName: "TestDisableRouteSettlesOldRouteBeforeDisabling",
				Note:     "disabling a route settles the enabled period before turning the route off",
			},
			{
				Package:  "gameproject/internal/game/production",
				TestName: "TestEnableRouteResetsLastCalculatedAtSoDisabledElapsedDoesNotTransfer",
				Note:     "enabling a route resets the settlement clock so disabled time cannot transfer value",
			},
		},
	},
	{
		Case: AbuseTestLockedSkillUnlock,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/progression",
				TestName: "TestUnlockPilotSkillValidatesLockedNodesAndConsumesPointOnce",
				Note:     "skill unlock rejects locked prerequisites and duplicate unlocks without extra spend",
			},
			{
				Package:  "gameproject/internal/game/progression",
				TestName: "TestUnlockPilotSkillChecksRankAndRoleRequirements",
				Note:     "skill unlock rejects unmet rank and role requirements",
			},
		},
	},
	{
		Case: AbuseTestBrokenModuleStillActive,
		Evidence: []AbuseTestEvidence{
			{
				Package:  "gameproject/internal/game/runtime",
				TestName: "TestStatInputProviderIgnoresBrokenEquippedModules",
				Note:     "runtime stat provider excludes broken equipped modules from effective inputs",
			},
			{
				Package:  "gameproject/internal/game/stats",
				TestName: "TestStatServiceExcludesBrokenModuleModifiers",
				Note:     "stat aggregation ignores broken module modifiers before producing snapshots",
			},
		},
	},
}

// RequiredAbuseTestCases returns the Phase 12 abuse checklist in stable order.
func RequiredAbuseTestCases() []AbuseTestCase {
	return cloneAbuseTestCases(requiredAbuseTestCases)
}

// Phase12AbuseTestCoverage returns deterministic evidence for current abuse coverage.
func Phase12AbuseTestCoverage() []AbuseTestCoverage {
	coverage := make([]AbuseTestCoverage, len(phase12AbuseTestCoverage))
	for i, item := range phase12AbuseTestCoverage {
		coverage[i] = cloneAbuseTestCoverage(item)
	}
	return coverage
}

// NewAbuseTestCoverageReport fails closed unless every required case has evidence.
func NewAbuseTestCoverageReport(coverage []AbuseTestCoverage) AbuseTestCoverageReport {
	covered := make(map[AbuseTestCase]bool, len(coverage))
	for _, item := range coverage {
		if len(item.Evidence) > 0 {
			covered[item.Case] = true
		}
	}

	missing := make([]AbuseTestCase, 0)
	for _, required := range requiredAbuseTestCases {
		if !covered[required] {
			missing = append(missing, required)
		}
	}
	return AbuseTestCoverageReport{
		Passed:  len(missing) == 0,
		Missing: cloneAbuseTestCases(missing),
	}
}

func cloneAbuseTestCoverage(item AbuseTestCoverage) AbuseTestCoverage {
	cloned := item
	if len(item.Evidence) > 0 {
		cloned.Evidence = make([]AbuseTestEvidence, len(item.Evidence))
		copy(cloned.Evidence, item.Evidence)
	}
	return cloned
}

func cloneAbuseTestCases(cases []AbuseTestCase) []AbuseTestCase {
	if len(cases) == 0 {
		return nil
	}
	cloned := make([]AbuseTestCase, len(cases))
	copy(cloned, cases)
	return cloned
}

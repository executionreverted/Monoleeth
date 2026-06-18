package quests

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/progression"
)

func TestMVPQuestCatalogCoversAllMVPQuestTypes(t *testing.T) {
	questCatalog := MustMVPQuestCatalog()
	seen := make(map[QuestType]bool)
	for _, template := range questCatalog.Templates() {
		seen[template.Type] = true
	}

	for _, questType := range []QuestType{
		QuestTypeKill,
		QuestTypeCollect,
		QuestTypeCraft,
		QuestTypeScan,
		QuestTypeBuild,
		QuestTypeDeliver,
	} {
		if !seen[questType] {
			t.Fatalf("MVP catalog missing quest type %q", questType)
		}
	}
}

func TestMVPQuestCatalogExcludesMarketQuestTypes(t *testing.T) {
	deniedMarketQuestTypes := []string{
		"market_sell",
		"market_buy",
		"market_trade",
		"auction_sell",
		"auction_buy",
		"auction_bid",
	}

	for _, questType := range deniedMarketQuestTypes {
		if err := QuestType(questType).Validate(); !errors.Is(err, ErrInvalidQuestType) {
			t.Fatalf("QuestType(%q).Validate() error = %v, want ErrInvalidQuestType", questType, err)
		}
	}

	for _, objectiveKind := range deniedMarketQuestTypes {
		if err := ObjectiveKind(objectiveKind).Validate(); !errors.Is(err, ErrInvalidObjectiveKind) {
			t.Fatalf("ObjectiveKind(%q).Validate() error = %v, want ErrInvalidObjectiveKind", objectiveKind, err)
		}
	}

	questCatalog := MustMVPQuestCatalog()
	for _, template := range questCatalog.Templates() {
		for _, value := range []string{
			template.TemplateID.String(),
			string(template.Type),
			template.TitleKey,
			template.DescriptionKey,
		} {
			lower := strings.ToLower(value)
			if strings.Contains(lower, "market") || strings.Contains(lower, "auction") {
				t.Fatalf("MVP catalog template %q contains market/auction reference %q", template.TemplateID, value)
			}
		}
	}
}

func TestGenerateBoardReturnsExactlyTenOffers(t *testing.T) {
	input := validBoardGenerationInput(t, MustMVPQuestCatalog())

	offers, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}
	if len(offers) != BoardOfferCount {
		t.Fatalf("GenerateBoard() returned %d offers, want %d", len(offers), BoardOfferCount)
	}

	seenOfferIDs := make(map[string]bool, len(offers))
	for _, offer := range offers {
		if err := offer.Validate(); err != nil {
			t.Fatalf("offer Validate() = %v, want nil", err)
		}
		if seenOfferIDs[offer.OfferID.String()] {
			t.Fatalf("duplicate offer id %q", offer.OfferID)
		}
		seenOfferIDs[offer.OfferID.String()] = true
	}
}

func TestGenerateBoardDeterministicForSameInput(t *testing.T) {
	input := validBoardGenerationInput(t, MustMVPQuestCatalog())

	first, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard(first) = %v, want nil", err)
	}
	second, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard(second) = %v, want nil", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("GenerateBoard() mismatch for same input\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestGenerateBoardFiltersRankAndRequirementsBeforeSelection(t *testing.T) {
	input := validBoardGenerationInput(t, MustMVPQuestCatalog())
	input.WeightHook = func(_ PlayerQuestBoardSnapshot, template QuestTemplate) int {
		switch template.TemplateID {
		case "quest_craft_laser_alpha_r2", "quest_kill_void_raiders_r3":
			return 1 << 30
		default:
			return 1
		}
	}

	offers, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}
	for _, offer := range offers {
		switch offer.TemplateID {
		case "quest_craft_laser_alpha_r2", "quest_kill_void_raiders_r3":
			t.Fatalf("generated unavailable template %q", offer.TemplateID)
		}
	}
}

func TestGenerateBoardStoresRewardPayloadAtOfferTime(t *testing.T) {
	offers, err := GenerateBoard(validBoardGenerationInput(t, MustMVPQuestCatalog()))
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}

	for _, offer := range offers {
		if len(offer.RewardPayload.Grants) == 0 {
			t.Fatalf("offer %q has no generated reward grants", offer.OfferID)
		}
		if err := offer.RewardPayload.Validate(); err != nil {
			t.Fatalf("offer %q reward payload Validate() = %v, want nil", offer.OfferID, err)
		}
	}
}

func TestGenerateBoardRareRewardCapHookCanBlockExcessiveRareOffers(t *testing.T) {
	blockedTemplateID := catalog.DefinitionID("quest_kill_pirates_r1")
	input := validBoardGenerationInput(t, MustMVPQuestCatalog())
	input.WeightHook = func(_ PlayerQuestBoardSnapshot, template QuestTemplate) int {
		if template.TemplateID == blockedTemplateID {
			return 1 << 30
		}
		return 1
	}
	blockedChecks := 0
	input.RareRewardCapHook = func(check RareRewardCapCheck) (bool, error) {
		if len(check.Hooks) == 0 {
			t.Fatalf("cap check for %q had no hooks", check.Template.TemplateID)
		}
		if check.Template.TemplateID == blockedTemplateID {
			blockedChecks++
			return false, nil
		}
		return true, nil
	}

	offers, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}
	if blockedChecks == 0 {
		t.Fatal("rare cap hook was not called for the blocked template")
	}
	if len(offers) != BoardOfferCount {
		t.Fatalf("offers = %d, want %d", len(offers), BoardOfferCount)
	}
	for _, offer := range offers {
		if offer.TemplateID == blockedTemplateID {
			t.Fatalf("blocked rare reward template %q was still offered", blockedTemplateID)
		}
	}
}

func TestGenerateBoardPayloadIncludesObjectiveAndDailyExpiry(t *testing.T) {
	questCatalog := MustMVPQuestCatalog()
	input := validBoardGenerationInput(t, questCatalog)

	offers, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}

	for _, offer := range offers {
		template, ok := questCatalog.Lookup(offer.TemplateID)
		if !ok {
			t.Fatalf("template %q not found", offer.TemplateID)
		}
		if offer.GeneratedPayload.Objective.IsZero() {
			t.Fatalf("offer %q generated payload missing objective", offer.OfferID)
		}
		if !reflect.DeepEqual(offer.GeneratedPayload.Objective, template.ObjectiveSchema) {
			t.Fatalf("offer %q objective = %#v, want %#v", offer.OfferID, offer.GeneratedPayload.Objective, template.ObjectiveSchema)
		}
		if !offer.CreatedAt.Equal(input.CreatedAt.UTC()) {
			t.Fatalf("offer %q created_at = %s, want %s", offer.OfferID, offer.CreatedAt, input.CreatedAt.UTC())
		}
		if !offer.ExpiresAt.After(offer.CreatedAt) {
			t.Fatalf("offer %q expires_at %s is not after created_at %s", offer.OfferID, offer.ExpiresAt, offer.CreatedAt)
		}
		if !offer.ExpiresAt.Equal(NextQuestBoardExpiry(input.CreatedAt)) {
			t.Fatalf("offer %q expires_at = %s, want %s", offer.OfferID, offer.ExpiresAt, NextQuestBoardExpiry(input.CreatedAt))
		}
	}
}

func TestGenerateBoardOffersCarryDirectionalTargetsForPlayer(t *testing.T) {
	input := validBoardGenerationInput(t, MustMVPQuestCatalog())
	input.Player.CurrentRegion = "frontier-east"

	offers, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}

	for _, offer := range offers {
		if offer.GeneratedPayload.TargetRegion != input.Player.CurrentRegion {
			t.Fatalf("offer %q target region = %q, want %q", offer.OfferID, offer.GeneratedPayload.TargetRegion, input.Player.CurrentRegion)
		}
		if offer.GeneratedPayload.Difficulty < input.Player.Rank {
			t.Fatalf("offer %q difficulty = %d, want at least player rank %d", offer.OfferID, offer.GeneratedPayload.Difficulty, input.Player.Rank)
		}
		assertDirectionalObjective(t, offer)
	}
}

func TestGenerateBoardInsufficientEligibleTemplatesReturnsClearError(t *testing.T) {
	templates := MVPQuestTemplates()[:BoardOfferCount-1]
	questCatalog, err := NewQuestCatalog(templates)
	if err != nil {
		t.Fatalf("NewQuestCatalog() = %v, want nil", err)
	}

	offers, err := GenerateBoard(validBoardGenerationInput(t, questCatalog))
	if !errors.Is(err, ErrInsufficientEligibleTemplates) {
		t.Fatalf("GenerateBoard() error = %v, want ErrInsufficientEligibleTemplates", err)
	}
	if offers != nil {
		t.Fatalf("GenerateBoard() offers = %#v, want nil", offers)
	}
}

func TestDefaultQuestWeightHookDeterministicallyNudgesPlayerNeeds(t *testing.T) {
	questCatalog := MustMVPQuestCatalog()
	scanTemplate := mustLookupQuestTemplate(t, questCatalog, "quest_scan_signals_r1")
	killTemplate := mustLookupQuestTemplate(t, questCatalog, "quest_kill_pirates_r1")
	snapshot := validBoardGenerationInput(t, questCatalog).Player
	snapshot.RoleLevels[progression.RoleTypeScout] = 1
	snapshot.RoleLevels[progression.RoleTypeCombat] = 5
	snapshot.RecentActivity = QuestRecentActivity{Kill: 12}

	firstScanWeight := DefaultQuestWeightHook(snapshot, scanTemplate)
	secondScanWeight := DefaultQuestWeightHook(snapshot, scanTemplate)
	killWeight := DefaultQuestWeightHook(snapshot, killTemplate)

	if firstScanWeight != secondScanWeight {
		t.Fatalf("scan weight changed between calls: %d then %d", firstScanWeight, secondScanWeight)
	}
	if firstScanWeight <= killWeight {
		t.Fatalf("scan weight = %d, want greater than kill weight %d for low-scout combat-heavy snapshot", firstScanWeight, killWeight)
	}
}

func validBoardGenerationInput(t *testing.T, questCatalog QuestCatalog) BoardGenerationInput {
	t.Helper()
	return BoardGenerationInput{
		Player: PlayerQuestBoardSnapshot{
			PlayerID:  "player_1",
			Rank:      1,
			MainLevel: 1,
			RoleLevels: map[progression.RoleType]int{
				progression.RoleTypeCombat:       1,
				progression.RoleTypeScout:        1,
				progression.RoleTypeCrafting:     1,
				progression.RoleTypeConstruction: 1,
			},
			CurrentRegion:    "frontier",
			KnownPlanetCount: 0,
			OwnedPlanetCount: 0,
		},
		Seed:      20260617,
		Catalog:   questCatalog,
		CreatedAt: time.Date(2026, 6, 17, 10, 30, 0, 0, time.UTC),
	}
}

func mustLookupQuestTemplate(t *testing.T, questCatalog QuestCatalog, templateID catalog.DefinitionID) QuestTemplate {
	t.Helper()
	template, ok := questCatalog.Lookup(templateID)
	if !ok {
		t.Fatalf("Lookup(%q) not found", templateID)
	}
	return template
}

func assertDirectionalObjective(t *testing.T, offer GeneratedBoardOffer) {
	t.Helper()
	objectives := offer.GeneratedPayload.Objective.Objectives
	if len(objectives) == 0 {
		t.Fatalf("offer %q has no directional objective rows", offer.OfferID)
	}
	for _, objective := range objectives {
		switch objective.Kind {
		case ObjectiveKindKill:
			if objective.Kill == nil || objective.Kill.TargetNPCType == "" || objective.Kill.RequiredCount.Int64() <= 0 {
				t.Fatalf("offer %q kill objective = %+v, want target and count", offer.OfferID, objective.Kill)
			}
		case ObjectiveKindCollect:
			if objective.Collect == nil || objective.Collect.ItemID.IsZero() || objective.Collect.Quantity.Int64() <= 0 {
				t.Fatalf("offer %q collect objective = %+v, want item and quantity", offer.OfferID, objective.Collect)
			}
		case ObjectiveKindCraft:
			if objective.Craft == nil || (objective.Craft.RecipeID.IsZero() && objective.Craft.ItemID.IsZero()) || objective.Craft.Quantity.Int64() <= 0 {
				t.Fatalf("offer %q craft objective = %+v, want recipe/item and quantity", offer.OfferID, objective.Craft)
			}
		case ObjectiveKindScan:
			if objective.Scan == nil || objective.Scan.TargetSignalType == "" || objective.Scan.RequiredCount.Int64() <= 0 {
				t.Fatalf("offer %q scan objective = %+v, want signal target and count", offer.OfferID, objective.Scan)
			}
		case ObjectiveKindBuild:
			if objective.Build == nil || objective.Build.BuildingType == "" || objective.Build.RequiredCount.Int64() <= 0 {
				t.Fatalf("offer %q build objective = %+v, want building and count", offer.OfferID, objective.Build)
			}
		case ObjectiveKindDeliver:
			if objective.Deliver == nil ||
				objective.Deliver.ItemID.IsZero() ||
				objective.Deliver.Quantity.Int64() <= 0 ||
				objective.Deliver.DestinationType == "" ||
				objective.Deliver.DestinationID == "" {
				t.Fatalf("offer %q deliver objective = %+v, want item, quantity, and destination", offer.OfferID, objective.Deliver)
			}
		default:
			t.Fatalf("offer %q unsupported objective kind %q", offer.OfferID, objective.Kind)
		}
	}
}

package quests

import (
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/testutil"
)

const compatQuestTemplateID catalog.DefinitionID = "quest_compat_collect_r1"

// compatCatalog builds a board-ready catalog (>= BoardOfferCount templates) that
// contains one deterministic collect quest with the given version and required
// count, padded with the MVP starter templates so board generation succeeds.
func compatCatalog(t *testing.T, version catalog.Version, requiredCount int64) QuestCatalog {
	t.Helper()
	source, err := catalog.NewQuestSource(string(compatQuestTemplateID), version.String())
	if err != nil {
		t.Fatalf("NewQuestSource() = %v", err)
	}
	target := QuestTemplate{
		Source:          source,
		TemplateID:      compatQuestTemplateID,
		Type:            QuestTypeCollect,
		TitleKey:        "quest.compat_collect.title",
		DescriptionKey:  "quest.compat_collect.description",
		ObjectiveSchema: collectObjective("collect_1", "carbon_shards", requiredCount),
	}
	templates := append([]QuestTemplate(nil), MVPQuestTemplates()[:BoardOfferCount-1]...)
	templates = append(templates, target)
	c, err := NewQuestCatalog(templates)
	if err != nil {
		t.Fatalf("NewQuestCatalog() = %v", err)
	}
	return c
}

// TestAcceptedQuestSurvivesContentRepublish proves the P09 lane-F accepted-old-
// quest compatibility property: once a player accepts a quest, the accepted quest
// runs off its accept-time objective/reward snapshot. A later content republish
// (new catalog version + changed objective) does NOT mutate or invalidate the
// in-flight quest, while new board offers pick up the fresh content.
func TestAcceptedQuestSurvivesContentRepublish(t *testing.T) {
	now := time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)
	v1Catalog := compatCatalog(t, "quest_compat_v1", 18)
	clock := testutil.NewFakeClock(now)
	store := NewInMemoryQuestStore()
	v1Service, err := NewQuestService(clock, v1Catalog, store)
	if err != nil {
		t.Fatalf("NewQuestService(v1) = %v", err)
	}

	input := validBoardGenerationInput(t, v1Catalog)
	offers, err := v1Service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard(v1) = %v", err)
	}
	offer := mustFindOffer(t, offers, string(compatQuestTemplateID))
	accepted, err := v1Service.AcceptQuest(AcceptQuestInput{Player: input.Player, OfferID: offer.OfferID})
	if err != nil {
		t.Fatalf("AcceptQuest(v1) = %v", err)
	}
	frozenObjective := accepted.GeneratedPayload.Objective
	frozenVersion := accepted.TemplateSource.Version
	if err := accepted.Progress.ValidateAgainst(frozenObjective); err != nil {
		t.Fatalf("v1 progress ValidateAgainst() = %v, want nil", err)
	}

	// Simulate a CMS republish: same template id, new catalog version, and a
	// changed objective (doubled required count). A fresh service now owns the
	// republished catalog but reuses the same store holding the accepted quest.
	v2Catalog := compatCatalog(t, "quest_compat_v2", 99)
	v2Service, err := NewQuestService(clock, v2Catalog, store)
	if err != nil {
		t.Fatalf("NewQuestService(v2) = %v", err)
	}

	republishedTemplate, ok := v2Catalog.Lookup(compatQuestTemplateID)
	if !ok {
		t.Fatalf("v2 catalog missing template %q", compatQuestTemplateID)
	}
	if reflect.DeepEqual(republishedTemplate.ObjectiveSchema, frozenObjective) {
		t.Fatalf("v2 objective equals frozen v1 objective; test setup must change the objective")
	}

	// New board offers generated after the republish must carry the v2 objective,
	// proving the running service serves fresh content while accepted quests stay frozen.
	v2Offers, err := v2Service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard(v2) = %v", err)
	}
	v2Offer := mustFindOffer(t, v2Offers, string(compatQuestTemplateID))
	if !reflect.DeepEqual(v2Offer.GeneratedPayload.Objective, republishedTemplate.ObjectiveSchema) {
		t.Fatalf("v2 offer objective = %+v, want republished v2 objective", v2Offer.GeneratedPayload.Objective)
	}

	storedQuests, err := store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v", err)
	}
	if len(storedQuests) != 1 {
		t.Fatalf("stored quests = %d, want 1 accepted quest preserved", len(storedQuests))
	}
	stored := storedQuests[0]
	if stored.TemplateSource.Version != frozenVersion {
		t.Fatalf("accepted quest version = %q, want frozen %q after republish", stored.TemplateSource.Version, frozenVersion)
	}
	if !reflect.DeepEqual(stored.GeneratedPayload.Objective, frozenObjective) {
		t.Fatalf("accepted quest objective changed after republish; accepted quests must be frozen at accept time")
	}
	if err := stored.Progress.ValidateAgainst(stored.GeneratedPayload.Objective); err != nil {
		t.Fatalf("accepted quest progress no longer validates against its frozen objective: %v", err)
	}
}

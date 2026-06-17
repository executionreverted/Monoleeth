package quests

import (
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

func TestConsumeCombatNPCKilledProgressesOnlyMatchingActiveQuest(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_kill_pirates_consumer", QuestTypeKill, killObjective("kill_pirate", "pirate", 2)),
		consumerTemplate("quest_kill_raiders_consumer", QuestTypeKill, killObjective("kill_raider", "raider", 2)),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	pirateQuest := acceptConsumerQuest(t, fixture, player, "quest_kill_pirates_consumer", "kill_pirates")
	raiderQuest := acceptConsumerQuest(t, fixture, player, "quest_kill_raiders_consumer", "kill_raiders")

	fixture.clock.Advance(time.Minute)
	updated, err := fixture.service.ConsumeCombatNPCKilled(CombatNPCKilledInput{
		EventID:  "event_kill_pirate_1",
		PlayerID: player.PlayerID,
		NPCType:  "pirate",
	})
	if err != nil {
		t.Fatalf("ConsumeCombatNPCKilled() = %v, want nil", err)
	}
	if len(updated) != 1 || updated[0].PlayerQuestID != pirateQuest.PlayerQuestID {
		t.Fatalf("updated quests = %#v, want only %q", updated, pirateQuest.PlayerQuestID)
	}

	assertStoredQuestProgress(t, fixture, player.PlayerID, pirateQuest.PlayerQuestID, QuestStateAccepted, 1, false)
	assertStoredQuestProgress(t, fixture, player.PlayerID, raiderQuest.PlayerQuestID, QuestStateAccepted, 0, false)
}

func TestConsumeLootPickedUpProgressesOnlyMatchingActiveQuest(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_collect_iron_consumer", QuestTypeCollect, collectObjective("collect_iron", "iron_ore", 5)),
		consumerTemplate("quest_collect_carbon_consumer", QuestTypeCollect, collectObjective("collect_carbon", "carbon_shards", 5)),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	ironQuest := acceptConsumerQuest(t, fixture, player, "quest_collect_iron_consumer", "collect_iron")
	carbonQuest := acceptConsumerQuest(t, fixture, player, "quest_collect_carbon_consumer", "collect_carbon")

	fixture.clock.Advance(time.Minute)
	updated, err := fixture.service.ConsumeLootPickedUp(LootPickedUpInput{
		EventID:  "event_loot_iron_1",
		PlayerID: player.PlayerID,
		ItemID:   "iron_ore",
		Quantity: qty(t, 2),
	})
	if err != nil {
		t.Fatalf("ConsumeLootPickedUp() = %v, want nil", err)
	}
	if len(updated) != 1 || updated[0].PlayerQuestID != ironQuest.PlayerQuestID {
		t.Fatalf("updated quests = %#v, want only %q", updated, ironQuest.PlayerQuestID)
	}

	assertStoredQuestProgress(t, fixture, player.PlayerID, ironQuest.PlayerQuestID, QuestStateAccepted, 2, false)
	assertStoredQuestProgress(t, fixture, player.PlayerID, carbonQuest.PlayerQuestID, QuestStateAccepted, 0, false)
}

func TestConsumeCraftJobCompletedProgressesOnlyMatchingActiveQuest(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_craft_energy_consumer", QuestTypeCraft, craftObjective("craft_energy", "energy_cell_batch", "", 2)),
		consumerTemplate("quest_craft_alloy_consumer", QuestTypeCraft, craftObjective("craft_alloy", "refined_alloy_batch", "", 2)),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	energyQuest := acceptConsumerQuest(t, fixture, player, "quest_craft_energy_consumer", "craft_energy")
	alloyQuest := acceptConsumerQuest(t, fixture, player, "quest_craft_alloy_consumer", "craft_alloy")

	fixture.clock.Advance(time.Minute)
	updated, err := fixture.service.ConsumeCraftJobCompleted(CraftJobCompletedInput{
		EventID:  "event_craft_energy_1",
		PlayerID: player.PlayerID,
		RecipeID: "energy_cell_batch",
		Quantity: qty(t, 1),
	})
	if err != nil {
		t.Fatalf("ConsumeCraftJobCompleted() = %v, want nil", err)
	}
	if len(updated) != 1 || updated[0].PlayerQuestID != energyQuest.PlayerQuestID {
		t.Fatalf("updated quests = %#v, want only %q", updated, energyQuest.PlayerQuestID)
	}

	assertStoredQuestProgress(t, fixture, player.PlayerID, energyQuest.PlayerQuestID, QuestStateAccepted, 1, false)
	assertStoredQuestProgress(t, fixture, player.PlayerID, alloyQuest.PlayerQuestID, QuestStateAccepted, 0, false)
}

func TestConsumeNonMatchingEventDoesNotMutateQuest(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_kill_pirates_consumer", QuestTypeKill, killObjective("kill_pirate", "pirate", 2)),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	quest := acceptConsumerQuest(t, fixture, player, "quest_kill_pirates_consumer", "kill_pirates")

	fixture.clock.Advance(time.Minute)
	updated, err := fixture.service.ConsumeCombatNPCKilled(CombatNPCKilledInput{
		EventID:  "event_kill_raider_1",
		PlayerID: player.PlayerID,
		NPCType:  "raider",
	})
	if err != nil {
		t.Fatalf("ConsumeCombatNPCKilled() = %v, want nil", err)
	}
	if len(updated) != 0 {
		t.Fatalf("updated quests len = %d, want 0", len(updated))
	}
	assertStoredQuestProgress(t, fixture, player.PlayerID, quest.PlayerQuestID, QuestStateAccepted, 0, false)
}

func TestConsumeDuplicateServerEventDoesNotProgressTwice(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_kill_pirates_consumer", QuestTypeKill, killObjective("kill_pirate", "pirate", 2)),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	quest := acceptConsumerQuest(t, fixture, player, "quest_kill_pirates_consumer", "kill_pirates")

	input := CombatNPCKilledInput{
		EventID:  "event_kill_duplicate",
		PlayerID: player.PlayerID,
		NPCType:  "pirate",
	}
	fixture.clock.Advance(time.Minute)
	if _, err := fixture.service.ConsumeCombatNPCKilled(input); err != nil {
		t.Fatalf("ConsumeCombatNPCKilled(first) = %v, want nil", err)
	}
	fixture.clock.Advance(time.Minute)
	updated, err := fixture.service.ConsumeCombatNPCKilled(input)
	if err != nil {
		t.Fatalf("ConsumeCombatNPCKilled(duplicate) = %v, want nil", err)
	}
	if len(updated) != 0 {
		t.Fatalf("duplicate updated quests len = %d, want 0", len(updated))
	}
	assertStoredQuestProgress(t, fixture, player.PlayerID, quest.PlayerQuestID, QuestStateAccepted, 1, false)
}

func TestConsumeCompletedQuestDoesNotOverflowOrProgressFurther(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_collect_iron_consumer", QuestTypeCollect, collectObjective("collect_iron", "iron_ore", 5)),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	quest := acceptConsumerQuest(t, fixture, player, "quest_collect_iron_consumer", "collect_iron")

	fixture.clock.Advance(time.Minute)
	updated, err := fixture.service.ConsumeLootPickedUp(LootPickedUpInput{
		EventID:  "event_loot_iron_overflow",
		PlayerID: player.PlayerID,
		ItemID:   "iron_ore",
		Quantity: qty(t, 99),
	})
	if err != nil {
		t.Fatalf("ConsumeLootPickedUp(overflow) = %v, want nil", err)
	}
	if len(updated) != 1 || updated[0].State != QuestStateCompleted {
		t.Fatalf("updated quests = %#v, want one completed quest", updated)
	}
	assertStoredQuestProgress(t, fixture, player.PlayerID, quest.PlayerQuestID, QuestStateCompleted, 5, true)

	fixture.clock.Advance(time.Minute)
	updated, err = fixture.service.ConsumeLootPickedUp(LootPickedUpInput{
		EventID:  "event_loot_iron_after_completed",
		PlayerID: player.PlayerID,
		ItemID:   "iron_ore",
		Quantity: qty(t, 99),
	})
	if err != nil {
		t.Fatalf("ConsumeLootPickedUp(after completed) = %v, want nil", err)
	}
	if len(updated) != 0 {
		t.Fatalf("after completed updated quests len = %d, want 0", len(updated))
	}
	assertStoredQuestProgress(t, fixture, player.PlayerID, quest.PlayerQuestID, QuestStateCompleted, 5, true)
}

func TestSkeletonConsumersValidateAndNoopSafely(t *testing.T) {
	fixture := newConsumerQuestFixture(t,
		consumerTemplate("quest_scan_consumer", QuestTypeScan, scanObjective("scan_planet", ScanTargetPlanet, 1)),
		consumerTemplate("quest_build_consumer", QuestTypeBuild, buildObjective("build_extractor", "extractor_t1", 1)),
		consumerTemplate("quest_deliver_consumer", QuestTypeDeliver, deliverObjective("deliver_iron", "iron_ore", 5, DeliveryTargetStation, "station_frontier")),
	)
	player := validBoardGenerationInput(t, fixture.catalog).Player
	scanQuest := acceptConsumerQuest(t, fixture, player, "quest_scan_consumer", "scan")
	buildQuest := acceptConsumerQuest(t, fixture, player, "quest_build_consumer", "build")
	deliverQuest := acceptConsumerQuest(t, fixture, player, "quest_deliver_consumer", "deliver")

	validConsumers := []struct {
		name    string
		consume func() ([]PlayerQuest, error)
	}{
		{
			name: "scan",
			consume: func() ([]PlayerQuest, error) {
				return fixture.service.ConsumeScanCompleted(ScanCompletedInput{
					EventID:          "event_scan_1",
					PlayerID:         player.PlayerID,
					TargetSignalType: "planet",
				})
			},
		},
		{
			name: "build",
			consume: func() ([]PlayerQuest, error) {
				return fixture.service.ConsumeBuildingCompleted(BuildingCompletedInput{
					EventID:      "event_build_1",
					PlayerID:     player.PlayerID,
					BuildingType: "extractor_t1",
				})
			},
		},
		{
			name: "deliver",
			consume: func() ([]PlayerQuest, error) {
				return fixture.service.ConsumeDeliveryCompleted(DeliveryCompletedInput{
					EventID:         "event_deliver_1",
					PlayerID:        player.PlayerID,
					ItemID:          "iron_ore",
					Quantity:        qty(t, 5),
					DestinationType: "station",
					DestinationID:   "station_frontier",
				})
			},
		},
	}
	for _, test := range validConsumers {
		updated, err := test.consume()
		if err != nil {
			t.Fatalf("%s skeleton consumer = %v, want nil", test.name, err)
		}
		if len(updated) != 0 {
			t.Fatalf("%s skeleton updated quests len = %d, want 0", test.name, len(updated))
		}
	}

	invalidConsumers := []struct {
		name    string
		consume func() ([]PlayerQuest, error)
	}{
		{
			name: "scan",
			consume: func() ([]PlayerQuest, error) {
				return fixture.service.ConsumeScanCompleted(ScanCompletedInput{
					EventID:  "event_scan_invalid",
					PlayerID: player.PlayerID,
				})
			},
		},
		{
			name: "build",
			consume: func() ([]PlayerQuest, error) {
				return fixture.service.ConsumeBuildingCompleted(BuildingCompletedInput{
					EventID:  "event_build_invalid",
					PlayerID: player.PlayerID,
				})
			},
		},
		{
			name: "deliver",
			consume: func() ([]PlayerQuest, error) {
				return fixture.service.ConsumeDeliveryCompleted(DeliveryCompletedInput{
					EventID:         "event_deliver_invalid",
					PlayerID:        player.PlayerID,
					ItemID:          "iron_ore",
					DestinationType: "station",
				})
			},
		},
	}
	for _, test := range invalidConsumers {
		if _, err := test.consume(); err == nil {
			t.Fatalf("%s invalid skeleton consumer error = nil, want error", test.name)
		}
	}

	assertStoredQuestProgress(t, fixture, player.PlayerID, scanQuest.PlayerQuestID, QuestStateAccepted, 0, false)
	assertStoredQuestProgress(t, fixture, player.PlayerID, buildQuest.PlayerQuestID, QuestStateAccepted, 0, false)
	assertStoredQuestProgress(t, fixture, player.PlayerID, deliverQuest.PlayerQuestID, QuestStateAccepted, 0, false)
}

func newConsumerQuestFixture(t *testing.T, templates ...QuestTemplate) questServiceFixture {
	t.Helper()
	questCatalog, err := NewQuestCatalog(templates)
	if err != nil {
		t.Fatalf("NewQuestCatalog() = %v, want nil", err)
	}
	return newQuestServiceFixture(t, questCatalog, time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC))
}

func consumerTemplate(templateID catalog.DefinitionID, questType QuestType, objective ObjectiveSchema) QuestTemplate {
	return newMVPQuestTemplate(
		templateID,
		questType,
		templateID.String()+".title",
		templateID.String()+".description",
		objective,
		nil,
	)
}

func acceptConsumerQuest(
	t *testing.T,
	fixture questServiceFixture,
	player PlayerQuestBoardSnapshot,
	templateID catalog.DefinitionID,
	offerSuffix string,
) PlayerQuest {
	t.Helper()
	template := mustLookupQuestTemplate(t, fixture.catalog, templateID)
	createdAt := fixture.clock.Now().UTC().Add(-time.Minute)
	expiresAt := fixture.clock.Now().UTC().Add(time.Hour)
	offer, err := NewGeneratedBoardOffer(
		foundation.QuestID("offer_"+offerSuffix),
		player.PlayerID,
		template,
		GeneratedPayload{Seed: 17},
		validRewardPayload(),
		createdAt,
		expiresAt,
	)
	if err != nil {
		t.Fatalf("NewGeneratedBoardOffer() = %v, want nil", err)
	}
	if err := fixture.service.StoreGeneratedBoardOffers([]GeneratedBoardOffer{offer}); err != nil {
		t.Fatalf("StoreGeneratedBoardOffers() = %v, want nil", err)
	}
	quest, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  player,
		OfferID: offer.OfferID,
	})
	if err != nil {
		t.Fatalf("AcceptQuest() = %v, want nil", err)
	}
	return quest
}

func assertStoredQuestProgress(
	t *testing.T,
	fixture questServiceFixture,
	playerID foundation.PlayerID,
	questID foundation.QuestID,
	wantState QuestState,
	wantCurrent int64,
	wantCompleted bool,
) {
	t.Helper()
	quest := storedQuestByID(t, fixture, playerID, questID)
	if quest.State != wantState {
		t.Fatalf("quest %q state = %q, want %q", questID, quest.State, wantState)
	}
	if len(quest.Progress.Objectives) != 1 {
		t.Fatalf("quest %q progress objectives len = %d, want 1", questID, len(quest.Progress.Objectives))
	}
	progress := quest.Progress.Objectives[0]
	if progress.Current != wantCurrent || progress.Completed != wantCompleted {
		t.Fatalf("quest %q progress = current %d completed %t, want current %d completed %t",
			questID, progress.Current, progress.Completed, wantCurrent, wantCompleted)
	}
	if wantState == QuestStateCompleted && quest.CompletedAt == nil {
		t.Fatalf("quest %q completed_at = nil, want timestamp", questID)
	}
}

func storedQuestByID(
	t *testing.T,
	fixture questServiceFixture,
	playerID foundation.PlayerID,
	questID foundation.QuestID,
) PlayerQuest {
	t.Helper()
	quests, err := fixture.store.PlayerQuests(playerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	for _, quest := range quests {
		if quest.PlayerQuestID == questID {
			return quest
		}
	}
	t.Fatalf("quest %q not found", questID)
	return PlayerQuest{}
}

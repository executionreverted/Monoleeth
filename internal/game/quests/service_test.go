package quests

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/progression"
	"gameproject/internal/game/testutil"
)

func TestAcceptQuestSucceedsFromGeneratedOffer(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	quest, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if err != nil {
		t.Fatalf("AcceptQuest() = %v, want nil", err)
	}

	if quest.PlayerID != input.Player.PlayerID {
		t.Fatalf("accepted quest player = %q, want %q", quest.PlayerID, input.Player.PlayerID)
	}
	if quest.State != QuestStateAccepted {
		t.Fatalf("accepted quest state = %q, want %q", quest.State, QuestStateAccepted)
	}
	if quest.ExpiresAt != nil {
		t.Fatalf("accepted quest expires_at = %s, want nil for MVP", *quest.ExpiresAt)
	}
	if !reflect.DeepEqual(quest.GeneratedPayload, offers[0].GeneratedPayload) {
		t.Fatalf("accepted generated payload = %#v, want stored offer payload %#v", quest.GeneratedPayload, offers[0].GeneratedPayload)
	}
	if !reflect.DeepEqual(quest.RewardPayload, offers[0].RewardPayload) {
		t.Fatalf("accepted reward payload = %#v, want stored offer reward %#v", quest.RewardPayload, offers[0].RewardPayload)
	}
	if err := quest.Progress.ValidateAgainst(offers[0].GeneratedPayload.Objective); err != nil {
		t.Fatalf("accepted quest progress ValidateAgainst() = %v, want nil", err)
	}
	if len(quest.Progress.Objectives) == 0 {
		t.Fatal("accepted quest progress has no objectives")
	}
	for _, objective := range quest.Progress.Objectives {
		if objective.Current != 0 || objective.Completed {
			t.Fatalf("objective progress = current %d completed %t, want zero incomplete", objective.Current, objective.Completed)
		}
	}

	playerQuests, err := fixture.store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	if len(playerQuests) != 1 {
		t.Fatalf("stored player quests len = %d, want 1", len(playerQuests))
	}
	availableOffers, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers() = %v, want nil", err)
	}
	if len(availableOffers) != len(offers)-1 {
		t.Fatalf("available offers len = %d, want %d after accept", len(availableOffers), len(offers)-1)
	}
}

func TestAcceptQuestRejectsExpiredOffer(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() = %v, want nil", err)
	}
	if err := fixture.service.StoreGeneratedBoardOffers(offers); err != nil {
		t.Fatalf("StoreGeneratedBoardOffers() = %v, want nil", err)
	}

	_, err = fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if !errors.Is(err, ErrQuestOfferExpired) {
		t.Fatalf("AcceptQuest() error = %v, want ErrQuestOfferExpired", err)
	}
	stored, err := fixture.store.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers() = %v, want nil", err)
	}
	if len(stored) != len(offers)-1 {
		t.Fatalf("stored offers after expired accept = %d, want %d", len(stored), len(offers)-1)
	}
}

func TestBoardOffersExpiresOldUnacceptedOffers(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 23, 50, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	before, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers(before) = %v, want nil", err)
	}
	if len(before) != len(offers) {
		t.Fatalf("board offers before expiry = %d, want %d", len(before), len(offers))
	}

	fixture.clock.Advance(NextQuestBoardExpiry(input.CreatedAt).Sub(fixture.clock.Now()))
	after, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers(after) = %v, want nil", err)
	}
	if len(after) != 0 {
		t.Fatalf("board offers after expiry = %d, want 0", len(after))
	}

	stored, err := fixture.store.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("raw BoardOffers() = %v, want nil", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored unaccepted offers after expiry prune = %d, want 0", len(stored))
	}
}

func TestBoardOfferExpiryPreservesAcceptedQuest(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 23, 50, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	accepted, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if err != nil {
		t.Fatalf("AcceptQuest() = %v, want nil", err)
	}

	fixture.clock.Advance(NextQuestBoardExpiry(input.CreatedAt).Sub(fixture.clock.Now()))
	available, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers() = %v, want nil", err)
	}
	if len(available) != 0 {
		t.Fatalf("available offers after expiry = %d, want 0", len(available))
	}

	playerQuests, err := fixture.store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	if len(playerQuests) != 1 || playerQuests[0].PlayerQuestID != accepted.PlayerQuestID {
		t.Fatalf("player quests after offer expiry = %#v, want accepted quest %q", playerQuests, accepted.PlayerQuestID)
	}

	duplicate, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if err != nil {
		t.Fatalf("duplicate AcceptQuest() after expiry = %v, want nil", err)
	}
	if duplicate.PlayerQuestID != accepted.PlayerQuestID {
		t.Fatalf("duplicate quest id = %q, want %q", duplicate.PlayerQuestID, accepted.PlayerQuestID)
	}
}

func TestAcceptQuestRejectsMaxActiveQuestOverflow(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	for index := 0; index < MaxActivePlayerQuests; index++ {
		if _, err := fixture.service.AcceptQuest(AcceptQuestInput{
			Player:  input.Player,
			OfferID: offers[index].OfferID,
		}); err != nil {
			t.Fatalf("AcceptQuest(%d) = %v, want nil", index, err)
		}
	}

	_, err = fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[MaxActivePlayerQuests].OfferID,
	})
	if !errors.Is(err, ErrTooManyActiveQuests) {
		t.Fatalf("AcceptQuest() error = %v, want ErrTooManyActiveQuests", err)
	}
	playerQuests, err := fixture.store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	if len(playerQuests) != MaxActivePlayerQuests {
		t.Fatalf("player quests len = %d, want %d", len(playerQuests), MaxActivePlayerQuests)
	}
}

func TestAcceptQuestRejectsWrongPlayerOffer(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	otherPlayer := input.Player
	otherPlayer.PlayerID = "player_2"
	_, err = fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  otherPlayer,
		OfferID: offers[0].OfferID,
	})
	if !errors.Is(err, ErrQuestOfferOwnerMismatch) {
		t.Fatalf("AcceptQuest() error = %v, want ErrQuestOfferOwnerMismatch", err)
	}
}

func TestAcceptQuestDuplicateAcceptReturnsExistingQuest(t *testing.T) {
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	first, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if err != nil {
		t.Fatalf("AcceptQuest(first) = %v, want nil", err)
	}
	fixture.clock.Advance(time.Minute)
	second, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if err != nil {
		t.Fatalf("AcceptQuest(second) = %v, want nil", err)
	}

	if second.PlayerQuestID != first.PlayerQuestID {
		t.Fatalf("duplicate accept quest id = %q, want %q", second.PlayerQuestID, first.PlayerQuestID)
	}
	if !second.AcceptedAt.Equal(first.AcceptedAt) {
		t.Fatalf("duplicate accept accepted_at = %s, want %s", second.AcceptedAt, first.AcceptedAt)
	}
	playerQuests, err := fixture.store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	if len(playerQuests) != 1 {
		t.Fatalf("player quests len after duplicate accept = %d, want 1", len(playerQuests))
	}
}

func TestAcceptQuestRechecksRequirementsAtAcceptTime(t *testing.T) {
	questCatalog := questCatalogWithRequiredTemplate(t)
	fixture := newQuestServiceFixture(t, questCatalog, time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC))
	input := validBoardGenerationInput(t, questCatalog)
	input.Player.Rank = 2
	input.Player.MainLevel = 2
	input.Player.RoleLevels[progression.RoleTypeCrafting] = 2

	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	requiredOffer := mustFindOffer(t, offers, "quest_craft_laser_alpha_r2")

	downgradedPlayer := input.Player
	downgradedPlayer.Rank = 1
	downgradedPlayer.RoleLevels = map[progression.RoleType]int{
		progression.RoleTypeCombat:       1,
		progression.RoleTypeScout:        1,
		progression.RoleTypeCrafting:     1,
		progression.RoleTypeConstruction: 1,
	}
	_, err = fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  downgradedPlayer,
		OfferID: requiredOffer.OfferID,
	})
	if !errors.Is(err, ErrQuestRequirementsNotMet) {
		t.Fatalf("AcceptQuest() error = %v, want ErrQuestRequirementsNotMet", err)
	}
	playerQuests, err := fixture.store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	if len(playerQuests) != 0 {
		t.Fatalf("player quests len after failed requirements = %d, want 0", len(playerQuests))
	}
}

type questServiceFixture struct {
	catalog QuestCatalog
	clock   *testutil.FakeClock
	store   *InMemoryQuestStore
	service *QuestService
}

func newQuestServiceFixture(t *testing.T, questCatalog QuestCatalog, now time.Time) questServiceFixture {
	t.Helper()
	clock := testutil.NewFakeClock(now)
	store := NewInMemoryQuestStore()
	service, err := NewQuestService(clock, questCatalog, store)
	if err != nil {
		t.Fatalf("NewQuestService() = %v, want nil", err)
	}
	return questServiceFixture{
		catalog: questCatalog,
		clock:   clock,
		store:   store,
		service: service,
	}
}

func questCatalogWithRequiredTemplate(t *testing.T) QuestCatalog {
	t.Helper()
	templates := append([]QuestTemplate(nil), MVPQuestTemplates()[:BoardOfferCount-1]...)
	templates = append(templates, mustTemplateByID(t, MVPQuestTemplates(), "quest_craft_laser_alpha_r2"))
	questCatalog, err := NewQuestCatalog(templates)
	if err != nil {
		t.Fatalf("NewQuestCatalog() = %v, want nil", err)
	}
	return questCatalog
}

func mustTemplateByID(t *testing.T, templates []QuestTemplate, templateID string) QuestTemplate {
	t.Helper()
	for _, template := range templates {
		if template.TemplateID.String() == templateID {
			return template
		}
	}
	t.Fatalf("template %q not found", templateID)
	return QuestTemplate{}
}

func mustFindOffer(t *testing.T, offers []GeneratedBoardOffer, templateID string) GeneratedBoardOffer {
	t.Helper()
	for _, offer := range offers {
		if offer.TemplateID.String() == templateID {
			return offer
		}
	}
	t.Fatalf("offer for template %q not found", templateID)
	return GeneratedBoardOffer{}
}

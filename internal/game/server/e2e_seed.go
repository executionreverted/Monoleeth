package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world"
)

const (
	runtimeE2EPlanetClaimSeedLedgerReason economy.LedgerReason = "e2e_planet_claim_seed"
	runtimePlaytestSeedLedgerReason       economy.LedgerReason = "playtest_seed"
	e2ePlanetClaimSeedReason                                   = "e2e_planet_claim_seed"
	playtestSeedReason                                         = "playtest_seed"
	e2eRouteSeedSourceStorageUnits                             = gamecontent.DefaultRouteSeedStorageUnits
	e2eRouteSeedEnergyPerHour                                  = gamecontent.DefaultRouteSeedEnergyPerHour
)

func (runtime *Runtime) seedE2EPlanetClaimProof(playerID foundation.PlayerID) error {
	seedPrefix, ok := runtime.claimSeedPrefix()
	if !ok {
		return nil
	}
	if err := runtime.seedE2EPlanetClaimXCore(playerID, seedPrefix); err != nil {
		return err
	}
	return runtime.seedE2EPlanetClaimProgression(playerID, seedPrefix)
}

func (runtime *Runtime) seedE2EPlanetClaimXCore(playerID foundation.PlayerID, seedPrefix string) error {
	definition, ok := runtime.itemCatalog[runtime.starterContent.ClaimSeed.CoreItemID]
	if !ok {
		return fmt.Errorf("%s definition missing", runtime.starterContent.ClaimSeed.CoreItemID)
	}
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return err
	}
	referenceKey, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), seedPrefix+"-planet-claim-x-core")
	if err != nil {
		return err
	}
	_, err = runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       int64(runtime.e2ePlanetClaimCoreQuantity()),
		Location:       location,
		Reason:         runtime.claimSeedLedgerReason(seedPrefix),
		ReferenceKey:   referenceKey,
	})
	return err
}

func (runtime *Runtime) seedE2EPlanetClaimProgression(playerID foundation.PlayerID, seedPrefix string) error {
	mainXP, err := progression.MainXPTable().RequiredXPForLevel(progression.MaxMVPLevel)
	if err != nil {
		return err
	}
	scoutXP, err := progression.RoleXPTable().RequiredXPForLevel(progression.MaxMVPLevel)
	if err != nil {
		return err
	}
	if _, err := runtime.Progression.GrantXP(progression.GrantXPInput{
		PlayerID:       playerID,
		Amount:         mainXP,
		SourceType:     progression.XPSourceTypeAdminAdjustment,
		SourceID:       progression.XPSourceID(seedPrefix + "-planet-claim-progression"),
		IdempotencyKey: progression.XPIdempotencyKey(seedPrefix + "_planet_claim:progression"),
		Authority:      progression.XPGrantAuthorityAdminService,
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeScout, Amount: scoutXP},
		},
	}); err != nil {
		return err
	}

	questSourceID := progression.XPSourceID(seedPrefix + "-planet-claim-quest")
	if _, err := runtime.Progression.GrantXP(progression.GrantXPInput{
		PlayerID:       playerID,
		Amount:         1,
		SourceType:     progression.XPSourceTypeQuest,
		SourceID:       questSourceID,
		IdempotencyKey: progression.XPIdempotencyKey("quest_reward:" + questSourceID.String()),
		Authority:      progression.XPGrantAuthorityQuestService,
	}); err != nil {
		return err
	}

	for rank := progression.MinRank + 1; rank <= progression.MaxMVPRank; rank++ {
		result, err := runtime.Progression.TryRankUp(progression.TryRankUpInput{
			PlayerID:       playerID,
			TargetRank:     rank,
			Reason:         runtime.claimSeedReason(seedPrefix),
			IdempotencyKey: progression.XPIdempotencyKey(fmt.Sprintf("%s_planet_claim:rank:%d", seedPrefix, rank)),
		})
		if err != nil {
			return err
		}
		if !result.RankedUp && !result.Duplicate && !result.AlreadyAtRank {
			return fmt.Errorf("e2e planet claim rank %d missing requirements: %v", rank, result.MissingRequirements)
		}
	}

	snapshot, err := runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		return err
	}
	state := runtime.players[playerID]
	state.Rank = snapshot.Player.Rank
	runtime.players[playerID] = state
	return nil
}

func (runtime *Runtime) seedE2ERouteProof(playerID foundation.PlayerID) error {
	seedPrefix, ok := runtime.routeSeedPrefix()
	if !ok {
		return nil
	}
	sourceID := routeSeedPlanetID(playerID, "source", seedPrefix)
	destinationID := routeSeedPlanetID(playerID, "destination", seedPrefix)
	now := runtime.clock.Now().UTC()
	sourceProductionCreated, err := runtime.seedE2EOwnedRoutePlanet(playerID, sourceID, world.Vec2{X: 2400, Y: 2600}, "source", seedPrefix, now)
	if err != nil {
		return err
	}
	if _, err := runtime.seedE2EOwnedRoutePlanet(playerID, destinationID, world.Vec2{X: 3300, Y: 2850}, "destination", seedPrefix, now); err != nil {
		return err
	}
	if !sourceProductionCreated {
		if _, ok, err := runtime.Production.PlanetStorage(sourceID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	sourceStorage, err := production.NewPlanetStorage(sourceID, runtime.starterContent.RouteSeed.SourceStorageUnits, runtime.starterContent.RouteSeed.SourceStoredItems, now)
	if err != nil {
		return err
	}
	return runtime.Production.SavePlanetStorage(sourceStorage)
}

func (runtime *Runtime) seedE2EOwnedRoutePlanet(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	coordinates world.Vec2,
	label string,
	seedPrefix string,
	now time.Time,
) (bool, error) {
	ownerChangedAt := now
	if _, err := runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: discovery.PlanetMaterializationKey(seedPrefix + "-route-" + label + "-" + routeSeedPlayerSuffix(playerID, seedPrefix)),
		Planet: discovery.Planet{
			ID:             planetID,
			WorldID:        runtime.worldID,
			ZoneID:         runtime.zoneID,
			Coordinates:    coordinates,
			Biome:          discovery.PlanetBiomeOriginBelt,
			Type:           discovery.PlanetTypeTerrestrial,
			Rarity:         discovery.PlanetRarityCommon,
			Level:          1,
			DiscoveredAt:   now,
			DiscoveredBy:   playerID,
			OwnerPlayerID:  playerID,
			OwnerChangedAt: &ownerChangedAt,
		},
	}); err != nil {
		return false, err
	}
	if _, _, err := runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        playerID,
		PlanetID:        planetID,
		WorldID:         runtime.worldID,
		ZoneID:          runtime.zoneID,
		Coordinates:     coordinates,
		State:           discovery.IntelStateVerified,
		Confidence:      100,
		LastSeenAt:      now,
		SourceType:      discovery.IntelSourceAdmin,
		SourceReference: seedPrefix + "-route-" + label,
	}); err != nil {
		return false, err
	}
	result, err := runtime.Production.InitializePlanetProduction(production.InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      now,
		StorageCapacityUnits:  runtime.starterContent.RouteSeed.SourceStorageUnits,
		EnergyCapacityPerHour: runtime.starterContent.RouteSeed.EnergyPerHour,
		UpdatedAt:             now,
	})
	return result.Created, err
}

func e2eRoutePlanetID(playerID foundation.PlayerID, label string) foundation.PlanetID {
	return routeSeedPlanetID(playerID, label, "e2e")
}

func playtestRoutePlanetID(playerID foundation.PlayerID, label string) foundation.PlanetID {
	return routeSeedPlanetID(playerID, label, "playtest")
}

func routeSeedPlanetID(playerID foundation.PlayerID, label string, seedPrefix string) foundation.PlanetID {
	return foundation.PlanetID(fmt.Sprintf("planet-%s-route-%s-%s", seedPrefix, label, routeSeedPlayerSuffix(playerID, seedPrefix)))
}

func routeSeedPlayerSuffix(playerID foundation.PlayerID, seedPrefix string) string {
	sum := sha256.Sum256([]byte(seedPrefix + "-route:" + playerID.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func (runtime *Runtime) claimSeedPrefix() (string, bool) {
	if runtime.playtestSeed {
		return "playtest", true
	}
	if runtime.e2ePlanetClaimSeed {
		return "e2e", true
	}
	return "", false
}

func (runtime *Runtime) routeSeedPrefix() (string, bool) {
	if runtime.playtestSeed {
		return "playtest", true
	}
	if runtime.e2eRouteSeed {
		return "e2e", true
	}
	return "", false
}

func (runtime *Runtime) claimSeedLedgerReason(seedPrefix string) economy.LedgerReason {
	if seedPrefix == "playtest" {
		return runtimePlaytestSeedLedgerReason
	}
	return runtimeE2EPlanetClaimSeedLedgerReason
}

func (runtime *Runtime) claimSeedReason(seedPrefix string) string {
	if seedPrefix == "playtest" {
		return playtestSeedReason
	}
	return e2ePlanetClaimSeedReason
}

func (runtime *Runtime) e2ePlanetClaimCoreQuantity() int {
	if runtime.playtestSeed {
		return runtime.starterContent.ClaimSeed.Quantity
	}
	return e2ePlanetClaimCoreQuantity(runtime.e2ePlanetClaimCores)
}

func e2ePlanetClaimCoreQuantity(quantity int) int {
	if quantity <= 0 {
		return defaultE2EClaimCores
	}
	return quantity
}

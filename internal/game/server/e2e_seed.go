package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world"
)

const (
	runtimeE2EPlanetClaimSeedLedgerReason economy.LedgerReason = "e2e_planet_claim_seed"
	e2ePlanetClaimSeedReason                                   = "e2e_planet_claim_seed"
	e2eRouteSeedSourceStorageUnits                             = 500
	e2eRouteSeedEnergyPerHour                                  = 80
)

func (runtime *Runtime) seedE2EPlanetClaimProof(playerID foundation.PlayerID) error {
	if !runtime.e2ePlanetClaimSeed {
		return nil
	}
	if err := runtime.seedE2EPlanetClaimXCore(playerID); err != nil {
		return err
	}
	return runtime.seedE2EPlanetClaimProgression(playerID)
}

func (runtime *Runtime) seedE2EPlanetClaimXCore(playerID foundation.PlayerID) error {
	definition, ok := runtime.itemCatalog["x_core"]
	if !ok {
		return fmt.Errorf("x_core definition missing")
	}
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return err
	}
	referenceKey, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "e2e-planet-claim-x-core")
	if err != nil {
		return err
	}
	_, err = runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       1,
		Location:       location,
		Reason:         runtimeE2EPlanetClaimSeedLedgerReason,
		ReferenceKey:   referenceKey,
	})
	return err
}

func (runtime *Runtime) seedE2EPlanetClaimProgression(playerID foundation.PlayerID) error {
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
		SourceID:       progression.XPSourceID("e2e-planet-claim-progression"),
		IdempotencyKey: progression.XPIdempotencyKey("e2e_planet_claim:progression"),
		Authority:      progression.XPGrantAuthorityAdminService,
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeScout, Amount: scoutXP},
		},
	}); err != nil {
		return err
	}

	questSourceID := progression.XPSourceID("e2e-planet-claim-quest")
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
			Reason:         e2ePlanetClaimSeedReason,
			IdempotencyKey: progression.XPIdempotencyKey(fmt.Sprintf("e2e_planet_claim:rank:%d", rank)),
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
	if !runtime.e2eRouteSeed {
		return nil
	}
	sourceID := e2eRoutePlanetID(playerID, "source")
	destinationID := e2eRoutePlanetID(playerID, "destination")
	now := runtime.clock.Now().UTC()
	sourceProductionCreated, err := runtime.seedE2EOwnedRoutePlanet(playerID, sourceID, world.Vec2{X: 2400, Y: 2600}, "source", now)
	if err != nil {
		return err
	}
	if _, err := runtime.seedE2EOwnedRoutePlanet(playerID, destinationID, world.Vec2{X: 3300, Y: 2850}, "destination", now); err != nil {
		return err
	}
	if !sourceProductionCreated {
		if _, ok, err := runtime.Production.PlanetStorage(sourceID); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	sourceStorage, err := production.NewPlanetStorage(sourceID, e2eRouteSeedSourceStorageUnits, []production.StoredItem{
		{ItemID: "refined_alloy", Quantity: 160},
	}, now)
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
	now time.Time,
) (bool, error) {
	ownerChangedAt := now
	if _, err := runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: discovery.PlanetMaterializationKey("e2e-route-" + label + "-" + e2eRoutePlayerSuffix(playerID)),
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
		SourceReference: "e2e-route-" + label,
	}); err != nil {
		return false, err
	}
	result, err := runtime.Production.InitializePlanetProduction(production.InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      now,
		StorageCapacityUnits:  e2eRouteSeedSourceStorageUnits,
		EnergyCapacityPerHour: e2eRouteSeedEnergyPerHour,
		UpdatedAt:             now,
	})
	return result.Created, err
}

func e2eRoutePlanetID(playerID foundation.PlayerID, label string) foundation.PlanetID {
	return foundation.PlanetID(fmt.Sprintf("planet-e2e-route-%s-%s", label, e2eRoutePlayerSuffix(playerID)))
}

func e2eRoutePlayerSuffix(playerID foundation.PlayerID) string {
	sum := sha256.Sum256([]byte("e2e-route:" + playerID.String()))
	return hex.EncodeToString(sum[:])[:16]
}

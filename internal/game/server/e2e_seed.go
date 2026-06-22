package server

import (
	"fmt"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

const (
	runtimeE2EPlanetClaimSeedLedgerReason economy.LedgerReason = "e2e_planet_claim_seed"
	e2ePlanetClaimSeedReason                                   = "e2e_planet_claim_seed"
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

package content

import (
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/production"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	DefaultStarterShipID              foundation.ShipID = ships.ShipIDStarter
	DefaultStarterShipDisplayName                       = "Sparrow"
	DefaultStarterScannerItemID       foundation.ItemID = "scanner_t1"
	DefaultStarterScannerModuleID     foundation.ItemID = "scanner_t1"
	DefaultStarterScannerScanPower                      = 500
	DefaultStarterScannerScanRadius                     = 2000
	DefaultStarterScannerScanInterval                   = time.Second
	DefaultStarterScannerEnergyCost                     = 8
	DefaultStarterWalletCredits                         = 1200
	DefaultStarterWalletPremiumPaid                     = 300
	DefaultWeeklyXCorePremiumPrice                      = 100
	DefaultWeeklyXCoreStockTotal                        = 5
	DefaultPlaytestClaimCoreItemID    foundation.ItemID = "x_core"
	DefaultPlaytestClaimCoreQuantity                    = 1
	DefaultRouteSeedStorageUnits                        = 500
	DefaultRouteSeedEnergyPerHour                       = 80
)

// StarterContent owns new-player and playtest seed values. Runtime consumes
// this server-only content; clients only see derived server snapshots/events.
type StarterContent struct {
	BalanceProfileID    string
	BalanceProfileNote  string
	ShipID              foundation.ShipID
	ShipDisplayName     string
	WalletCredits       int64
	WalletPremiumPaid   int64
	ModuleItemIDs       []foundation.ItemID
	ScannerItemID       foundation.ItemID
	ScannerModuleID     foundation.ItemID
	ScannerScanPower    float64
	ScannerScanRadius   float64
	ScannerScanInterval time.Duration
	ScannerEnergyCost   int
	WeeklyXCore         WeeklyXCoreContent
	WorldSeeds          []WorldSeedContent
	ClaimSeed           ClaimSeedContent
	RouteSeed           RouteSeedContent
}

type WeeklyXCoreContent struct {
	PremiumPrice int64
	StockTotal   int64
}

type WorldSeedContent struct {
	MapID             worldmaps.MapID
	EnemyPoolID       worldmaps.EnemyPoolID
	EntityIDOverrides []world.EntityID
}

type ClaimSeedContent struct {
	CoreItemID foundation.ItemID
	Quantity   int
}

type RouteSeedContent struct {
	SourceStorageUnits int64
	EnergyPerHour      int64
	SourceStoredItems  []production.StoredItem
}

func DefaultStarterContent() StarterContent {
	return StarterContent{
		BalanceProfileID:   DefaultStarterBalanceProfileID,
		BalanceProfileNote: DefaultStarterBalanceProfileNote,
		ShipID:             DefaultStarterShipID,
		ShipDisplayName:    DefaultStarterShipDisplayName,
		WalletCredits:      DefaultStarterWalletCredits,
		WalletPremiumPaid:  DefaultStarterWalletPremiumPaid,
		ModuleItemIDs: []foundation.ItemID{
			"scanner_t1",
			"laser_alpha_t1",
			"shield_generator_t1",
		},
		ScannerItemID:       DefaultStarterScannerItemID,
		ScannerModuleID:     DefaultStarterScannerModuleID,
		ScannerScanPower:    DefaultStarterScannerScanPower,
		ScannerScanRadius:   DefaultStarterScannerScanRadius,
		ScannerScanInterval: DefaultStarterScannerScanInterval,
		ScannerEnergyCost:   DefaultStarterScannerEnergyCost,
		WeeklyXCore: WeeklyXCoreContent{
			PremiumPrice: DefaultWeeklyXCorePremiumPrice,
			StockTotal:   DefaultWeeklyXCoreStockTotal,
		},
		WorldSeeds: []WorldSeedContent{{
			MapID:             worldmaps.StarterMapID,
			EnemyPoolID:       "starter_training_drone_pool",
			EntityIDOverrides: []world.EntityID{"entity_training_npc"},
		}},
		ClaimSeed: ClaimSeedContent{
			CoreItemID: DefaultPlaytestClaimCoreItemID,
			Quantity:   DefaultPlaytestClaimCoreQuantity,
		},
		RouteSeed: RouteSeedContent{
			SourceStorageUnits: DefaultRouteSeedStorageUnits,
			EnergyPerHour:      DefaultRouteSeedEnergyPerHour,
			SourceStoredItems: []production.StoredItem{
				{ItemID: "refined_alloy", Quantity: 160},
			},
		},
	}
}

func (content StarterContent) Validate(bundle GameplayContent) error {
	if strings.TrimSpace(content.BalanceProfileID) == "" || strings.TrimSpace(content.BalanceProfileNote) == "" {
		return fmt.Errorf("starter balance profile: %w", ErrInvalidStarterContent)
	}
	if strings.TrimSpace(content.ShipDisplayName) == "" {
		return fmt.Errorf("starter ship display name: %w", ErrInvalidStarterContent)
	}
	if _, ok := bundle.Ships.Get(content.ShipID); !ok {
		return fmt.Errorf("starter ship %q: %w", content.ShipID, ErrUnknownContentShip)
	}
	if content.WalletCredits < 0 || content.WalletPremiumPaid < 0 {
		return fmt.Errorf("starter wallet credits=%d premium=%d: %w", content.WalletCredits, content.WalletPremiumPaid, ErrInvalidStarterContent)
	}
	if content.ScannerScanPower <= 0 || content.ScannerScanRadius <= 0 || content.ScannerScanInterval <= 0 || content.ScannerEnergyCost <= 0 {
		return fmt.Errorf("starter scanner power=%v radius=%v interval=%s cost=%d: %w", content.ScannerScanPower, content.ScannerScanRadius, content.ScannerScanInterval, content.ScannerEnergyCost, ErrInvalidStarterContent)
	}
	if content.WeeklyXCore.PremiumPrice <= 0 || content.WeeklyXCore.StockTotal <= 0 {
		return fmt.Errorf("weekly xcore price=%d stock=%d: %w", content.WeeklyXCore.PremiumPrice, content.WeeklyXCore.StockTotal, ErrInvalidStarterContent)
	}
	if content.ClaimSeed.Quantity <= 0 {
		return fmt.Errorf("claim seed quantity=%d: %w", content.ClaimSeed.Quantity, ErrInvalidStarterContent)
	}
	if err := validateKnownItem(bundle, "claim seed core", content.ClaimSeed.CoreItemID); err != nil {
		return err
	}
	if err := validateStarterModules(bundle, content); err != nil {
		return err
	}
	if err := validateWorldSeeds(bundle, content.WorldSeeds); err != nil {
		return err
	}
	return validateRouteSeed(bundle, content.RouteSeed)
}

func validateStarterModules(bundle GameplayContent, content StarterContent) error {
	if len(content.ModuleItemIDs) == 0 {
		return fmt.Errorf("starter modules: %w", ErrInvalidStarterContent)
	}
	seen := make(map[foundation.ItemID]struct{}, len(content.ModuleItemIDs))
	for _, itemID := range content.ModuleItemIDs {
		if _, exists := seen[itemID]; exists {
			return fmt.Errorf("starter module %q duplicate: %w", itemID, ErrInvalidStarterContent)
		}
		seen[itemID] = struct{}{}
		if err := validateKnownItem(bundle, "starter module", itemID); err != nil {
			return err
		}
		if _, ok := bundle.Modules.Lookup(itemID); !ok {
			return fmt.Errorf("starter module %q: %w", itemID, modules.ErrUnknownModuleDefinition)
		}
	}
	for _, itemID := range []foundation.ItemID{content.ScannerItemID, content.ScannerModuleID} {
		if _, ok := seen[itemID]; !ok {
			return fmt.Errorf("starter scanner module %q not granted: %w", itemID, ErrInvalidStarterContent)
		}
	}
	return nil
}

func validateWorldSeeds(bundle GameplayContent, seeds []WorldSeedContent) error {
	for _, seed := range seeds {
		definition, ok := bundle.Maps.Get(seed.MapID)
		if !ok {
			return fmt.Errorf("world seed map %q: %w", seed.MapID, ErrInvalidStarterContent)
		}
		poolExists := false
		for _, pool := range definition.EnemyPools {
			if pool.EnemyPoolID == seed.EnemyPoolID {
				poolExists = true
				break
			}
		}
		if !poolExists {
			return fmt.Errorf("world seed map %q pool %q: %w", seed.MapID, seed.EnemyPoolID, ErrInvalidStarterContent)
		}
		for _, entityID := range seed.EntityIDOverrides {
			if strings.TrimSpace(entityID.String()) == "" {
				return fmt.Errorf("world seed entity id: %w", ErrInvalidStarterContent)
			}
		}
	}
	return nil
}

func validateRouteSeed(bundle GameplayContent, seed RouteSeedContent) error {
	if seed.SourceStorageUnits <= 0 || seed.EnergyPerHour <= 0 {
		return fmt.Errorf("route seed storage=%d energy=%d: %w", seed.SourceStorageUnits, seed.EnergyPerHour, ErrInvalidStarterContent)
	}
	for _, item := range seed.SourceStoredItems {
		if item.Quantity <= 0 {
			return fmt.Errorf("route seed item %q quantity=%d: %w", item.ItemID, item.Quantity, ErrInvalidStarterContent)
		}
		if err := validateKnownItem(bundle, "route seed", item.ItemID); err != nil {
			return err
		}
	}
	return nil
}

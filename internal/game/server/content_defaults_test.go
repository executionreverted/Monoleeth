package server

import (
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

const (
	starterShipID               foundation.ShipID = gamecontent.DefaultStarterShipID
	defaultPlayerSpeed                            = gamecontent.DefaultPlayerSpeed
	defaultRadarRange                             = gamecontent.DefaultRadarRange
	runtimeLootPickupRange                        = gamecontent.DefaultLootPickupRange
	runtimeBasicLaserEnergyCost                   = gamecontent.DefaultBasicLaserEnergyCost
	runtimeBasicLaserCooldownMS                   = gamecontent.DefaultBasicLaserCooldownMS
	starterScannerEnergyCost                      = gamecontent.DefaultStarterScannerEnergyCost
	starterWalletCredits                          = gamecontent.DefaultStarterWalletCredits
	starterWalletPremiumPaid                      = gamecontent.DefaultStarterWalletPremiumPaid
	weeklyXCorePremiumPrice                       = gamecontent.DefaultWeeklyXCorePremiumPrice
	weeklyXCoreStockTotal                         = gamecontent.DefaultWeeklyXCoreStockTotal
	trainingNPCType                               = gamecontent.DefaultTrainingNPCType

	buildingBuildAlloyFoundryCredits      int64 = gamecontent.DefaultBuildingBuildAlloyFoundryCredits
	buildingUpgradeIronExtractorCredits   int64 = gamecontent.DefaultBuildingUpgradeIronExtractorCredits
	runtimeRouteCreateMaxRoutesPerPlayer        = gamecontent.DefaultRouteCreateMaxRoutesPerPlayer
	runtimePlanetClaimRange                     = gamecontent.DefaultPlanetClaimRange
	runtimeClaimProductionStorageCapacity       = gamecontent.DefaultClaimProductionStorageCapacity
	runtimeClaimProductionEnergyCapacity        = gamecontent.DefaultClaimProductionEnergyCapacity
)

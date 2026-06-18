package production

import (
	"fmt"

	"gameproject/internal/game/discovery"
)

// ClaimProductionInitializationDefaults are the explicit production defaults
// applied when discovery reports a newly claimed planet.
type ClaimProductionInitializationDefaults struct {
	StorageCapacityUnits  int64
	EnergyCapacityPerHour int64
}

// ClaimProductionInitializerConfig wires the discovery claim adapter to a
// production store without requiring discovery to import production.
type ClaimProductionInitializerConfig struct {
	Store    *InMemoryStore
	Defaults ClaimProductionInitializationDefaults
}

// ClaimProductionInitializer adapts discovery claim events to production
// primitive row initialization.
type ClaimProductionInitializer struct {
	store    *InMemoryStore
	defaults ClaimProductionInitializationDefaults
}

// NewClaimProductionInitializer returns a claim adapter with explicit storage
// and energy defaults.
func NewClaimProductionInitializer(config ClaimProductionInitializerConfig) (*ClaimProductionInitializer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &ClaimProductionInitializer{
		store:    config.Store,
		defaults: config.Defaults,
	}, nil
}

// InitializeClaimProduction implements discovery.ClaimProductionInitializer.
func (initializer *ClaimProductionInitializer) InitializeClaimProduction(input discovery.ClaimProductionInitializeInput) (discovery.ClaimProductionInitializeResult, error) {
	if initializer == nil || initializer.store == nil {
		return discovery.ClaimProductionInitializeResult{}, ErrInvalidClaimProductionInitializerConfig
	}
	if err := input.Validate(); err != nil {
		return discovery.ClaimProductionInitializeResult{}, err
	}
	result, err := initializer.store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              input.PlanetID,
		LastCalculatedAt:      input.ClaimedAt,
		StorageCapacityUnits:  initializer.defaults.StorageCapacityUnits,
		EnergyCapacityPerHour: initializer.defaults.EnergyCapacityPerHour,
		UpdatedAt:             input.ClaimedAt,
	})
	if err != nil {
		return discovery.ClaimProductionInitializeResult{}, err
	}
	return discovery.ClaimProductionInitializeResult{
		Created:            result.Created,
		AlreadyInitialized: !result.Created,
	}, nil
}

// Validate reports whether config has an explicit store and usable defaults.
func (config ClaimProductionInitializerConfig) Validate() error {
	if config.Store == nil {
		return fmt.Errorf("store: %w", ErrInvalidClaimProductionInitializerConfig)
	}
	if err := config.Defaults.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether defaults can initialize production rows.
func (defaults ClaimProductionInitializationDefaults) Validate() error {
	if err := validatePositiveBoundedAmount("storage capacity", defaults.StorageCapacityUnits, ErrInvalidStorageCapacity); err != nil {
		return err
	}
	if err := validateNonNegativeBoundedAmount("energy capacity per hour", defaults.EnergyCapacityPerHour, ErrInvalidEnergyRate); err != nil {
		return err
	}
	return nil
}

package crafting

import (
	"fmt"
	"sort"
	"sync"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
)

const (
	craftStartReason    economy.LedgerReason = "craft_start"
	craftFeeReason      economy.LedgerReason = "craft_fee"
	craftCompleteReason economy.LedgerReason = "craft_complete"

	craftXPMainAmount int64 = 40
	craftXPRoleAmount int64 = 100
)

// ItemDefinitionProvider resolves server-owned economy item definitions for
// recipe inputs and item outputs.
type ItemDefinitionProvider interface {
	ItemDefinition(itemID foundation.ItemID) (economy.ItemDefinition, bool)
}

// ItemDefinitionMap is a small in-memory ItemDefinitionProvider.
type ItemDefinitionMap map[foundation.ItemID]economy.ItemDefinition

// ItemDefinition returns a defensive copy of the configured item definition.
func (definitions ItemDefinitionMap) ItemDefinition(itemID foundation.ItemID) (economy.ItemDefinition, bool) {
	definition, ok := definitions[itemID]
	if !ok {
		return economy.ItemDefinition{}, false
	}
	return cloneItemDefinition(definition), true
}

// ReservationService is the economy reservation boundary used by crafting.
type ReservationService interface {
	ReserveItems(input economy.ReserveItemsInput) (economy.ReserveItemsResult, error)
	ReleaseReservation(reservationID economy.ReservationID) (economy.ReleaseReservationResult, error)
	CommitReservation(reservationID economy.ReservationID) (economy.CommitReservationResult, error)
}

// InventoryService is the economy inventory boundary used by crafting outputs.
type InventoryService interface {
	AddItem(input economy.AddItemInput) (economy.AddItemResult, error)
}

// WalletService is the economy wallet boundary used by crafting fees.
type WalletService interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
}

// ProgressionService is the progression boundary used for gates and craft XP.
type ProgressionService interface {
	GetProgressionSnapshot(playerID foundation.PlayerID) (progression.ProgressionSnapshot, error)
	GrantXP(input progression.GrantXPInput) (progression.GrantXPResult, error)
}

// ShipService is the ship boundary used by ship unlock recipes.
type ShipService interface {
	UnlockShip(input ships.UnlockShipInput) (ships.UnlockShipResult, error)
}

// CraftingServiceConfig wires CraftingService to public gameplay boundaries.
type CraftingServiceConfig struct {
	Clock           foundation.Clock
	Recipes         RecipeCatalog
	ItemDefinitions ItemDefinitionProvider
	Reservations    ReservationService
	Inventory       InventoryService
	Wallet          WalletService
	Progression     ProgressionService
	Ships           ShipService
}

// CraftingService owns in-memory craft job orchestration for the Phase 06 MVP.
type CraftingService struct {
	mu    sync.Mutex
	clock foundation.Clock

	recipes         RecipeCatalog
	itemDefinitions ItemDefinitionProvider
	reservations    ReservationService
	inventory       InventoryService
	wallet          WalletService
	progression     ProgressionService
	ships           ShipService

	nextJobSequence int64
	jobs            map[CraftJobID]CraftJob
	completions     map[CraftJobID]CompleteCraftResult
}

// StartCraftInput describes one server-authoritative craft start request.
type StartCraftInput struct {
	PlayerID foundation.PlayerID  `json:"player_id"`
	RecipeID catalog.DefinitionID `json:"recipe_id"`
	Location CraftLocation        `json:"location"`
}

// StartCraftResult reports the running job and economy mutations.
type StartCraftResult struct {
	Job            CraftJob                  `json:"job"`
	Recipe         RecipeDefinition          `json:"recipe"`
	Reservation    economy.Reservation       `json:"reservation"`
	WalletDebit    economy.DebitWalletResult `json:"wallet_debit"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
	SourceLocation economy.ItemLocation      `json:"source_location"`
}

// CompleteCraftInput describes one craft completion request.
type CompleteCraftInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	JobID    CraftJobID          `json:"job_id"`
}

// CompleteCraftResult reports the output mutations applied by completion.
type CompleteCraftResult struct {
	Job               CraftJob                        `json:"job"`
	Recipe            RecipeDefinition                `json:"recipe"`
	ReservationCommit economy.CommitReservationResult `json:"reservation_commit"`
	ItemOutput        *economy.AddItemResult          `json:"item_output,omitempty"`
	ShipUnlock        *ships.UnlockShipResult         `json:"ship_unlock,omitempty"`
	XPGrant           progression.GrantXPResult       `json:"xp_grant"`
	ReferenceKey      foundation.IdempotencyKey       `json:"reference_id"`
	OutputLocation    economy.ItemLocation            `json:"output_location,omitempty"`
	Duplicate         bool                            `json:"duplicate"`
}

// NewCraftingService returns an in-memory crafting orchestrator.
func NewCraftingService(config CraftingServiceConfig) (*CraftingService, error) {
	if len(config.Recipes.Definitions()) == 0 {
		return nil, ErrMissingRecipeCatalog
	}
	if config.ItemDefinitions == nil {
		return nil, ErrMissingItemDefinitions
	}
	if config.Reservations == nil {
		return nil, ErrMissingReservationService
	}
	if config.Inventory == nil {
		return nil, ErrMissingInventoryService
	}
	if config.Wallet == nil {
		return nil, ErrMissingWalletService
	}
	if config.Progression == nil {
		return nil, ErrMissingProgressionService
	}
	if config.Ships == nil {
		return nil, ErrMissingShipService
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}

	return &CraftingService{
		clock:           clock,
		recipes:         config.Recipes,
		itemDefinitions: config.ItemDefinitions,
		reservations:    config.Reservations,
		inventory:       config.Inventory,
		wallet:          config.Wallet,
		progression:     config.Progression,
		ships:           config.Ships,
		jobs:            make(map[CraftJobID]CraftJob),
		completions:     make(map[CraftJobID]CompleteCraftResult),
	}, nil
}

// StartCraft validates recipe gates, reserves materials, debits the craft fee,
// and stores a running server-timed craft job.
func (service *CraftingService) StartCraft(input StartCraftInput) (StartCraftResult, error) {
	if service == nil {
		return StartCraftResult{}, ErrMissingRecipeCatalog
	}
	if err := input.validate(); err != nil {
		return StartCraftResult{}, err
	}

	recipe, err := service.recipes.MustGet(input.RecipeID)
	if err != nil {
		return StartCraftResult{}, err
	}
	snapshot, err := service.progression.GetProgressionSnapshot(input.PlayerID)
	if err != nil {
		return StartCraftResult{}, err
	}
	if err := recipe.ValidateRequirements(snapshot.Player.Rank, roleLevelsForRequirements(snapshot), input.Location); err != nil {
		return StartCraftResult{}, err
	}
	sourceLocation, err := craftItemLocation(input.PlayerID, input.Location)
	if err != nil {
		return StartCraftResult{}, err
	}
	requirements, err := service.reserveRequirements(recipe, sourceLocation)
	if err != nil {
		return StartCraftResult{}, err
	}

	service.mu.Lock()
	jobID := service.nextCraftJobIDLocked()
	reservationID := reservationIDForJob(jobID)
	referenceKey, err := craftReferenceKey(jobID)
	if err != nil {
		service.mu.Unlock()
		return StartCraftResult{}, err
	}
	job, err := NewCraftJob(jobID, input.PlayerID, recipe, reservationID, input.Location, service.clock.Now())
	if err != nil {
		service.mu.Unlock()
		return StartCraftResult{}, err
	}
	service.mu.Unlock()

	reservation, err := service.reservations.ReserveItems(economy.ReserveItemsInput{
		ReservationID:      reservationID,
		Kind:               economy.ReservationKindCraft,
		PlayerID:           input.PlayerID,
		Requirements:       requirements,
		ReservedLocationID: economy.LocationID(jobID.String()),
		Reason:             craftStartReason,
		ReferenceKey:       referenceKey,
	})
	if err != nil {
		return StartCraftResult{}, err
	}

	walletDebit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.PlayerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       recipe.RequiredCredits.Int64(),
		Reason:       craftFeeReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		if _, releaseErr := service.reservations.ReleaseReservation(reservationID); releaseErr != nil {
			return StartCraftResult{}, fmt.Errorf("craft fee debit: %w; release reservation %q: %v", err, reservationID, releaseErr)
		}
		return StartCraftResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if _, exists := service.jobs[jobID]; exists {
		return StartCraftResult{}, fmt.Errorf("craft job %q: %w", jobID, ErrCraftJobAlreadyExists)
	}
	service.jobs[jobID] = cloneCraftJob(job)

	return cloneStartCraftResult(StartCraftResult{
		Job:            job,
		Recipe:         recipe,
		Reservation:    reservation.Reservation,
		WalletDebit:    walletDebit,
		ReferenceKey:   referenceKey,
		SourceLocation: sourceLocation,
	}), nil
}

// CompleteCraft rejects early completion, consumes reserved materials, creates
// the recipe output, grants craft XP, and marks the job completed once.
func (service *CraftingService) CompleteCraft(input CompleteCraftInput) (CompleteCraftResult, error) {
	if service == nil {
		return CompleteCraftResult{}, ErrMissingRecipeCatalog
	}
	if err := input.validate(); err != nil {
		return CompleteCraftResult{}, err
	}

	service.mu.Lock()
	if previous, ok := service.completions[input.JobID]; ok {
		service.mu.Unlock()
		if previous.Job.PlayerID != input.PlayerID {
			return CompleteCraftResult{}, fmt.Errorf("craft job %q player %q want %q: %w", input.JobID, input.PlayerID, previous.Job.PlayerID, ErrCraftJobPlayerMismatch)
		}
		result := cloneCompleteCraftResult(previous)
		result.Duplicate = true
		return result, nil
	}
	job, ok := service.jobs[input.JobID]
	if !ok {
		service.mu.Unlock()
		return CompleteCraftResult{}, fmt.Errorf("craft job %q: %w", input.JobID, ErrCraftJobNotFound)
	}
	job = cloneCraftJob(job)
	if job.PlayerID != input.PlayerID {
		service.mu.Unlock()
		return CompleteCraftResult{}, fmt.Errorf("craft job %q player %q want %q: %w", input.JobID, input.PlayerID, job.PlayerID, ErrCraftJobPlayerMismatch)
	}
	if job.State != CraftJobStateRunning {
		service.mu.Unlock()
		return CompleteCraftResult{}, fmt.Errorf("craft job %q state %q: %w", input.JobID, job.State, ErrInvalidCraftJobState)
	}
	now := service.clock.Now()
	if now.Before(job.CompletesAt) {
		service.mu.Unlock()
		return CompleteCraftResult{}, fmt.Errorf("craft job %q completes at %s: %w", input.JobID, job.CompletesAt, ErrCraftNotReady)
	}
	service.mu.Unlock()

	recipe, err := service.recipeForJob(job)
	if err != nil {
		return CompleteCraftResult{}, err
	}
	referenceKey, err := craftReferenceKey(job.JobID)
	if err != nil {
		return CompleteCraftResult{}, err
	}
	outputLocation, err := craftItemLocation(job.PlayerID, job.Location)
	if err != nil {
		return CompleteCraftResult{}, err
	}

	var outputDefinition economy.ItemDefinition
	if recipe.Output.Kind == RecipeOutputKindItem {
		outputDefinition, err = service.itemDefinition(recipe.Output.ItemID)
		if err != nil {
			return CompleteCraftResult{}, err
		}
	}

	commitResult, err := service.reservations.CommitReservation(job.ReservationID)
	if err != nil {
		return CompleteCraftResult{}, err
	}
	commitTime := service.clock.Now()

	var itemOutput *economy.AddItemResult
	var shipUnlock *ships.UnlockShipResult
	switch recipe.Output.Kind {
	case RecipeOutputKindItem:
		result, err := service.inventory.AddItem(economy.AddItemInput{
			PlayerID:       job.PlayerID,
			ItemDefinition: outputDefinition,
			Quantity:       recipe.Output.Quantity.Int64(),
			Location:       outputLocation,
			Reason:         craftCompleteReason,
			ReferenceKey:   referenceKey,
		})
		if err != nil {
			return CompleteCraftResult{}, err
		}
		cloned := cloneAddItemResult(result)
		itemOutput = &cloned
	case RecipeOutputKindShipUnlock:
		result, err := service.ships.UnlockShip(ships.UnlockShipInput{
			PlayerID:    job.PlayerID,
			ShipID:      recipe.Output.ShipID,
			Source:      "craft",
			ReferenceID: referenceKey.String(),
		})
		if err != nil {
			return CompleteCraftResult{}, err
		}
		cloned := result
		shipUnlock = &cloned
	default:
		return CompleteCraftResult{}, fmt.Errorf("recipe output %q: %w", recipe.Output.Kind, ErrUnsupportedRecipeOutput)
	}
	outputTime := service.clock.Now()

	xpGrant, err := service.progression.GrantXP(progression.GrantXPInput{
		PlayerID:       job.PlayerID,
		Amount:         craftXPMainAmount,
		SourceType:     progression.XPSourceTypeCraft,
		SourceID:       progression.XPSourceID(job.JobID.String()),
		IdempotencyKey: progression.XPIdempotencyKey(referenceKey.String()),
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeCrafting, Amount: craftXPRoleAmount},
		},
	})
	if err != nil {
		return CompleteCraftResult{}, err
	}
	xpTime := service.clock.Now()

	completedAt := service.clock.Now()
	job.State = CraftJobStateCompleted
	job.ReservationCommittedAt = &commitTime
	job.OutputGrantedAt = &outputTime
	job.XPGrantedAt = &xpTime
	job.CompletedAt = &completedAt
	if err := job.Validate(); err != nil {
		return CompleteCraftResult{}, err
	}

	result := CompleteCraftResult{
		Job:               job,
		Recipe:            recipe,
		ReservationCommit: commitResult,
		ItemOutput:        itemOutput,
		ShipUnlock:        shipUnlock,
		XPGrant:           xpGrant,
		ReferenceKey:      referenceKey,
		OutputLocation:    outputLocation,
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if previous, ok := service.completions[input.JobID]; ok {
		duplicate := cloneCompleteCraftResult(previous)
		duplicate.Duplicate = true
		return duplicate, nil
	}
	service.jobs[input.JobID] = cloneCraftJob(job)
	service.completions[input.JobID] = cloneCompleteCraftResult(result)

	return cloneCompleteCraftResult(result), nil
}

// Job returns a craft job snapshot.
func (service *CraftingService) Job(jobID CraftJobID) (CraftJob, bool) {
	if service == nil {
		return CraftJob{}, false
	}
	service.mu.Lock()
	defer service.mu.Unlock()

	job, ok := service.jobs[jobID]
	if !ok {
		return CraftJob{}, false
	}
	return cloneCraftJob(job), true
}

// Jobs returns all craft jobs in deterministic id order.
func (service *CraftingService) Jobs() []CraftJob {
	if service == nil {
		return nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()

	jobs := make([]CraftJob, 0, len(service.jobs))
	for _, job := range service.jobs {
		jobs = append(jobs, cloneCraftJob(job))
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].JobID < jobs[j].JobID
	})
	return jobs
}

func (input StartCraftInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.RecipeID.Validate(); err != nil {
		return err
	}
	return input.Location.Validate()
}

func (input CompleteCraftInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	return input.JobID.Validate()
}

func (service *CraftingService) reserveRequirements(
	recipe RecipeDefinition,
	sourceLocation economy.ItemLocation,
) ([]economy.ReserveItemRequirement, error) {
	requirements := make([]economy.ReserveItemRequirement, 0, len(recipe.Inputs))
	for _, input := range recipe.Inputs {
		definition, err := service.itemDefinition(input.ItemID)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, economy.ReserveItemRequirement{
			Definition:   definition,
			Quantity:     input.Quantity.Int64(),
			FromLocation: sourceLocation,
		})
	}
	return requirements, nil
}

func (service *CraftingService) itemDefinition(itemID foundation.ItemID) (economy.ItemDefinition, error) {
	definition, ok := service.itemDefinitions.ItemDefinition(itemID)
	if !ok {
		return economy.ItemDefinition{}, fmt.Errorf("item %q: %w", itemID, ErrUnknownCraftItem)
	}
	if err := definition.Validate(); err != nil {
		return economy.ItemDefinition{}, err
	}
	return definition, nil
}

func (service *CraftingService) recipeForJob(job CraftJob) (RecipeDefinition, error) {
	recipe, err := service.recipes.MustGet(job.RecipeSource.DefinitionID)
	if err != nil {
		return RecipeDefinition{}, err
	}
	if recipe.Source.Version != job.RecipeSource.Version {
		return RecipeDefinition{}, fmt.Errorf("recipe %q job version %q catalog version %q: %w", job.RecipeSource.DefinitionID, job.RecipeSource.Version, recipe.Source.Version, ErrRecipeVersionMismatch)
	}
	return recipe, nil
}

func (service *CraftingService) nextCraftJobIDLocked() CraftJobID {
	service.nextJobSequence++
	return CraftJobID(fmt.Sprintf("craft-job-%d", service.nextJobSequence))
}

func reservationIDForJob(jobID CraftJobID) economy.ReservationID {
	return economy.ReservationID(jobID.String() + "-reservation")
}

func craftReferenceKey(jobID CraftJobID) (foundation.IdempotencyKey, error) {
	return foundation.CraftCompleteIdempotencyKey(jobID.String())
}

func craftItemLocation(playerID foundation.PlayerID, location CraftLocation) (economy.ItemLocation, error) {
	if err := playerID.Validate(); err != nil {
		return economy.ItemLocation{}, err
	}
	if err := location.Validate(); err != nil {
		return economy.ItemLocation{}, err
	}
	switch location.Type {
	case CraftLocationStation, CraftLocationSpecialEventStation:
		return economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	case CraftLocationOwnedPlanet, CraftLocationPlanetBuilding:
		return economy.NewItemLocation(economy.LocationKindPlanetStorage, location.ID)
	default:
		return economy.ItemLocation{}, location.Type.Validate()
	}
}

func roleLevelsForRequirements(snapshot progression.ProgressionSnapshot) map[progression.RoleType]int {
	roleLevels := snapshot.RoleLevelMap()
	levels := make(map[progression.RoleType]int, len(progression.SupportedRoleTypes()))
	// Missing role rows represent zero-XP tracks, which start at level 1.
	for _, role := range progression.SupportedRoleTypes() {
		levels[role] = progression.MinProgressionLevel
	}
	for role, state := range roleLevels {
		levels[role] = state.Level
	}
	return levels
}

func cloneStartCraftResult(result StartCraftResult) StartCraftResult {
	result.Job = cloneCraftJob(result.Job)
	result.Recipe = cloneRecipeDefinition(result.Recipe)
	return result
}

func cloneCompleteCraftResult(result CompleteCraftResult) CompleteCraftResult {
	result.Job = cloneCraftJob(result.Job)
	result.Recipe = cloneRecipeDefinition(result.Recipe)
	result.ReservationCommit.Moves = append([]economy.MoveItemResult(nil), result.ReservationCommit.Moves...)
	if result.ItemOutput != nil {
		itemOutput := cloneAddItemResult(*result.ItemOutput)
		result.ItemOutput = &itemOutput
	}
	if result.ShipUnlock != nil {
		shipUnlock := *result.ShipUnlock
		result.ShipUnlock = &shipUnlock
	}
	result.XPGrant.Snapshot = result.XPGrant.Snapshot.Clone()
	result.XPGrant.RoleLevelUps = append([]progression.RoleLevelChange(nil), result.XPGrant.RoleLevelUps...)
	result.XPGrant.StatInvalidationSignals = append([]progression.StatInvalidationSignal(nil), result.XPGrant.StatInvalidationSignals...)
	return result
}

func cloneAddItemResult(result economy.AddItemResult) economy.AddItemResult {
	result.StackableItems = append([]economy.StackableItem(nil), result.StackableItems...)
	result.InstanceItems = append([]economy.InstanceItem(nil), result.InstanceItems...)
	return result
}

func cloneItemDefinition(definition economy.ItemDefinition) economy.ItemDefinition {
	definition.TradeFlags = append([]economy.TradeFlag(nil), definition.TradeFlags...)
	definition.BindRules = append([]economy.BindRule(nil), definition.BindRules...)
	if definition.MetadataSchema != nil {
		definition.MetadataSchema = append([]byte(nil), definition.MetadataSchema...)
	}
	return definition
}

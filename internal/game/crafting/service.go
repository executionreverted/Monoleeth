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

// CraftLocationAuthorizationInput contains the server-owned context needed to
// authorize a recipe location before crafting reserves materials or debits fees.
type CraftLocationAuthorizationInput struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	Recipe   RecipeDefinition    `json:"recipe"`
	Location CraftLocation       `json:"location"`
}

// CraftLocationAuthorizer validates that a player can craft this recipe at the
// requested location. Runtime station, planet, and building ownership checks can
// plug in behind this boundary without changing crafting economy flows.
type CraftLocationAuthorizer interface {
	AuthorizeCraftLocation(input CraftLocationAuthorizationInput) error
}

// ShipService is the ship boundary used by ship unlock recipes.
type ShipService interface {
	UnlockShip(input ships.UnlockShipInput) (ships.UnlockShipResult, error)
	GetHangar(playerID foundation.PlayerID) (ships.HangarSnapshot, error)
}

// CraftingServiceConfig wires CraftingService to public gameplay boundaries.
type CraftingServiceConfig struct {
	Clock              foundation.Clock
	Recipes            RecipeCatalog
	ItemDefinitions    ItemDefinitionProvider
	Reservations       ReservationService
	Inventory          InventoryService
	Wallet             WalletService
	Progression        ProgressionService
	Ships              ShipService
	LocationAuthorizer CraftLocationAuthorizer
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
	locationAuth    CraftLocationAuthorizer

	nextJobSequence int64
	jobs            map[CraftJobID]CraftJob
	startResults    map[craftStartReferenceKey]StartCraftResult
	startInFlight   map[craftStartReferenceKey]*startCraftInFlight
	completions     map[CraftJobID]CompleteCraftResult
	completing      map[CraftJobID]*completionInFlight
}

type completionInFlight struct {
	done   chan struct{}
	result CompleteCraftResult
	err    error
}

type craftStartReferenceKey struct {
	playerID     foundation.PlayerID
	referenceKey foundation.IdempotencyKey
}

type startCraftInFlight struct {
	input  StartCraftInput
	done   chan struct{}
	result StartCraftResult
	err    error
}

// StartCraftInput describes one server-authoritative craft start request.
type StartCraftInput struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	RecipeID     catalog.DefinitionID      `json:"recipe_id"`
	Location     CraftLocation             `json:"location"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// StartCraftResult reports the running job and economy mutations.
type StartCraftResult struct {
	Job            CraftJob                  `json:"job"`
	Recipe         RecipeDefinition          `json:"recipe"`
	Reservation    economy.Reservation       `json:"reservation"`
	WalletDebit    economy.DebitWalletResult `json:"wallet_debit"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
	SourceLocation economy.ItemLocation      `json:"source_location"`
	Duplicate      bool                      `json:"duplicate"`
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
		locationAuth:    config.LocationAuthorizer,
		jobs:            make(map[CraftJobID]CraftJob),
		startResults:    make(map[craftStartReferenceKey]StartCraftResult),
		startInFlight:   make(map[craftStartReferenceKey]*startCraftInFlight),
		completions:     make(map[CraftJobID]CompleteCraftResult),
		completing:      make(map[CraftJobID]*completionInFlight),
	}, nil
}

// StartCraft validates recipe gates, reserves materials, debits the craft fee,
// and stores a running server-timed craft job.
func (service *CraftingService) StartCraft(input StartCraftInput) (result StartCraftResult, err error) {
	if service == nil {
		return StartCraftResult{}, ErrMissingRecipeCatalog
	}
	if err := input.validate(); err != nil {
		return StartCraftResult{}, err
	}
	startKey, inFlight, cached, handled, err := service.beginStartCraft(input)
	if handled || err != nil {
		return cached, err
	}
	defer func() {
		if err != nil {
			service.finishStartCraft(startKey, inFlight, StartCraftResult{}, err)
		}
	}()

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
	if err := service.authorizeCraftLocation(input.PlayerID, recipe, input.Location); err != nil {
		return StartCraftResult{}, err
	}
	if err := service.rejectOwnedNonRepeatableShipOutput(input.PlayerID, recipe); err != nil {
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
	service.mu.Unlock()

	reservationID := reservationIDForJob(jobID)
	job, err := NewCraftJob(jobID, input.PlayerID, recipe, reservationID, input.Location, service.clock.Now())
	if err != nil {
		return StartCraftResult{}, err
	}

	reservation, err := service.reservations.ReserveItems(economy.ReserveItemsInput{
		ReservationID:      reservationID,
		Kind:               economy.ReservationKindCraft,
		PlayerID:           input.PlayerID,
		Requirements:       requirements,
		ReservedLocationID: economy.LocationID(jobID.String()),
		Reason:             craftStartReason,
		ReferenceKey:       input.ReferenceKey,
	})
	if err != nil {
		return StartCraftResult{}, err
	}

	walletDebit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.PlayerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       recipe.RequiredCredits.Int64(),
		Reason:       craftFeeReason,
		ReferenceKey: input.ReferenceKey,
	})
	if err != nil {
		if _, releaseErr := service.reservations.ReleaseReservation(reservationID); releaseErr != nil {
			return StartCraftResult{}, fmt.Errorf("craft fee debit: %w; release reservation %q: %v", err, reservationID, releaseErr)
		}
		return StartCraftResult{}, err
	}

	service.mu.Lock()
	if _, exists := service.jobs[jobID]; exists {
		service.mu.Unlock()
		err = fmt.Errorf("craft job %q: %w", jobID, ErrCraftJobAlreadyExists)
		return StartCraftResult{}, err
	}
	service.jobs[jobID] = cloneCraftJob(job)
	service.mu.Unlock()

	result = StartCraftResult{
		Job:            job,
		Recipe:         recipe,
		Reservation:    reservation.Reservation,
		WalletDebit:    walletDebit,
		ReferenceKey:   input.ReferenceKey,
		SourceLocation: sourceLocation,
	}
	service.finishStartCraft(startKey, inFlight, result, nil)

	return cloneStartCraftResult(result), nil
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
	if inFlight, ok := service.completing[input.JobID]; ok {
		service.mu.Unlock()
		<-inFlight.done
		if inFlight.err != nil {
			return CompleteCraftResult{}, inFlight.err
		}
		result := cloneCompleteCraftResult(inFlight.result)
		result.Duplicate = true
		return result, nil
	}
	inFlight := &completionInFlight{done: make(chan struct{})}
	service.completing[input.JobID] = inFlight
	service.mu.Unlock()

	failCompletion := func(err error) (CompleteCraftResult, error) {
		service.mu.Lock()
		service.finishCompletionInFlightLocked(input.JobID, CompleteCraftResult{}, err)
		service.mu.Unlock()
		return CompleteCraftResult{}, err
	}

	recipe, err := service.recipeForJob(job)
	if err != nil {
		return failCompletion(err)
	}
	referenceKey, err := craftCompleteReferenceKey(job.JobID)
	if err != nil {
		return failCompletion(err)
	}
	outputLocation, err := craftItemLocation(job.PlayerID, job.Location)
	if err != nil {
		return failCompletion(err)
	}

	var outputDefinition economy.ItemDefinition
	if recipe.Output.Kind == RecipeOutputKindItem {
		outputDefinition, err = service.itemDefinition(recipe.Output.ItemID)
		if err != nil {
			return failCompletion(err)
		}
	}

	commitResult, err := service.reservations.CommitReservation(job.ReservationID)
	if err != nil {
		return failCompletion(err)
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
			return failCompletion(err)
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
			return failCompletion(err)
		}
		cloned := result
		shipUnlock = &cloned
	default:
		return failCompletion(fmt.Errorf("recipe output %q: %w", recipe.Output.Kind, ErrUnsupportedRecipeOutput))
	}
	outputTime := service.clock.Now()

	xpGrant, err := service.progression.GrantXP(progression.GrantXPInput{
		PlayerID:       job.PlayerID,
		Amount:         craftXPMainAmount,
		SourceType:     progression.XPSourceTypeCraft,
		SourceID:       progression.XPSourceID(job.JobID.String()),
		IdempotencyKey: progression.XPIdempotencyKey(referenceKey.String()),
		Authority:      progression.XPGrantAuthorityCraftingService,
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeCrafting, Amount: craftXPRoleAmount},
		},
	})
	if err != nil {
		return failCompletion(err)
	}
	xpTime := service.clock.Now()

	completedAt := service.clock.Now()
	job.State = CraftJobStateCompleted
	job.ReservationCommittedAt = &commitTime
	job.OutputGrantedAt = &outputTime
	job.XPGrantedAt = &xpTime
	job.CompletedAt = &completedAt
	if err := job.Validate(); err != nil {
		return failCompletion(err)
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
		service.finishCompletionInFlightLocked(input.JobID, previous, nil)
		return duplicate, nil
	}
	service.jobs[input.JobID] = cloneCraftJob(job)
	service.completions[input.JobID] = cloneCompleteCraftResult(result)
	service.finishCompletionInFlightLocked(input.JobID, result, nil)

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
	if err := input.Location.Validate(); err != nil {
		return err
	}
	return input.ReferenceKey.Validate()
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

func (service *CraftingService) rejectOwnedNonRepeatableShipOutput(playerID foundation.PlayerID, recipe RecipeDefinition) error {
	if recipe.Repeatable || recipe.Output.Kind != RecipeOutputKindShipUnlock {
		return nil
	}
	hangar, err := service.ships.GetHangar(playerID)
	if err != nil {
		return err
	}
	for _, playerShip := range hangar.Ships {
		if playerShip.ShipID == recipe.Output.ShipID {
			return fmt.Errorf("ship %q: %w", recipe.Output.ShipID, ErrCraftOutputAlreadyOwned)
		}
	}
	return nil
}

func (service *CraftingService) authorizeCraftLocation(
	playerID foundation.PlayerID,
	recipe RecipeDefinition,
	location CraftLocation,
) error {
	if service.locationAuth == nil {
		if requiresAuthoritativeCraftLocation(recipe.RequiredLocationType) {
			return fmt.Errorf("location type %q: %w", recipe.RequiredLocationType, ErrMissingLocationAuthorizer)
		}
		return nil
	}
	return service.locationAuth.AuthorizeCraftLocation(CraftLocationAuthorizationInput{
		PlayerID: playerID,
		Recipe:   recipe,
		Location: location,
	})
}

func (service *CraftingService) finishCompletionInFlightLocked(jobID CraftJobID, result CompleteCraftResult, err error) {
	inFlight, ok := service.completing[jobID]
	if !ok {
		return
	}
	if err == nil {
		inFlight.result = cloneCompleteCraftResult(result)
	}
	inFlight.err = err
	delete(service.completing, jobID)
	close(inFlight.done)
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

func craftCompleteReferenceKey(jobID CraftJobID) (foundation.IdempotencyKey, error) {
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
	case CraftLocationOwnedPlanet:
		return economy.NewItemLocation(economy.LocationKindPlanetStorage, location.ID)
	case CraftLocationPlanetBuilding:
		return economy.NewItemLocation(economy.LocationKindPlanetStorage, location.PlanetID.String())
	default:
		return economy.ItemLocation{}, location.Type.Validate()
	}
}

func requiresAuthoritativeCraftLocation(locationType CraftLocationType) bool {
	switch locationType {
	case CraftLocationOwnedPlanet, CraftLocationPlanetBuilding:
		return true
	default:
		return false
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

func (service *CraftingService) beginStartCraft(input StartCraftInput) (craftStartReferenceKey, *startCraftInFlight, StartCraftResult, bool, error) {
	startKey := craftStartReferenceKey{
		playerID:     input.PlayerID,
		referenceKey: input.ReferenceKey,
	}
	service.mu.Lock()
	previous, ok := service.startResults[startKey]
	if ok {
		service.mu.Unlock()
		result, err := duplicateStartResultForInput(previous, input)
		return startKey, nil, result, true, err
	}
	if inFlight, ok := service.startInFlight[startKey]; ok {
		if !startCraftInputMatchesInput(inFlight.input, input) {
			service.mu.Unlock()
			return startKey, nil, StartCraftResult{}, true, fmt.Errorf("player %q craft start reference %q: %w", input.PlayerID, input.ReferenceKey, ErrCraftStartReferenceMismatch)
		}
		service.mu.Unlock()
		<-inFlight.done
		if inFlight.err != nil {
			return startKey, nil, StartCraftResult{}, true, inFlight.err
		}
		result, err := duplicateStartResultForInput(inFlight.result, input)
		return startKey, nil, result, true, err
	}
	inFlight := &startCraftInFlight{
		input: input,
		done:  make(chan struct{}),
	}
	service.startInFlight[startKey] = inFlight
	service.mu.Unlock()
	return startKey, inFlight, StartCraftResult{}, false, nil
}

func (service *CraftingService) finishStartCraft(startKey craftStartReferenceKey, inFlight *startCraftInFlight, result StartCraftResult, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	current, ok := service.startInFlight[startKey]
	if !ok || current != inFlight {
		return
	}
	if err == nil {
		current.result = cloneStartCraftResult(result)
		service.startResults[startKey] = cloneStartCraftResult(result)
	} else {
		current.err = err
	}
	delete(service.startInFlight, startKey)
	close(current.done)
}

func duplicateStartResultForInput(previous StartCraftResult, input StartCraftInput) (StartCraftResult, error) {
	if !startCraftResultMatchesInput(previous, input) {
		return StartCraftResult{}, fmt.Errorf("player %q craft start reference %q: %w", input.PlayerID, input.ReferenceKey, ErrCraftStartReferenceMismatch)
	}
	result := cloneStartCraftResult(previous)
	result.Duplicate = true
	return result, nil
}

func cloneStartCraftResult(result StartCraftResult) StartCraftResult {
	result.Job = cloneCraftJob(result.Job)
	result.Recipe = cloneRecipeDefinition(result.Recipe)
	result.Reservation = cloneStartReservation(result.Reservation)
	return result
}

func startCraftInputMatchesInput(previous StartCraftInput, input StartCraftInput) bool {
	return previous.PlayerID == input.PlayerID &&
		previous.RecipeID == input.RecipeID &&
		previous.Location == input.Location
}

func startCraftResultMatchesInput(result StartCraftResult, input StartCraftInput) bool {
	return result.Job.PlayerID == input.PlayerID &&
		result.Recipe.RecipeID == input.RecipeID &&
		result.Job.Location == input.Location
}

func cloneStartReservation(reservation economy.Reservation) economy.Reservation {
	reservation.ItemLines = append([]economy.ReservationItemLine(nil), reservation.ItemLines...)
	reservation.CurrencyLines = append([]economy.ReservationCurrencyLine(nil), reservation.CurrencyLines...)
	if reservation.ExpiresAt != nil {
		expiresAt := *reservation.ExpiresAt
		reservation.ExpiresAt = &expiresAt
	}
	return reservation
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

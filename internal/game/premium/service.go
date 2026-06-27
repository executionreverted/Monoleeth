package premium

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
)

const (
	LedgerReasonPremiumEntitlementClaim economy.LedgerReason = "premium_entitlement_claim"
	LedgerReasonPremiumWeeklyXCore      economy.LedgerReason = "premium_weekly_xcore_purchase"

	premiumProviderOperation               = "premium_provider_entitlement"
	premiumClaimOperation                  = "premium_claim"
	premiumPostCommitCacheAnomalyOperation = "premium_post_commit_cache_anomaly"

	premiumOutboxTopic             = "economy"
	premiumAggregateType           = "premium_entitlement"
	premiumEntitlementCreatedEvent = "premium.entitlement_created"
	premiumEntitlementClaimedEvent = "premium.entitlement_claimed"
	premiumProviderOutboxPrefix    = "premium_provider:"
	premiumClaimOutboxPrefix       = "premium_claim:"
)

// CreateEntitlementInput describes one provider-confirmed entitlement.
type CreateEntitlementInput struct {
	EntitlementID       EntitlementID
	PlayerID            foundation.PlayerID
	Type                EntitlementType
	Provider            ProviderReference
	Payload             EntitlementGrantPayload
	CreatedAt           time.Time
	ProviderConfirmedAt time.Time
}

// ClaimEntitlementInput describes an idempotent claim request.
type ClaimEntitlementInput struct {
	EntitlementID    EntitlementID
	PlayerID         foundation.PlayerID
	RequestReference string
}

// ConfigureWeeklyXCoreStockInput defines one period stock record.
type ConfigureWeeklyXCoreStockInput struct {
	WorldID    foundation.WorldID
	PeriodKey  string
	StockTotal int64
}

// PurchaseWeeklyXCoreInput describes one premium weekly X Core stock purchase.
type PurchaseWeeklyXCoreInput struct {
	PlayerID          foundation.PlayerID
	WorldID           foundation.WorldID
	PeriodKey         string
	PurchaseReference string
	PaymentCurrency   economy.CurrencyBucket
	PriceAmount       int64
}

// RecordSuspiciousTradeInput describes one deterministic fraud-review log.
type RecordSuspiciousTradeInput struct {
	ActorPlayerID  foundation.PlayerID
	CounterpartyID foundation.PlayerID
	Currency       economy.CurrencyBucket
	Amount         int64
	Reason         string
	Reference      string
}

// ApplyProviderRiskLockInput describes one fraud/chargeback provider event.
type ApplyProviderRiskLockInput struct {
	Provider  ProviderReference
	Reason    string
	Reference string
}

// PremiumEntitlementServiceConfig wires premium grants to economy primitives.
type PremiumEntitlementServiceConfig struct {
	Wallet                           *economy.WalletService
	Clock                            foundation.Clock
	IdempotencyStore                 economy.IdempotencyStore
	EntitlementRepository            PremiumEntitlementRepository
	EntitlementTransactionRepository PremiumEntitlementTransactionRepository
	TransitionLogger                 observability.PremiumTransitionLogger
}

// PremiumEntitlementRepository persists entitlement snapshots when durable
// storage is wired.
type PremiumEntitlementRepository interface {
	SavePremiumEntitlement(ctx context.Context, entitlement Entitlement) error
}

// PremiumEntitlementTransactionRepository commits entitlement settlement rows
// through one durable transaction when contentdb is configured.
type PremiumEntitlementTransactionRepository interface {
	PremiumEntitlementRepository
	WithPremiumEntitlementTransaction(ctx context.Context, fn func(PremiumEntitlementTransaction) error) error
}

// PremiumEntitlementTransaction is the single-transaction seam for premium
// provider ingest, entitlement claims, wallet credits, idempotency, and outbox.
type PremiumEntitlementTransaction interface {
	LoadPremiumEntitlementForUpdate(ctx context.Context, entitlementID EntitlementID) (Entitlement, bool, error)
	LoadPremiumEntitlementByProviderForUpdate(ctx context.Context, provider ProviderReference) (Entitlement, bool, error)
	SavePremiumEntitlement(ctx context.Context, entitlement Entitlement) error
	CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error
	ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error)
	CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error)
	InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error
}

// CreateEntitlementResult reports the stored entitlement.
type CreateEntitlementResult struct {
	Entitlement Entitlement
	Duplicate   bool
}

// ClaimEntitlementResult reports the entitlement state and grant created by a claim.
type ClaimEntitlementResult struct {
	Entitlement                   Entitlement
	WalletCredit                  *economy.CreditWalletResult
	LoadoutSlotGrant              *LoadoutSlotGrant
	WeeklyXCorePurchaseRightGrant *WeeklyXCorePurchaseRightGrant
	CosmeticGrant                 *CosmeticGrant
	BadgeGrant                    *BadgeGrant
	Duplicate                     bool
}

// PurchaseWeeklyXCoreResult reports the skeleton grant and stock snapshot.
type PurchaseWeeklyXCoreResult struct {
	Purchase    WeeklyXCorePurchase
	Stock       WeeklyXCoreStockRecord
	WalletDebit *economy.DebitWalletResult
	Duplicate   bool
}

// ApplyProviderRiskLockResult reports a provider risk lock.
type ApplyProviderRiskLockResult struct {
	Lock        ProviderRiskLock
	Entitlement Entitlement
	Duplicate   bool
}

// PremiumEntitlementService is an in-memory premium entitlement MVP.
type PremiumEntitlementService struct {
	mu               sync.Mutex
	clock            foundation.Clock
	wallet           *economy.WalletService
	idempotencyStore economy.IdempotencyStore
	entitlementRepo  PremiumEntitlementRepository
	entitlementTx    PremiumEntitlementTransactionRepository
	transitionLogger observability.PremiumTransitionLogger

	entitlements            map[EntitlementID]Entitlement
	providerReferences      map[providerReferenceKey]EntitlementID
	claimResults            map[claimReferenceKey]ClaimEntitlementResult
	providerIdempotencyRows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	claimIdempotencyRows    map[foundation.IdempotencyKey]economy.IdempotencyKeyRow

	loadoutSlotGrants              []LoadoutSlotGrant
	weeklyXCorePurchaseRightGrants []WeeklyXCorePurchaseRightGrant
	cosmeticGrants                 []CosmeticGrant
	badgeGrants                    []BadgeGrant

	weeklyStock                  map[weeklyStockKey]WeeklyXCoreStockRecord
	weeklyPurchases              []WeeklyXCorePurchase
	weeklyPurchaseByReference    map[string]PurchaseWeeklyXCoreResult
	weeklyPurchaseByPlayerPeriod map[playerPeriodKey]WeeklyXCorePurchase

	suspiciousTradeLogs []SuspiciousTradeLog
	nextSuspiciousLogID int64

	providerRiskLocks       []ProviderRiskLock
	providerRiskByReference map[string]ApplyProviderRiskLockResult
	nextProviderRiskLockID  int64
}

type providerReferenceKey struct {
	source    string
	reference string
}

type claimReferenceKey struct {
	entitlementID EntitlementID
	playerID      foundation.PlayerID
	reference     string
}

type weeklyStockKey struct {
	worldID   foundation.WorldID
	periodKey string
}

type playerPeriodKey struct {
	playerID  foundation.PlayerID
	periodKey string
}

// NewPremiumEntitlementService returns a concurrency-safe in-memory service.
func NewPremiumEntitlementService(wallet *economy.WalletService, clock foundation.Clock) (*PremiumEntitlementService, error) {
	return NewPremiumEntitlementServiceWithConfig(PremiumEntitlementServiceConfig{
		Wallet: wallet,
		Clock:  clock,
	})
}

// NewPremiumEntitlementServiceWithConfig returns a premium service with optional
// durable economy idempotency rows.
func NewPremiumEntitlementServiceWithConfig(config PremiumEntitlementServiceConfig) (*PremiumEntitlementService, error) {
	if config.Wallet == nil {
		return nil, ErrNilWalletService
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	entitlementRepo := config.EntitlementRepository
	entitlementTx := config.EntitlementTransactionRepository
	if entitlementTx == nil {
		if configured, ok := entitlementRepo.(PremiumEntitlementTransactionRepository); ok {
			entitlementTx = configured
		}
	}
	if entitlementRepo == nil && entitlementTx != nil {
		entitlementRepo = entitlementTx
	}
	return &PremiumEntitlementService{
		clock:                        clock,
		wallet:                       config.Wallet,
		idempotencyStore:             config.IdempotencyStore,
		entitlementRepo:              entitlementRepo,
		entitlementTx:                entitlementTx,
		transitionLogger:             config.TransitionLogger,
		entitlements:                 make(map[EntitlementID]Entitlement),
		providerReferences:           make(map[providerReferenceKey]EntitlementID),
		claimResults:                 make(map[claimReferenceKey]ClaimEntitlementResult),
		providerIdempotencyRows:      make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow),
		claimIdempotencyRows:         make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow),
		weeklyStock:                  make(map[weeklyStockKey]WeeklyXCoreStockRecord),
		weeklyPurchaseByReference:    make(map[string]PurchaseWeeklyXCoreResult),
		weeklyPurchaseByPlayerPeriod: make(map[playerPeriodKey]WeeklyXCorePurchase),
		providerRiskByReference:      make(map[string]ApplyProviderRiskLockResult),
	}, nil
}

// CreateEntitlement validates and stores a pending entitlement. Provider
// reference replays return the original entitlement without creating value.
func (service *PremiumEntitlementService) CreateEntitlement(input CreateEntitlementInput) (result CreateEntitlementResult, err error) {
	entitlement := Entitlement{
		ID:                  input.EntitlementID,
		PlayerID:            input.PlayerID,
		Type:                input.Type,
		State:               EntitlementStatePending,
		Provider:            input.Provider,
		Payload:             input.Payload,
		CreatedAt:           input.CreatedAt,
		ProviderConfirmedAt: input.ProviderConfirmedAt,
	}
	if err = entitlement.validateCreate(); err != nil {
		return CreateEntitlementResult{}, err
	}
	referenceKey, err := premiumProviderIdempotencyKey(entitlement.Provider)
	if err != nil {
		return CreateEntitlementResult{}, err
	}
	startedAt := service.nowUTC()
	defer func() {
		service.recordPremiumTransition(
			observability.Operation(premiumProviderOperation),
			input.PlayerID,
			"",
			referenceKey,
			premiumTransitionReferenceIDs(input.EntitlementID, result.Entitlement.ID),
			result.Entitlement.Provider,
			input.Provider,
			startedAt,
			err,
		)
	}()
	requestHash, err := premiumProviderRequestHash(input)
	if err != nil {
		return CreateEntitlementResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if service.premiumEntitlementTransactionRepository() != nil {
		return service.createEntitlementWithTransactionLocked(entitlement, input, referenceKey, requestHash)
	}

	idempotencyRow, duplicateResult, duplicate, err := service.claimPremiumProviderIdempotency(entitlement, referenceKey, requestHash)
	if err != nil {
		return CreateEntitlementResult{}, err
	}
	if duplicate {
		if err := service.replayProviderEntitlementLocked(duplicateResult.Entitlement); err != nil {
			return CreateEntitlementResult{}, err
		}
		result = duplicateResult
		return result, nil
	}

	providerKey := providerKey(entitlement.Provider)
	if existingID, ok := service.providerReferences[providerKey]; ok {
		result = CreateEntitlementResult{
			Entitlement: service.entitlements[existingID],
			Duplicate:   true,
		}
		if err := service.completePremiumProviderIdempotency(idempotencyRow, result); err != nil {
			return CreateEntitlementResult{}, service.failPremiumProviderIdempotency(idempotencyRow, err)
		}
		return result, nil
	}
	if _, exists := service.entitlements[entitlement.ID]; exists {
		return CreateEntitlementResult{}, service.failPremiumProviderIdempotency(
			idempotencyRow,
			fmt.Errorf("entitlement id %q: %w", entitlement.ID, ErrDuplicateEntitlementID),
		)
	}

	snapshot := service.snapshotPremiumProviderMutationLocked()
	service.entitlements[entitlement.ID] = entitlement
	service.providerReferences[providerKey] = entitlement.ID
	result = CreateEntitlementResult{Entitlement: entitlement}
	if err := service.saveEntitlementSnapshot(entitlement); err != nil {
		service.restorePremiumProviderMutationLocked(snapshot)
		return CreateEntitlementResult{}, service.failPremiumProviderIdempotency(idempotencyRow, err)
	}
	if err := service.completePremiumProviderIdempotency(idempotencyRow, result); err != nil {
		service.restorePremiumProviderMutationLocked(snapshot)
		return CreateEntitlementResult{}, service.failPremiumProviderIdempotency(idempotencyRow, err)
	}
	return result, nil
}

// ClaimEntitlement grants a pending entitlement once. Retries with the same
// entitlement/player/request reference return the original claim result with the
// current entitlement state.
func (service *PremiumEntitlementService) ClaimEntitlement(input ClaimEntitlementInput) (result ClaimEntitlementResult, err error) {
	if err = input.EntitlementID.Validate(); err != nil {
		return ClaimEntitlementResult{}, err
	}
	if err = input.PlayerID.Validate(); err != nil {
		return ClaimEntitlementResult{}, err
	}
	if err = validateRequestReference(input.RequestReference); err != nil {
		return ClaimEntitlementResult{}, err
	}
	referenceKey, err := premiumClaimIdempotencyKey(input)
	if err != nil {
		return ClaimEntitlementResult{}, err
	}
	startedAt := service.nowUTC()
	defer func() {
		service.recordPremiumTransition(
			observability.Operation(premiumClaimOperation),
			input.PlayerID,
			input.RequestReference,
			referenceKey,
			premiumTransitionReferenceIDs(input.EntitlementID, result.Entitlement.ID),
			result.Entitlement.Provider,
			ProviderReference{},
			startedAt,
			err,
		)
	}()
	requestHash, err := premiumClaimRequestHash(input)
	if err != nil {
		return ClaimEntitlementResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if service.premiumEntitlementTransactionRepository() != nil {
		return service.claimEntitlementWithTransactionLocked(input, referenceKey, requestHash)
	}

	if service.idempotencyStore == nil {
		entitlement, ok := service.entitlements[input.EntitlementID]
		if !ok {
			return ClaimEntitlementResult{}, ErrEntitlementNotFound
		}
		if entitlement.PlayerID != input.PlayerID {
			return ClaimEntitlementResult{}, ErrEntitlementWrongPlayer
		}
		if entitlement.State == EntitlementStateClaimed && entitlement.ClaimRequestRef != input.RequestReference {
			return ClaimEntitlementResult{}, ErrEntitlementAlreadyClaimed
		}
	}

	idempotencyRow, duplicateResult, duplicate, err := service.claimPremiumClaimIdempotency(input, referenceKey, requestHash)
	if err != nil {
		return ClaimEntitlementResult{}, err
	}
	if duplicate {
		if entitlement, ok := service.entitlements[input.EntitlementID]; ok && entitlement.PlayerID == input.PlayerID {
			duplicateResult.Entitlement = entitlement
		}
		result = duplicateResult
		return result, nil
	}

	claimKey := claimReferenceKey{
		entitlementID: input.EntitlementID,
		playerID:      input.PlayerID,
		reference:     input.RequestReference,
	}
	entitlement, ok := service.entitlements[input.EntitlementID]
	if !ok {
		return ClaimEntitlementResult{}, service.failPremiumClaimIdempotency(idempotencyRow, ErrEntitlementNotFound)
	}
	if entitlement.PlayerID != input.PlayerID {
		return ClaimEntitlementResult{}, service.failPremiumClaimIdempotency(idempotencyRow, ErrEntitlementWrongPlayer)
	}
	if previous, ok := service.claimResults[claimKey]; ok {
		result = cloneClaimEntitlementResult(previous)
		result.Entitlement = entitlement
		result.Duplicate = true
		if err := service.completePremiumClaimIdempotency(idempotencyRow, result); err != nil {
			return ClaimEntitlementResult{}, service.failPremiumClaimIdempotency(idempotencyRow, err)
		}
		return result, nil
	}
	if entitlement.State == EntitlementStateClaimed {
		return ClaimEntitlementResult{}, service.failPremiumClaimIdempotency(idempotencyRow, ErrEntitlementAlreadyClaimed)
	}
	if entitlement.State != EntitlementStatePending {
		return ClaimEntitlementResult{}, service.failPremiumClaimIdempotency(
			idempotencyRow,
			fmt.Errorf("state %q: %w", entitlement.State, ErrEntitlementNotPending),
		)
	}

	snapshot := service.snapshotPremiumClaimMutationLocked()
	rollback := func(cause error) (ClaimEntitlementResult, error) {
		service.restorePremiumClaimMutationLocked(snapshot)
		return ClaimEntitlementResult{}, service.failPremiumClaimIdempotency(idempotencyRow, cause)
	}

	now := service.clock.Now()
	result = ClaimEntitlementResult{}
	switch entitlement.Type {
	case EntitlementTypePremiumCurrencyPack:
		creditResult, err := service.creditPremiumCurrencyPackLocked(entitlement)
		if err != nil {
			return rollback(err)
		}
		result.WalletCredit = &creditResult
	case EntitlementTypeLoadoutSlot:
		grant := LoadoutSlotGrant{
			EntitlementID: entitlement.ID,
			PlayerID:      entitlement.PlayerID,
			Scope:         entitlement.Payload.LoadoutSlotScope,
			Count:         entitlement.Payload.LoadoutSlotCount,
			GrantedAt:     now,
		}
		service.loadoutSlotGrants = append(service.loadoutSlotGrants, grant)
		result.LoadoutSlotGrant = &grant
	case EntitlementTypeWeeklyXCorePurchaseRight:
		grant := WeeklyXCorePurchaseRightGrant{
			EntitlementID: entitlement.ID,
			PlayerID:      entitlement.PlayerID,
			WorldID:       entitlement.Payload.WorldID,
			PeriodKey:     entitlement.Payload.PeriodKey,
			GrantedAt:     now,
		}
		service.weeklyXCorePurchaseRightGrants = append(service.weeklyXCorePurchaseRightGrants, grant)
		result.WeeklyXCorePurchaseRightGrant = &grant
	case EntitlementTypeCosmetic:
		grant := CosmeticGrant{
			EntitlementID: entitlement.ID,
			PlayerID:      entitlement.PlayerID,
			CosmeticID:    entitlement.Payload.CosmeticID,
			GrantedAt:     now,
		}
		service.cosmeticGrants = append(service.cosmeticGrants, grant)
		result.CosmeticGrant = &grant
	case EntitlementTypeBadge:
		grant := BadgeGrant{
			EntitlementID: entitlement.ID,
			PlayerID:      entitlement.PlayerID,
			BadgeID:       entitlement.Payload.BadgeID,
			GrantedAt:     now,
		}
		service.badgeGrants = append(service.badgeGrants, grant)
		result.BadgeGrant = &grant
	default:
		return rollback(fmt.Errorf("entitlement type %q: %w", entitlement.Type, ErrInvalidEntitlementType))
	}

	entitlement.State = EntitlementStateClaimed
	entitlement.ClaimedAt = now
	entitlement.ClaimRequestRef = input.RequestReference
	service.entitlements[entitlement.ID] = entitlement

	result.Entitlement = entitlement
	if err := service.completePremiumClaimIdempotency(idempotencyRow, result); err != nil {
		return rollback(err)
	}
	service.claimResults[claimKey] = cloneClaimEntitlementResult(result)
	return cloneClaimEntitlementResult(result), nil
}

func (service *PremiumEntitlementService) creditPremiumCurrencyPackLocked(entitlement Entitlement) (economy.CreditWalletResult, error) {
	referenceKey, err := foundation.PremiumWebhookIdempotencyKey(providerWalletReference(entitlement.Provider))
	if err != nil {
		return economy.CreditWalletResult{}, err
	}
	return service.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     entitlement.PlayerID,
		Currency:     entitlement.Payload.CurrencyBucket,
		Amount:       entitlement.Payload.Amount,
		Reason:       LedgerReasonPremiumEntitlementClaim,
		ReferenceKey: referenceKey,
	})
}

func (service *PremiumEntitlementService) creditPremiumCurrencyPackWithoutRepositoryLocked(entitlement Entitlement) (economy.CreditWalletResult, error) {
	referenceKey, err := foundation.PremiumWebhookIdempotencyKey(providerWalletReference(entitlement.Provider))
	if err != nil {
		return economy.CreditWalletResult{}, err
	}
	return service.wallet.CreditWalletWithoutRepository(economy.CreditWalletInput{
		PlayerID:     entitlement.PlayerID,
		Currency:     entitlement.Payload.CurrencyBucket,
		Amount:       entitlement.Payload.Amount,
		Reason:       LedgerReasonPremiumEntitlementClaim,
		ReferenceKey: referenceKey,
	})
}

func (service *PremiumEntitlementService) createEntitlementWithTransactionLocked(
	entitlement Entitlement,
	input CreateEntitlementInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (CreateEntitlementResult, error) {
	repository := service.premiumEntitlementTransactionRepository()
	if repository == nil {
		return CreateEntitlementResult{}, errors.New("premium entitlement transaction repository missing")
	}
	idempotencyRow := service.premiumProviderIdempotencyCandidate(entitlement, referenceKey, requestHash)
	snapshot := service.snapshotPremiumProviderMutationLocked()

	var result CreateEntitlementResult
	var completedRow economy.IdempotencyKeyRow
	err := repository.WithPremiumEntitlementTransaction(context.Background(), func(tx PremiumEntitlementTransaction) error {
		claim, err := tx.ClaimIdempotencyKey(context.Background(), idempotencyRow)
		if err != nil {
			return err
		}
		claimedRow, duplicateResult, isDuplicate, err := premiumProviderResultFromClaim(claim)
		if err != nil {
			return err
		}
		if isDuplicate {
			result = duplicateResult
			return nil
		}

		if existing, ok, err := tx.LoadPremiumEntitlementByProviderForUpdate(context.Background(), entitlement.Provider); err != nil {
			return err
		} else if ok {
			result = CreateEntitlementResult{Entitlement: existing, Duplicate: true}
			completedRow, err = service.completedPremiumProviderIdempotencyRow(claimedRow, result)
			if err != nil {
				return err
			}
			_, err = tx.CompleteIdempotencyKey(context.Background(), completedRow)
			return err
		}
		if _, ok, err := tx.LoadPremiumEntitlementForUpdate(context.Background(), entitlement.ID); err != nil {
			return err
		} else if ok {
			return fmt.Errorf("entitlement id %q: %w", entitlement.ID, ErrDuplicateEntitlementID)
		}

		result = CreateEntitlementResult{Entitlement: entitlement}
		completedRow, err = service.completedPremiumProviderIdempotencyRow(claimedRow, result)
		if err != nil {
			return err
		}
		outboxRow, err := service.premiumProviderOutboxRow(result, input, referenceKey)
		if err != nil {
			return err
		}
		if err := tx.SavePremiumEntitlement(context.Background(), entitlement); err != nil {
			return err
		}
		if _, err := tx.CompleteIdempotencyKey(context.Background(), completedRow); err != nil {
			return err
		}
		return tx.InsertOutboxRow(context.Background(), outboxRow)
	})
	if err != nil {
		service.restorePremiumProviderMutationLocked(snapshot)
		return CreateEntitlementResult{}, err
	}
	if result.Duplicate {
		if err := service.replayProviderEntitlementLocked(result.Entitlement); err != nil {
			service.restorePremiumProviderMutationLocked(snapshot)
			return CreateEntitlementResult{}, err
		}
		if err := service.recordCompletedPremiumProviderIdempotencyRow(completedRow); err != nil {
			service.recordPostCommitIdempotencyCacheAnomaly(
				result.Entitlement.PlayerID,
				completedRow.Key,
				premiumTransitionReferenceIDs(result.Entitlement.ID),
				err,
			)
		}
		return result, nil
	}
	service.entitlements[result.Entitlement.ID] = result.Entitlement
	service.providerReferences[providerKey(result.Entitlement.Provider)] = result.Entitlement.ID
	if err := service.recordCompletedPremiumProviderIdempotencyRow(completedRow); err != nil {
		service.recordPostCommitIdempotencyCacheAnomaly(
			result.Entitlement.PlayerID,
			completedRow.Key,
			premiumTransitionReferenceIDs(result.Entitlement.ID),
			err,
		)
	}
	return result, nil
}

func (service *PremiumEntitlementService) claimEntitlementWithTransactionLocked(
	input ClaimEntitlementInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (ClaimEntitlementResult, error) {
	repository := service.premiumEntitlementTransactionRepository()
	if repository == nil {
		return ClaimEntitlementResult{}, errors.New("premium entitlement transaction repository missing")
	}
	idempotencyRow := service.premiumClaimIdempotencyCandidate(input, referenceKey, requestHash)
	snapshot := service.snapshotPremiumClaimMutationLocked()

	var result ClaimEntitlementResult
	var completedRow economy.IdempotencyKeyRow
	var duplicate bool
	err := repository.WithPremiumEntitlementTransaction(context.Background(), func(tx PremiumEntitlementTransaction) error {
		entitlement, ok, err := tx.LoadPremiumEntitlementForUpdate(context.Background(), input.EntitlementID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrEntitlementNotFound
		}
		if entitlement.PlayerID != input.PlayerID {
			return ErrEntitlementWrongPlayer
		}

		claim, err := tx.ClaimIdempotencyKey(context.Background(), idempotencyRow)
		if err != nil {
			return err
		}
		claimedRow, duplicateResult, isDuplicate, err := premiumClaimResultFromClaim(claim)
		if err != nil {
			return err
		}
		if isDuplicate {
			duplicateResult.Entitlement = entitlement
			result = duplicateResult
			duplicate = true
			return nil
		}

		if entitlement.State == EntitlementStateClaimed {
			return ErrEntitlementAlreadyClaimed
		}
		if entitlement.State != EntitlementStatePending {
			return fmt.Errorf("state %q: %w", entitlement.State, ErrEntitlementNotPending)
		}

		now := service.clock.Now()
		result = ClaimEntitlementResult{}
		switch entitlement.Type {
		case EntitlementTypePremiumCurrencyPack:
			creditResult, err := service.creditPremiumCurrencyPackWithoutRepositoryLocked(entitlement)
			if err != nil {
				return err
			}
			result.WalletCredit = &creditResult
		case EntitlementTypeLoadoutSlot:
			grant := LoadoutSlotGrant{
				EntitlementID: entitlement.ID,
				PlayerID:      entitlement.PlayerID,
				Scope:         entitlement.Payload.LoadoutSlotScope,
				Count:         entitlement.Payload.LoadoutSlotCount,
				GrantedAt:     now,
			}
			result.LoadoutSlotGrant = &grant
		case EntitlementTypeWeeklyXCorePurchaseRight:
			grant := WeeklyXCorePurchaseRightGrant{
				EntitlementID: entitlement.ID,
				PlayerID:      entitlement.PlayerID,
				WorldID:       entitlement.Payload.WorldID,
				PeriodKey:     entitlement.Payload.PeriodKey,
				GrantedAt:     now,
			}
			result.WeeklyXCorePurchaseRightGrant = &grant
		case EntitlementTypeCosmetic:
			grant := CosmeticGrant{
				EntitlementID: entitlement.ID,
				PlayerID:      entitlement.PlayerID,
				CosmeticID:    entitlement.Payload.CosmeticID,
				GrantedAt:     now,
			}
			result.CosmeticGrant = &grant
		case EntitlementTypeBadge:
			grant := BadgeGrant{
				EntitlementID: entitlement.ID,
				PlayerID:      entitlement.PlayerID,
				BadgeID:       entitlement.Payload.BadgeID,
				GrantedAt:     now,
			}
			result.BadgeGrant = &grant
		default:
			return fmt.Errorf("entitlement type %q: %w", entitlement.Type, ErrInvalidEntitlementType)
		}

		entitlement.State = EntitlementStateClaimed
		entitlement.ClaimedAt = now
		entitlement.ClaimRequestRef = input.RequestReference
		result.Entitlement = entitlement
		completedRow, err = service.completedPremiumClaimIdempotencyRow(claimedRow, result)
		if err != nil {
			return err
		}
		outboxRow, err := service.premiumClaimOutboxRow(result, input, referenceKey)
		if err != nil {
			return err
		}
		if err := tx.SavePremiumEntitlement(context.Background(), entitlement); err != nil {
			return err
		}
		if result.WalletCredit != nil {
			if err := tx.CommitWalletMutation(context.Background(), premiumWalletCreditCommit(*result.WalletCredit)); err != nil {
				return err
			}
		}
		if _, err := tx.CompleteIdempotencyKey(context.Background(), completedRow); err != nil {
			return err
		}
		return tx.InsertOutboxRow(context.Background(), outboxRow)
	})
	if err != nil {
		service.restorePremiumClaimMutationLocked(snapshot)
		return ClaimEntitlementResult{}, err
	}
	if duplicate {
		return cloneClaimEntitlementResult(result), nil
	}
	service.entitlements[result.Entitlement.ID] = result.Entitlement
	service.providerReferences[providerKey(result.Entitlement.Provider)] = result.Entitlement.ID
	service.recordClaimSkeletonGrantsLocked(result)
	claimKey := claimReferenceKey{
		entitlementID: input.EntitlementID,
		playerID:      input.PlayerID,
		reference:     input.RequestReference,
	}
	service.claimResults[claimKey] = cloneClaimEntitlementResult(result)
	if err := service.recordCompletedPremiumClaimIdempotencyRow(completedRow); err != nil {
		service.recordPostCommitIdempotencyCacheAnomaly(
			input.PlayerID,
			completedRow.Key,
			premiumTransitionReferenceIDs(result.Entitlement.ID),
			err,
		)
	}
	return cloneClaimEntitlementResult(result), nil
}

func (service *PremiumEntitlementService) recordClaimSkeletonGrantsLocked(result ClaimEntitlementResult) {
	if result.LoadoutSlotGrant != nil {
		service.loadoutSlotGrants = append(service.loadoutSlotGrants, *result.LoadoutSlotGrant)
	}
	if result.WeeklyXCorePurchaseRightGrant != nil {
		service.weeklyXCorePurchaseRightGrants = append(service.weeklyXCorePurchaseRightGrants, *result.WeeklyXCorePurchaseRightGrant)
	}
	if result.CosmeticGrant != nil {
		service.cosmeticGrants = append(service.cosmeticGrants, *result.CosmeticGrant)
	}
	if result.BadgeGrant != nil {
		service.badgeGrants = append(service.badgeGrants, *result.BadgeGrant)
	}
}

func (service *PremiumEntitlementService) saveEntitlementSnapshot(entitlement Entitlement) error {
	if service == nil || service.entitlementRepo == nil {
		return nil
	}
	return service.entitlementRepo.SavePremiumEntitlement(context.Background(), entitlement)
}

func (service *PremiumEntitlementService) premiumEntitlementTransactionRepository() PremiumEntitlementTransactionRepository {
	if service == nil {
		return nil
	}
	return service.entitlementTx
}

// ConfigureWeeklyXCoreStock creates or replaces one world/period stock record
// while preserving already-consumed stock.
func (service *PremiumEntitlementService) ConfigureWeeklyXCoreStock(input ConfigureWeeklyXCoreStockInput) (WeeklyXCoreStockRecord, error) {
	if err := input.WorldID.Validate(); err != nil {
		return WeeklyXCoreStockRecord{}, err
	}
	if err := validatePeriodKey(input.PeriodKey); err != nil {
		return WeeklyXCoreStockRecord{}, err
	}
	if input.StockTotal < 0 {
		return WeeklyXCoreStockRecord{}, fmt.Errorf("stock total %d: %w", input.StockTotal, ErrInvalidWeeklyStock)
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.clock.Now()
	key := weeklyStockKey{worldID: input.WorldID, periodKey: input.PeriodKey}
	consumed := int64(0)
	createdAt := now
	if existing, ok := service.weeklyStock[key]; ok {
		consumed = existing.StockTotal - existing.StockRemaining
		createdAt = existing.CreatedAt
	}
	if consumed > input.StockTotal {
		return WeeklyXCoreStockRecord{}, fmt.Errorf("consumed stock %d exceeds total %d: %w", consumed, input.StockTotal, ErrInvalidWeeklyStock)
	}
	record := WeeklyXCoreStockRecord{
		WorldID:        input.WorldID,
		PeriodKey:      input.PeriodKey,
		StockTotal:     input.StockTotal,
		StockRemaining: input.StockTotal - consumed,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}
	service.weeklyStock[key] = record
	return record, nil
}

// PurchaseWeeklyXCore consumes one world stock unit and records one player
// purchase per period. The grant is a service-owned skeleton state.
func (service *PremiumEntitlementService) PurchaseWeeklyXCore(input PurchaseWeeklyXCoreInput) (PurchaseWeeklyXCoreResult, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return PurchaseWeeklyXCoreResult{}, err
	}
	if err := input.WorldID.Validate(); err != nil {
		return PurchaseWeeklyXCoreResult{}, err
	}
	if err := validatePeriodKey(input.PeriodKey); err != nil {
		return PurchaseWeeklyXCoreResult{}, err
	}
	if err := validatePurchaseReference(input.PurchaseReference); err != nil {
		return PurchaseWeeklyXCoreResult{}, err
	}
	if err := ValidatePaidPremiumUse(input.PaymentCurrency); err != nil {
		return PurchaseWeeklyXCoreResult{}, err
	}
	if input.PriceAmount > 0 {
		if _, err := foundation.NewMoney(input.PriceAmount); err != nil {
			return PurchaseWeeklyXCoreResult{}, err
		}
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.weeklyPurchaseByReference[input.PurchaseReference]; ok {
		result := clonePurchaseWeeklyXCoreResult(previous)
		result.Duplicate = true
		return result, nil
	}

	playerPeriod := playerPeriodKey{playerID: input.PlayerID, periodKey: input.PeriodKey}
	if _, ok := service.weeklyPurchaseByPlayerPeriod[playerPeriod]; ok {
		return PurchaseWeeklyXCoreResult{}, ErrWeeklyLimitReached
	}

	stockKey := weeklyStockKey{worldID: input.WorldID, periodKey: input.PeriodKey}
	stock, ok := service.weeklyStock[stockKey]
	if !ok {
		return PurchaseWeeklyXCoreResult{}, ErrWeeklyStockNotSet
	}
	if stock.StockRemaining <= 0 {
		return PurchaseWeeklyXCoreResult{}, ErrWeeklyStockSoldOut
	}
	if input.PriceAmount > 0 && service.wallet.Balance(input.PlayerID, input.PaymentCurrency) < input.PriceAmount {
		return PurchaseWeeklyXCoreResult{}, economy.ErrInsufficientWalletFunds
	}

	var walletDebit *economy.DebitWalletResult
	if input.PriceAmount > 0 {
		debitReference, err := foundation.PremiumWeeklyXCorePurchaseIdempotencyKey(input.PlayerID, input.PeriodKey, input.PurchaseReference)
		if err != nil {
			return PurchaseWeeklyXCoreResult{}, err
		}
		debit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
			PlayerID:     input.PlayerID,
			Currency:     input.PaymentCurrency,
			Amount:       input.PriceAmount,
			Reason:       LedgerReasonPremiumWeeklyXCore,
			ReferenceKey: debitReference,
		})
		if err != nil {
			return PurchaseWeeklyXCoreResult{}, err
		}
		walletDebit = &debit
	}

	stock.StockRemaining--
	stock.UpdatedAt = service.clock.Now()
	purchase := WeeklyXCorePurchase{
		PlayerID:          input.PlayerID,
		WorldID:           input.WorldID,
		PeriodKey:         input.PeriodKey,
		PurchaseReference: input.PurchaseReference,
		PaymentCurrency:   input.PaymentCurrency,
		GrantedAt:         stock.UpdatedAt,
	}
	result := PurchaseWeeklyXCoreResult{
		Purchase:    purchase,
		Stock:       stock,
		WalletDebit: walletDebit,
	}

	service.weeklyStock[stockKey] = stock
	service.weeklyPurchases = append(service.weeklyPurchases, purchase)
	service.weeklyPurchaseByReference[input.PurchaseReference] = clonePurchaseWeeklyXCoreResult(result)
	service.weeklyPurchaseByPlayerPeriod[playerPeriod] = purchase
	return clonePurchaseWeeklyXCoreResult(result), nil
}

// RecordSuspiciousTrade appends one immutable-by-snapshot review event.
func (service *PremiumEntitlementService) RecordSuspiciousTrade(input RecordSuspiciousTradeInput) (SuspiciousTradeLog, error) {
	if err := input.ActorPlayerID.Validate(); err != nil {
		return SuspiciousTradeLog{}, err
	}
	if err := input.CounterpartyID.Validate(); err != nil {
		return SuspiciousTradeLog{}, err
	}
	if err := input.Currency.Validate(); err != nil {
		return SuspiciousTradeLog{}, err
	}
	if _, err := foundation.NewMoney(input.Amount); err != nil {
		return SuspiciousTradeLog{}, err
	}
	if err := validateSuspiciousTradeReason(input.Reason); err != nil {
		return SuspiciousTradeLog{}, err
	}
	if err := validateSuspiciousTradeReference(input.Reference); err != nil {
		return SuspiciousTradeLog{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	service.nextSuspiciousLogID++
	log := SuspiciousTradeLog{
		LogID:          fmt.Sprintf("suspicious-trade-%d", service.nextSuspiciousLogID),
		ActorPlayerID:  input.ActorPlayerID,
		CounterpartyID: input.CounterpartyID,
		Currency:       input.Currency,
		Amount:         input.Amount,
		Reason:         input.Reason,
		Reference:      input.Reference,
		CreatedAt:      service.clock.Now(),
	}
	service.suspiciousTradeLogs = append(service.suspiciousTradeLogs, log)
	return log, nil
}

// ApplyProviderRiskLock records a provider fraud/chargeback hook and revokes
// the matching entitlement for future provider integration.
func (service *PremiumEntitlementService) ApplyProviderRiskLock(input ApplyProviderRiskLockInput) (ApplyProviderRiskLockResult, error) {
	if err := input.Provider.validate(); err != nil {
		return ApplyProviderRiskLockResult{}, err
	}
	if err := validateProviderRiskReason(input.Reason); err != nil {
		return ApplyProviderRiskLockResult{}, err
	}
	if err := validateProviderRiskReference(input.Reference); err != nil {
		return ApplyProviderRiskLockResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.providerRiskByReference[input.Reference]; ok {
		if providerKey(previous.Lock.Provider) != providerKey(input.Provider) {
			return ApplyProviderRiskLockResult{}, ErrProviderRiskReferenceConflict
		}
		result := previous
		result.Duplicate = true
		return result, nil
	}

	entitlementID, ok := service.providerReferences[providerKey(input.Provider)]
	if !ok {
		return ApplyProviderRiskLockResult{}, ErrEntitlementNotFound
	}
	entitlement := service.entitlements[entitlementID]
	previousState := entitlement.State
	entitlement.State = EntitlementStateRevoked
	service.entitlements[entitlementID] = entitlement

	service.nextProviderRiskLockID++
	lock := ProviderRiskLock{
		LockID:        fmt.Sprintf("provider-risk-lock-%d", service.nextProviderRiskLockID),
		EntitlementID: entitlement.ID,
		PlayerID:      entitlement.PlayerID,
		Provider:      input.Provider,
		Reason:        input.Reason,
		Reference:     input.Reference,
		PreviousState: previousState,
		CurrentState:  entitlement.State,
		CreatedAt:     service.clock.Now(),
	}
	result := ApplyProviderRiskLockResult{
		Lock:        lock,
		Entitlement: entitlement,
	}
	service.providerRiskLocks = append(service.providerRiskLocks, lock)
	service.providerRiskByReference[input.Reference] = result
	return result, nil
}

// Entitlements returns a stable entitlement snapshot.
func (service *PremiumEntitlementService) Entitlements() []Entitlement {
	service.mu.Lock()
	defer service.mu.Unlock()

	entitlements := make([]Entitlement, 0, len(service.entitlements))
	for _, entitlement := range service.entitlements {
		entitlements = append(entitlements, entitlement)
	}
	sort.Slice(entitlements, func(i, j int) bool {
		return entitlements[i].ID < entitlements[j].ID
	})
	return entitlements
}

// LoadoutSlotGrants returns a snapshot of loadout slot skeleton grants.
func (service *PremiumEntitlementService) LoadoutSlotGrants() []LoadoutSlotGrant {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]LoadoutSlotGrant(nil), service.loadoutSlotGrants...)
}

// WeeklyXCorePurchaseRightGrants returns a snapshot of claimed right grants.
func (service *PremiumEntitlementService) WeeklyXCorePurchaseRightGrants() []WeeklyXCorePurchaseRightGrant {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]WeeklyXCorePurchaseRightGrant(nil), service.weeklyXCorePurchaseRightGrants...)
}

// WeeklyXCoreStockRecords returns a stable stock snapshot.
func (service *PremiumEntitlementService) WeeklyXCoreStockRecords() []WeeklyXCoreStockRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	records := make([]WeeklyXCoreStockRecord, 0, len(service.weeklyStock))
	for _, record := range service.weeklyStock {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].WorldID != records[j].WorldID {
			return records[i].WorldID < records[j].WorldID
		}
		return records[i].PeriodKey < records[j].PeriodKey
	})
	return records
}

// WeeklyXCorePurchases returns a stable purchase snapshot.
func (service *PremiumEntitlementService) WeeklyXCorePurchases() []WeeklyXCorePurchase {
	service.mu.Lock()
	defer service.mu.Unlock()

	purchases := append([]WeeklyXCorePurchase(nil), service.weeklyPurchases...)
	sort.Slice(purchases, func(i, j int) bool {
		if purchases[i].PeriodKey != purchases[j].PeriodKey {
			return purchases[i].PeriodKey < purchases[j].PeriodKey
		}
		if purchases[i].PlayerID != purchases[j].PlayerID {
			return purchases[i].PlayerID < purchases[j].PlayerID
		}
		return purchases[i].PurchaseReference < purchases[j].PurchaseReference
	})
	return purchases
}

// SuspiciousTradeLogs returns a stable snapshot of review logs.
func (service *PremiumEntitlementService) SuspiciousTradeLogs() []SuspiciousTradeLog {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]SuspiciousTradeLog(nil), service.suspiciousTradeLogs...)
}

// ProviderRiskLocks returns a stable snapshot of provider risk locks.
func (service *PremiumEntitlementService) ProviderRiskLocks() []ProviderRiskLock {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]ProviderRiskLock(nil), service.providerRiskLocks...)
}

// recordPostCommitIdempotencyCacheAnomaly reports an in-memory idempotency cache
// conflict discovered AFTER the durable transaction already committed. The
// committed transaction is authoritative, so such a conflict must not roll back
// committed state; the in-memory cache is an acceleration layer that reconciles
// from durable rows. The anomaly is recorded best-effort through the transition
// logger so it surfaces in metrics/logs without altering the primary operation
// status.
func (service *PremiumEntitlementService) recordPostCommitIdempotencyCacheAnomaly(
	playerID foundation.PlayerID,
	idempotencyKey foundation.IdempotencyKey,
	referenceIDs []string,
	cacheErr error,
) {
	if service == nil || cacheErr == nil {
		return
	}
	startedAt := service.nowUTC()
	service.recordPremiumTransition(
		observability.Operation(premiumPostCommitCacheAnomalyOperation),
		playerID,
		"",
		idempotencyKey,
		referenceIDs,
		ProviderReference{},
		ProviderReference{},
		startedAt,
		cacheErr,
	)
}

func (service *PremiumEntitlementService) recordPremiumTransition(
	operation observability.Operation,
	playerID foundation.PlayerID,
	requestReference string,
	idempotencyKey foundation.IdempotencyKey,
	referenceIDs []string,
	resultProvider ProviderReference,
	fallbackProvider ProviderReference,
	startedAt time.Time,
	transitionErr error,
) {
	if service == nil || service.transitionLogger == nil {
		return
	}
	finishedAt := service.nowUTC()
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}

	status := observability.CommandStatusOK
	var code foundation.Code
	if transitionErr != nil {
		status = observability.CommandStatusError
		code = premiumTransitionErrorCode(transitionErr)
	}

	provider := resultProvider
	if provider.Source == "" && provider.Reference == "" {
		provider = fallbackProvider
	}
	_ = service.transitionLogger.RecordPremiumTransition(observability.PremiumTransitionLogEntry{
		PlayerID:       playerID,
		RequestRef:     requestReference,
		Operation:      operation,
		ErrorCode:      code,
		IdempotencyKey: idempotencyKey,
		ReferenceIDs:   referenceIDs,
		ProviderRefIDs: premiumTransitionProviderRefIDs(provider),
		Duration:       duration,
		Status:         status,
		Timestamp:      startedAt,
	})
}

func premiumTransitionErrorCode(err error) foundation.Code {
	if domainCode, ok := foundation.CodeOf(err); ok {
		return domainCode
	}
	switch {
	case errors.Is(err, ErrEntitlementNotFound):
		return foundation.CodeNotFound
	case errors.Is(err, ErrEntitlementWrongPlayer):
		return foundation.CodeForbidden
	case errors.Is(err, economy.ErrInsufficientWalletFunds):
		return foundation.CodeNotEnoughFunds
	case errors.Is(err, economy.ErrIdempotencyKeyConflict):
		return foundation.CodeRequestReplayMismatch
	case errors.Is(err, ErrEntitlementAlreadyClaimed),
		errors.Is(err, ErrEntitlementNotPending),
		errors.Is(err, ErrDuplicateEntitlementID),
		errors.Is(err, ErrPremiumProviderInProgress),
		errors.Is(err, ErrPremiumClaimInProgress),
		errors.Is(err, ErrInvalidEntitlementType),
		errors.Is(err, ErrInvalidEntitlementState),
		errors.Is(err, ErrInvalidEntitlementGrant),
		errors.Is(err, ErrInvalidProviderSource),
		errors.Is(err, ErrInvalidProviderReference),
		errors.Is(err, ErrInvalidRequestReference),
		errors.Is(err, ErrInvalidPurchaseReference),
		errors.Is(err, ErrInvalidTimestamp),
		errors.Is(err, foundation.ErrEmptyID),
		errors.Is(err, foundation.ErrInvalidID),
		errors.Is(err, foundation.ErrEmptyIdempotencyKey),
		errors.Is(err, foundation.ErrInvalidIdempotencyKey):
		return foundation.CodeInvalidPayload
	default:
		return foundation.CodeInternal
	}
}

func premiumTransitionProviderRefIDs(provider ProviderReference) []string {
	if provider.Source == "" || provider.Reference == "" {
		return nil
	}
	return []string{providerWalletReference(provider)}
}

func premiumTransitionReferenceIDs(entitlementIDs ...EntitlementID) []string {
	seen := make(map[EntitlementID]struct{}, len(entitlementIDs))
	referenceIDs := make([]string, 0, len(entitlementIDs))
	for _, entitlementID := range entitlementIDs {
		if entitlementID.IsZero() {
			continue
		}
		if _, ok := seen[entitlementID]; ok {
			continue
		}
		seen[entitlementID] = struct{}{}
		referenceIDs = append(referenceIDs, "entitlement."+entitlementID.String())
	}
	return referenceIDs
}

func (service *PremiumEntitlementService) nowUTC() time.Time {
	if service == nil || service.clock == nil {
		return foundation.RealClock{}.Now().UTC()
	}
	return service.clock.Now().UTC()
}

func providerKey(provider ProviderReference) providerReferenceKey {
	return providerReferenceKey{source: provider.Source, reference: provider.Reference}
}

func providerWalletReference(provider ProviderReference) string {
	return provider.Source + "." + provider.Reference
}

func cloneClaimEntitlementResult(result ClaimEntitlementResult) ClaimEntitlementResult {
	if result.WalletCredit != nil {
		credit := *result.WalletCredit
		result.WalletCredit = &credit
	}
	if result.LoadoutSlotGrant != nil {
		grant := *result.LoadoutSlotGrant
		result.LoadoutSlotGrant = &grant
	}
	if result.WeeklyXCorePurchaseRightGrant != nil {
		grant := *result.WeeklyXCorePurchaseRightGrant
		result.WeeklyXCorePurchaseRightGrant = &grant
	}
	if result.CosmeticGrant != nil {
		grant := *result.CosmeticGrant
		result.CosmeticGrant = &grant
	}
	if result.BadgeGrant != nil {
		grant := *result.BadgeGrant
		result.BadgeGrant = &grant
	}
	return result
}

func clonePurchaseWeeklyXCoreResult(result PurchaseWeeklyXCoreResult) PurchaseWeeklyXCoreResult {
	if result.WalletDebit != nil {
		debit := *result.WalletDebit
		result.WalletDebit = &debit
	}
	return result
}

package premium

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const LedgerReasonPremiumEntitlementClaim economy.LedgerReason = "premium_entitlement_claim"

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
	Purchase  WeeklyXCorePurchase
	Stock     WeeklyXCoreStockRecord
	Duplicate bool
}

// ApplyProviderRiskLockResult reports a provider risk lock.
type ApplyProviderRiskLockResult struct {
	Lock        ProviderRiskLock
	Entitlement Entitlement
	Duplicate   bool
}

// PremiumEntitlementService is an in-memory premium entitlement MVP.
type PremiumEntitlementService struct {
	mu     sync.Mutex
	clock  foundation.Clock
	wallet *economy.WalletService

	entitlements       map[EntitlementID]Entitlement
	providerReferences map[providerReferenceKey]EntitlementID
	claimResults       map[claimReferenceKey]ClaimEntitlementResult

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
	if wallet == nil {
		return nil, ErrNilWalletService
	}
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &PremiumEntitlementService{
		clock:                        clock,
		wallet:                       wallet,
		entitlements:                 make(map[EntitlementID]Entitlement),
		providerReferences:           make(map[providerReferenceKey]EntitlementID),
		claimResults:                 make(map[claimReferenceKey]ClaimEntitlementResult),
		weeklyStock:                  make(map[weeklyStockKey]WeeklyXCoreStockRecord),
		weeklyPurchaseByReference:    make(map[string]PurchaseWeeklyXCoreResult),
		weeklyPurchaseByPlayerPeriod: make(map[playerPeriodKey]WeeklyXCorePurchase),
		providerRiskByReference:      make(map[string]ApplyProviderRiskLockResult),
	}, nil
}

// CreateEntitlement validates and stores a pending entitlement. Provider
// reference replays return the original entitlement without creating value.
func (service *PremiumEntitlementService) CreateEntitlement(input CreateEntitlementInput) (CreateEntitlementResult, error) {
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
	if err := entitlement.validateCreate(); err != nil {
		return CreateEntitlementResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	providerKey := providerKey(entitlement.Provider)
	if existingID, ok := service.providerReferences[providerKey]; ok {
		return CreateEntitlementResult{
			Entitlement: service.entitlements[existingID],
			Duplicate:   true,
		}, nil
	}
	if _, exists := service.entitlements[entitlement.ID]; exists {
		return CreateEntitlementResult{}, fmt.Errorf("entitlement id %q: %w", entitlement.ID, ErrDuplicateEntitlementID)
	}

	service.entitlements[entitlement.ID] = entitlement
	service.providerReferences[providerKey] = entitlement.ID
	return CreateEntitlementResult{Entitlement: entitlement}, nil
}

// ClaimEntitlement grants a pending entitlement once. Retries with the same
// entitlement/player/request reference return the original claim result with the
// current entitlement state.
func (service *PremiumEntitlementService) ClaimEntitlement(input ClaimEntitlementInput) (ClaimEntitlementResult, error) {
	if err := input.EntitlementID.Validate(); err != nil {
		return ClaimEntitlementResult{}, err
	}
	if err := input.PlayerID.Validate(); err != nil {
		return ClaimEntitlementResult{}, err
	}
	if err := validateRequestReference(input.RequestReference); err != nil {
		return ClaimEntitlementResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	claimKey := claimReferenceKey{
		entitlementID: input.EntitlementID,
		playerID:      input.PlayerID,
		reference:     input.RequestReference,
	}
	entitlement, ok := service.entitlements[input.EntitlementID]
	if !ok {
		return ClaimEntitlementResult{}, ErrEntitlementNotFound
	}
	if entitlement.PlayerID != input.PlayerID {
		return ClaimEntitlementResult{}, ErrEntitlementWrongPlayer
	}
	if previous, ok := service.claimResults[claimKey]; ok {
		result := cloneClaimEntitlementResult(previous)
		result.Entitlement = entitlement
		result.Duplicate = true
		return result, nil
	}
	if entitlement.State == EntitlementStateClaimed {
		return ClaimEntitlementResult{}, ErrEntitlementAlreadyClaimed
	}
	if entitlement.State != EntitlementStatePending {
		return ClaimEntitlementResult{}, fmt.Errorf("state %q: %w", entitlement.State, ErrEntitlementNotPending)
	}

	now := service.clock.Now()
	result := ClaimEntitlementResult{}
	switch entitlement.Type {
	case EntitlementTypePremiumCurrencyPack:
		creditResult, err := service.creditPremiumCurrencyPackLocked(entitlement)
		if err != nil {
			return ClaimEntitlementResult{}, err
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
		return ClaimEntitlementResult{}, fmt.Errorf("entitlement type %q: %w", entitlement.Type, ErrInvalidEntitlementType)
	}

	entitlement.State = EntitlementStateClaimed
	entitlement.ClaimedAt = now
	entitlement.ClaimRequestRef = input.RequestReference
	service.entitlements[entitlement.ID] = entitlement

	result.Entitlement = entitlement
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

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.weeklyPurchaseByReference[input.PurchaseReference]; ok {
		result := previous
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
		Purchase: purchase,
		Stock:    stock,
	}

	service.weeklyStock[stockKey] = stock
	service.weeklyPurchases = append(service.weeklyPurchases, purchase)
	service.weeklyPurchaseByReference[input.PurchaseReference] = result
	service.weeklyPurchaseByPlayerPeriod[playerPeriod] = purchase
	return result, nil
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

package death

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

const (
	// LedgerReasonShipRepair records the credit sink for restoring a disabled ship.
	LedgerReasonShipRepair = economy.LedgerReason("ship_repair")
	// LedgerReasonShipRepairRefund records a compensating refund when ship
	// restore fails after wallet debit in the in-memory orchestration slice.
	LedgerReasonShipRepairRefund = economy.LedgerReason("ship_repair_refund")

	defaultRepairRateBps             int64 = 1_000
	defaultRepairLocationModifierBps int64 = 10_000
	repairBpsDenominator             int64 = 10_000
)

var (
	ErrNilWalletService             = errors.New("nil wallet service")
	ErrEmptyRepairShipCatalog       = errors.New("empty repair ship catalog")
	ErrInvalidRepairCostPolicy      = errors.New("invalid repair cost policy")
	ErrRepairReferenceShipMismatch  = errors.New("repair reference ship mismatch")
	ErrRepairReferenceInputMismatch = errors.New("repair reference input mismatch")
	ErrRepairPreviouslyCompensated  = errors.New("repair previously failed and was compensated")
	ErrRepairCostOverflow           = errors.New("repair cost overflow")
)

// RepairWallet is the wallet boundary RepairService needs for credit repair and
// compensating refund. It intentionally uses public economy service methods.
type RepairWallet interface {
	DebitWallet(economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWallet(economy.CreditWalletInput) (economy.CreditWalletResult, error)
}

type repairWalletLedgerLookup interface {
	FindCurrencyLedgerEntry(economy.CurrencyLedgerReferenceLookup) (economy.CurrencyLedgerEntry, bool)
}

type repairWalletLedgerReader interface {
	CurrencyLedgerEntries() []economy.CurrencyLedgerEntry
}

// ShipRepairer is the ship boundary RepairService needs for validation and
// wallet-free restore. It intentionally uses public ship service methods.
type ShipRepairer interface {
	GetHangar(foundation.PlayerID) (ships.HangarSnapshot, error)
	RepairShip(ships.RepairShipInput) (ships.RepairShipResult, error)
}

// RepairConfig describes RepairService dependencies and catalog policy.
type RepairConfig struct {
	ShipCatalog ships.Catalog
	Wallet      RepairWallet
	Ships       ShipRepairer

	// RepairRateBps defaults to 10%. LocationModifierBps defaults to 100%.
	RepairRateBps       int64
	LocationModifierBps int64
}

// RepairService orchestrates credit repair of disabled ships.
type RepairService struct {
	mu sync.Mutex

	catalog             ships.Catalog
	wallet              RepairWallet
	ships               ShipRepairer
	repairRateBps       int64
	locationModifierBps int64

	attempts map[repairAttemptKey]repairAttemptRecord
	inFlight map[repairAttemptKey]*repairInFlight
}

type repairAttemptKey struct {
	playerID     foundation.PlayerID
	referenceKey foundation.IdempotencyKey
}

type repairAttemptRecord struct {
	input  RepairShipInput
	result RepairShipResult
	err    error
}

type repairInFlight struct {
	input RepairShipInput
	done  chan struct{}
}

// RepairShipInput is one server-authoritative credit repair command.
type RepairShipInput struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	ShipID       foundation.ShipID         `json:"ship_id"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// RepairShipResult reports the completed repair orchestration.
type RepairShipResult struct {
	PlayerID     foundation.PlayerID         `json:"player_id"`
	ShipID       foundation.ShipID           `json:"ship_id"`
	ReferenceKey foundation.IdempotencyKey   `json:"reference_id"`
	Currency     economy.CurrencyBucket      `json:"currency_type"`
	RepairCost   int64                       `json:"repair_cost"`
	WalletDebit  economy.DebitWalletResult   `json:"wallet_debit"`
	WalletRefund *economy.CreditWalletResult `json:"wallet_refund,omitempty"`
	ShipRepair   ships.RepairShipResult      `json:"ship_repair"`
	Repaired     bool                        `json:"repaired"`
	Duplicate    bool                        `json:"duplicate"`
	Compensated  bool                        `json:"compensated"`
}

// NewRepairService returns an in-memory repair orchestrator.
func NewRepairService(config RepairConfig) (*RepairService, error) {
	if len(config.ShipCatalog.All()) == 0 {
		return nil, ErrEmptyRepairShipCatalog
	}
	if config.Wallet == nil {
		return nil, ErrNilWalletService
	}
	if config.Ships == nil {
		return nil, ErrNilShipService
	}
	repairRateBps := config.RepairRateBps
	if repairRateBps == 0 {
		repairRateBps = defaultRepairRateBps
	}
	locationModifierBps := config.LocationModifierBps
	if locationModifierBps == 0 {
		locationModifierBps = defaultRepairLocationModifierBps
	}
	if err := validateRepairCostPolicy(repairRateBps, locationModifierBps); err != nil {
		return nil, err
	}
	return &RepairService{
		catalog:             config.ShipCatalog,
		wallet:              config.Wallet,
		ships:               config.Ships,
		repairRateBps:       repairRateBps,
		locationModifierBps: locationModifierBps,
		attempts:            make(map[repairAttemptKey]repairAttemptRecord),
		inFlight:            make(map[repairAttemptKey]*repairInFlight),
	}, nil
}

// RepairShip validates, charges, and restores one disabled ship. The credit cost
// is always derived from the server-side ship catalog and service policy.
func (service *RepairService) RepairShip(input RepairShipInput) (RepairShipResult, error) {
	if err := input.validate(); err != nil {
		return RepairShipResult{}, err
	}

	attemptKey := repairAttemptKey{playerID: input.PlayerID, referenceKey: input.ReferenceKey}

	for {
		service.mu.Lock()
		if previous, ok := service.attempts[attemptKey]; ok {
			if !sameRepairInput(previous.input, input) {
				service.mu.Unlock()
				return RepairShipResult{}, ErrRepairReferenceInputMismatch
			}
			result := cloneRepairShipResult(previous.result)
			result.Duplicate = true
			service.mu.Unlock()
			if previous.err != nil {
				return result, previous.err
			}
			return result, nil
		}

		if current, ok := service.inFlight[attemptKey]; ok {
			if !sameRepairInput(current.input, input) {
				service.mu.Unlock()
				return RepairShipResult{}, ErrRepairReferenceInputMismatch
			}
			done := current.done
			service.mu.Unlock()
			<-done
			continue
		}

		service.inFlight[attemptKey] = &repairInFlight{
			input: input,
			done:  make(chan struct{}),
		}
		service.mu.Unlock()
		break
	}

	result, err, cache := service.repairShip(input)
	service.finishRepairAttempt(attemptKey, input, result, err, cache)
	return result, err
}

func (service *RepairService) repairShip(input RepairShipInput) (RepairShipResult, error, bool) {
	definition, err := service.catalog.MustGet(input.ShipID)
	if err != nil {
		return RepairShipResult{}, err, false
	}
	cost, err := repairCost(definition, service.repairRateBps, service.locationModifierBps)
	if err != nil {
		return RepairShipResult{}, err, false
	}
	if err := service.requireDisabledShip(input.PlayerID, input.ShipID); err != nil {
		return RepairShipResult{}, err, false
	}
	if service.previouslyRefundedRepair(input) {
		return RepairShipResult{
			PlayerID:     input.PlayerID,
			ShipID:       input.ShipID,
			ReferenceKey: input.ReferenceKey,
			Currency:     economy.CurrencyBucketCredits,
			RepairCost:   cost,
			Compensated:  true,
		}, ErrRepairPreviouslyCompensated, false
	}

	var debit economy.DebitWalletResult
	if cost > 0 {
		debit, err = service.wallet.DebitWallet(economy.DebitWalletInput{
			PlayerID:     input.PlayerID,
			Currency:     economy.CurrencyBucketCredits,
			Amount:       cost,
			Reason:       LedgerReasonShipRepair,
			ReferenceKey: input.ReferenceKey,
		})
		if err != nil {
			return RepairShipResult{}, err, false
		}
	}

	shipRepair, err := service.ships.RepairShip(ships.RepairShipInput{
		PlayerID: input.PlayerID,
		ShipID:   input.ShipID,
	})
	if err != nil {
		result, compensationErr := service.compensateFailedRestore(input, cost, debit)
		if compensationErr != nil {
			return result, fmt.Errorf("%w; repair refund failed: %v", err, compensationErr), false
		}
		resultErr := fmt.Errorf("%w: %w", ErrRepairPreviouslyCompensated, err)
		return result, resultErr, true
	}

	result := RepairShipResult{
		PlayerID:     input.PlayerID,
		ShipID:       input.ShipID,
		ReferenceKey: input.ReferenceKey,
		Currency:     economy.CurrencyBucketCredits,
		RepairCost:   cost,
		WalletDebit:  debit,
		ShipRepair:   shipRepair,
		Repaired:     true,
	}
	return result, nil, true
}

func (service *RepairService) finishRepairAttempt(
	attemptKey repairAttemptKey,
	input RepairShipInput,
	result RepairShipResult,
	err error,
	cache bool,
) {
	service.mu.Lock()
	defer service.mu.Unlock()

	if cache {
		service.attempts[attemptKey] = repairAttemptRecord{
			input:  input,
			result: cloneRepairShipResult(result),
			err:    err,
		}
	}
	if current, ok := service.inFlight[attemptKey]; ok && sameRepairInput(current.input, input) {
		delete(service.inFlight, attemptKey)
		close(current.done)
	}
}

func (service *RepairService) requireDisabledShip(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	hangar, err := service.ships.GetHangar(playerID)
	if err != nil {
		return err
	}
	for _, playerShip := range hangar.Ships {
		if playerShip.ShipID != shipID {
			continue
		}
		if playerShip.State != ships.ShipStateDisabled {
			return ships.ErrShipNotDisabled
		}
		return nil
	}
	return fmt.Errorf("ship %q: %w", shipID, ships.ErrShipNotUnlocked)
}

func (service *RepairService) previouslyRefundedRepair(input RepairShipInput) bool {
	lookup := economy.CurrencyLedgerReferenceLookup{
		PlayerID:     input.PlayerID,
		Currency:     economy.CurrencyBucketCredits,
		Action:       economy.LedgerActionIncrease,
		Reason:       LedgerReasonShipRepairRefund,
		ReferenceKey: input.ReferenceKey,
	}
	if ledgerLookup, ok := service.wallet.(repairWalletLedgerLookup); ok {
		_, found := ledgerLookup.FindCurrencyLedgerEntry(lookup)
		return found
	}

	reader, ok := service.wallet.(repairWalletLedgerReader)
	if !ok {
		return false
	}
	for _, entry := range reader.CurrencyLedgerEntries() {
		if entry.PlayerID == lookup.PlayerID &&
			entry.Currency == lookup.Currency &&
			entry.Action == lookup.Action &&
			entry.Reason == lookup.Reason &&
			entry.ReferenceKey == lookup.ReferenceKey {
			return true
		}
	}
	return false
}

func (service *RepairService) compensateFailedRestore(
	input RepairShipInput,
	cost int64,
	debit economy.DebitWalletResult,
) (RepairShipResult, error) {
	result := RepairShipResult{
		PlayerID:     input.PlayerID,
		ShipID:       input.ShipID,
		ReferenceKey: input.ReferenceKey,
		Currency:     economy.CurrencyBucketCredits,
		RepairCost:   cost,
		WalletDebit:  debit,
		Compensated:  cost > 0,
	}
	if cost == 0 {
		return result, nil
	}

	refund, err := service.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     input.PlayerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       cost,
		Reason:       LedgerReasonShipRepairRefund,
		ReferenceKey: input.ReferenceKey,
	})
	if err != nil {
		return result, err
	}
	result.WalletRefund = &refund
	return result, nil
}

func (input RepairShipInput) validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ShipID.Validate(); err != nil {
		return err
	}
	repairShipID, err := foundation.ShipRepairShipID(input.ReferenceKey)
	if err != nil {
		return err
	}
	if repairShipID != input.ShipID {
		return fmt.Errorf("reference ship %q input ship %q: %w", repairShipID, input.ShipID, ErrRepairReferenceShipMismatch)
	}
	return nil
}

func validateRepairCostPolicy(repairRateBps int64, locationModifierBps int64) error {
	if repairRateBps <= 0 {
		return fmt.Errorf("repair rate bps %d: %w", repairRateBps, ErrInvalidRepairCostPolicy)
	}
	if locationModifierBps <= 0 {
		return fmt.Errorf("location modifier bps %d: %w", locationModifierBps, ErrInvalidRepairCostPolicy)
	}
	return nil
}

func repairCost(definition ships.ShipDefinition, repairRateBps int64, locationModifierBps int64) (int64, error) {
	if err := definition.Validate(); err != nil {
		return 0, err
	}
	if err := validateRepairCostPolicy(repairRateBps, locationModifierBps); err != nil {
		return 0, err
	}
	if definition.CreditPrice == 0 {
		return 0, nil
	}

	numerator := big.NewInt(definition.CreditPrice)
	numerator.Mul(numerator, big.NewInt(repairRateBps))
	numerator.Mul(numerator, big.NewInt(definition.RepairCostMultiplierBps))
	numerator.Mul(numerator, big.NewInt(locationModifierBps))

	denominator := big.NewInt(repairBpsDenominator)
	denominator.Mul(denominator, big.NewInt(repairBpsDenominator))
	denominator.Mul(denominator, big.NewInt(repairBpsDenominator))

	quotient := new(big.Int).Quo(numerator, denominator)
	if quotient.Sign() == 0 {
		return 1, nil
	}
	if !quotient.IsInt64() || quotient.Int64() > foundation.MaxAmount {
		return 0, ErrRepairCostOverflow
	}
	return quotient.Int64(), nil
}

func sameRepairInput(a RepairShipInput, b RepairShipInput) bool {
	return a.PlayerID == b.PlayerID && a.ShipID == b.ShipID && a.ReferenceKey == b.ReferenceKey
}

func cloneRepairShipResult(result RepairShipResult) RepairShipResult {
	clone := result
	if result.WalletRefund != nil {
		refund := *result.WalletRefund
		clone.WalletRefund = &refund
	}
	return clone
}

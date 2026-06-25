package premium

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func premiumProviderIdempotencyKey(provider ProviderReference) (foundation.IdempotencyKey, error) {
	return foundation.PremiumWebhookIdempotencyKey(providerWalletReference(provider))
}

func premiumClaimIdempotencyKey(input ClaimEntitlementInput) (foundation.IdempotencyKey, error) {
	return foundation.PremiumWebhookIdempotencyKey(
		"claim." + input.EntitlementID.String() + "." + input.PlayerID.String(),
	)
}

func premiumProviderRequestHash(input CreateEntitlementInput) (string, error) {
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"premium_provider_entitlement|provider=%s|player=%s|type=%s|confirmed=%s|payload=%s",
		providerWalletReference(input.Provider),
		input.PlayerID,
		input.Type,
		input.ProviderConfirmedAt.UTC().Format(time.RFC3339Nano),
		payload,
	)))
	return fmt.Sprintf("sha256:%x", hash[:]), nil
}

func premiumClaimRequestHash(input ClaimEntitlementInput) (string, error) {
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"premium_claim|entitlement=%s|player=%s|request=%s",
		input.EntitlementID,
		input.PlayerID,
		input.RequestReference,
	)))
	return fmt.Sprintf("sha256:%x", hash[:]), nil
}

func (service *PremiumEntitlementService) claimPremiumProviderIdempotency(
	entitlement Entitlement,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (economy.IdempotencyKeyRow, CreateEntitlementResult, bool, error) {
	now := service.clock.Now()
	candidate := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   premiumProviderOperation,
		PlayerID:    entitlement.PlayerID,
		RequestHash: requestHash,
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if service.idempotencyStore == nil {
		service.ensurePremiumIdempotencyMapsLocked()
		existing, ok := service.providerIdempotencyRows[referenceKey]
		if !ok {
			if err := candidate.Validate(); err != nil {
				return economy.IdempotencyKeyRow{}, CreateEntitlementResult{}, false, err
			}
			service.providerIdempotencyRows[referenceKey] = candidate.Clone()
			return candidate.Clone(), CreateEntitlementResult{}, false, nil
		}
		return premiumProviderResultFromExisting(existing, candidate)
	}
	claim, err := service.idempotencyStore.ClaimIdempotencyKey(context.Background(), candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, CreateEntitlementResult{}, false, err
	}
	return premiumProviderResultFromClaim(claim)
}

func premiumProviderResultFromExisting(
	existing economy.IdempotencyKeyRow,
	candidate economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, CreateEntitlementResult, bool, error) {
	if existing.Status == economy.IdempotencyStatusCompleted &&
		existing.Scope == candidate.Scope &&
		existing.Key == candidate.Key {
		result, err := premiumProviderResultFromIdempotencyRow(existing)
		if err != nil {
			return economy.IdempotencyKeyRow{}, CreateEntitlementResult{}, false, err
		}
		result.Duplicate = true
		return existing.Clone(), result, true, nil
	}
	claim, err := economy.ResolveIdempotencyClaim(&existing, candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, CreateEntitlementResult{}, false, err
	}
	return premiumProviderResultFromClaim(claim)
}

func premiumProviderResultFromClaim(claim economy.IdempotencyClaimResult) (economy.IdempotencyKeyRow, CreateEntitlementResult, bool, error) {
	if !claim.Duplicate {
		return claim.Row, CreateEntitlementResult{}, false, nil
	}
	switch claim.Row.Status {
	case economy.IdempotencyStatusCompleted:
		result, err := premiumProviderResultFromIdempotencyRow(claim.Row)
		if err != nil {
			return claim.Row, CreateEntitlementResult{}, false, err
		}
		result.Duplicate = true
		return claim.Row, result, true, nil
	case economy.IdempotencyStatusFailed:
		return claim.Row, CreateEntitlementResult{}, false, nil
	default:
		return claim.Row, CreateEntitlementResult{}, false, ErrPremiumProviderInProgress
	}
}

func (service *PremiumEntitlementService) completePremiumProviderIdempotency(row economy.IdempotencyKeyRow, result CreateEntitlementResult) error {
	if row.Key.IsZero() {
		return nil
	}
	payload, err := premiumProviderResultJSON(result)
	if err != nil {
		return err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row); err != nil {
			return err
		}
	}
	service.ensurePremiumIdempotencyMapsLocked()
	if existing, ok := service.providerIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.providerIdempotencyRows[row.Key] = row.Clone()
	return nil
}

func (service *PremiumEntitlementService) failPremiumProviderIdempotency(row economy.IdempotencyKeyRow, cause error) error {
	if row.Key.IsZero() {
		return cause
	}
	payload, err := idempotencyErrorJSON(cause)
	if err != nil {
		return errors.Join(cause, err)
	}
	row.Status = economy.IdempotencyStatusFailed
	row.ResultJSON = payload
	row.UpdatedAt = service.clock.Now()
	row.CompletedAt = time.Time{}
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row); err != nil {
			return errors.Join(cause, err)
		}
	}
	service.ensurePremiumIdempotencyMapsLocked()
	service.providerIdempotencyRows[row.Key] = row.Clone()
	return cause
}

func (service *PremiumEntitlementService) claimPremiumClaimIdempotency(
	input ClaimEntitlementInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (economy.IdempotencyKeyRow, ClaimEntitlementResult, bool, error) {
	now := service.clock.Now()
	candidate := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   premiumClaimOperation,
		PlayerID:    input.PlayerID,
		RequestHash: requestHash,
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if service.idempotencyStore == nil {
		service.ensurePremiumIdempotencyMapsLocked()
		existing, ok := service.claimIdempotencyRows[referenceKey]
		if !ok {
			if err := candidate.Validate(); err != nil {
				return economy.IdempotencyKeyRow{}, ClaimEntitlementResult{}, false, err
			}
			service.claimIdempotencyRows[referenceKey] = candidate.Clone()
			return candidate.Clone(), ClaimEntitlementResult{}, false, nil
		}
		return premiumClaimResultFromExisting(existing, candidate)
	}
	claim, err := service.idempotencyStore.ClaimIdempotencyKey(context.Background(), candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, ClaimEntitlementResult{}, false, err
	}
	return premiumClaimResultFromClaim(claim)
}

func premiumClaimResultFromExisting(
	existing economy.IdempotencyKeyRow,
	candidate economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, ClaimEntitlementResult, bool, error) {
	claim, err := economy.ResolveIdempotencyClaim(&existing, candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, ClaimEntitlementResult{}, false, err
	}
	return premiumClaimResultFromClaim(claim)
}

func premiumClaimResultFromClaim(claim economy.IdempotencyClaimResult) (economy.IdempotencyKeyRow, ClaimEntitlementResult, bool, error) {
	if !claim.Duplicate {
		return claim.Row, ClaimEntitlementResult{}, false, nil
	}
	switch claim.Row.Status {
	case economy.IdempotencyStatusCompleted:
		result, err := premiumClaimResultFromIdempotencyRow(claim.Row)
		if err != nil {
			return claim.Row, ClaimEntitlementResult{}, false, err
		}
		result.Duplicate = true
		return claim.Row, result, true, nil
	case economy.IdempotencyStatusFailed:
		return claim.Row, ClaimEntitlementResult{}, false, nil
	default:
		return claim.Row, ClaimEntitlementResult{}, false, ErrPremiumClaimInProgress
	}
}

func (service *PremiumEntitlementService) completePremiumClaimIdempotency(row economy.IdempotencyKeyRow, result ClaimEntitlementResult) error {
	if row.Key.IsZero() {
		return nil
	}
	payload, err := premiumClaimResultJSON(result)
	if err != nil {
		return err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row); err != nil {
			return err
		}
	}
	service.ensurePremiumIdempotencyMapsLocked()
	if existing, ok := service.claimIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.claimIdempotencyRows[row.Key] = row.Clone()
	return nil
}

func (service *PremiumEntitlementService) failPremiumClaimIdempotency(row economy.IdempotencyKeyRow, cause error) error {
	if row.Key.IsZero() {
		return cause
	}
	payload, err := idempotencyErrorJSON(cause)
	if err != nil {
		return errors.Join(cause, err)
	}
	row.Status = economy.IdempotencyStatusFailed
	row.ResultJSON = payload
	row.UpdatedAt = service.clock.Now()
	row.CompletedAt = time.Time{}
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row); err != nil {
			return errors.Join(cause, err)
		}
	}
	service.ensurePremiumIdempotencyMapsLocked()
	service.claimIdempotencyRows[row.Key] = row.Clone()
	return cause
}

func premiumProviderResultJSON(result CreateEntitlementResult) (json.RawMessage, error) {
	result.Duplicate = false
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func premiumProviderResultFromIdempotencyRow(row economy.IdempotencyKeyRow) (CreateEntitlementResult, error) {
	var result CreateEntitlementResult
	if err := json.Unmarshal(row.ResultJSON, &result); err != nil {
		return CreateEntitlementResult{}, err
	}
	if result.Entitlement.ID.IsZero() {
		return CreateEntitlementResult{}, ErrPremiumProviderResult
	}
	return result, nil
}

type premiumClaimResultRow struct {
	Entitlement                   Entitlement                    `json:"entitlement"`
	WalletCredit                  *premiumWalletCreditResultRow  `json:"wallet_credit,omitempty"`
	LoadoutSlotGrant              *LoadoutSlotGrant              `json:"loadout_slot_grant,omitempty"`
	WeeklyXCorePurchaseRightGrant *WeeklyXCorePurchaseRightGrant `json:"weekly_x_core_purchase_right_grant,omitempty"`
	CosmeticGrant                 *CosmeticGrant                 `json:"cosmetic_grant,omitempty"`
	BadgeGrant                    *BadgeGrant                    `json:"badge_grant,omitempty"`
}

type premiumWalletCreditResultRow struct {
	Balance     economy.WalletBalance         `json:"balance"`
	LedgerEntry premiumCurrencyLedgerEntryRow `json:"ledger_entry"`
}

type premiumCurrencyLedgerEntryRow struct {
	LedgerID     economy.LedgerID          `json:"ledger_id"`
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Currency     economy.CurrencyBucket    `json:"currency_type"`
	Amount       int64                     `json:"amount"`
	Action       economy.LedgerAction      `json:"action"`
	BalanceAfter int64                     `json:"balance_after"`
	Reason       economy.LedgerReason      `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	CreatedAt    time.Time                 `json:"created_at"`
}

func premiumClaimResultJSON(result ClaimEntitlementResult) (json.RawMessage, error) {
	payload, err := json.Marshal(premiumClaimResultRow{
		Entitlement:                   result.Entitlement,
		WalletCredit:                  premiumWalletCreditResultRowFromResult(result.WalletCredit),
		LoadoutSlotGrant:              result.LoadoutSlotGrant,
		WeeklyXCorePurchaseRightGrant: result.WeeklyXCorePurchaseRightGrant,
		CosmeticGrant:                 result.CosmeticGrant,
		BadgeGrant:                    result.BadgeGrant,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func premiumClaimResultFromIdempotencyRow(row economy.IdempotencyKeyRow) (ClaimEntitlementResult, error) {
	var stored premiumClaimResultRow
	if err := json.Unmarshal(row.ResultJSON, &stored); err != nil {
		return ClaimEntitlementResult{}, err
	}
	if stored.Entitlement.ID.IsZero() {
		return ClaimEntitlementResult{}, ErrPremiumClaimResult
	}
	walletCredit, err := premiumWalletCreditResultFromRow(stored.WalletCredit)
	if err != nil {
		return ClaimEntitlementResult{}, err
	}
	return ClaimEntitlementResult{
		Entitlement:                   stored.Entitlement,
		WalletCredit:                  walletCredit,
		LoadoutSlotGrant:              stored.LoadoutSlotGrant,
		WeeklyXCorePurchaseRightGrant: stored.WeeklyXCorePurchaseRightGrant,
		CosmeticGrant:                 stored.CosmeticGrant,
		BadgeGrant:                    stored.BadgeGrant,
	}, nil
}

func premiumWalletCreditResultRowFromResult(result *economy.CreditWalletResult) *premiumWalletCreditResultRow {
	if result == nil {
		return nil
	}
	return &premiumWalletCreditResultRow{
		Balance: result.Balance,
		LedgerEntry: premiumCurrencyLedgerEntryRow{
			LedgerID:     result.LedgerEntry.LedgerID,
			PlayerID:     result.LedgerEntry.PlayerID,
			Currency:     result.LedgerEntry.Currency,
			Amount:       result.LedgerEntry.Amount.Int64(),
			Action:       result.LedgerEntry.Action,
			BalanceAfter: result.LedgerEntry.BalanceAfter,
			Reason:       result.LedgerEntry.Reason,
			ReferenceKey: result.LedgerEntry.ReferenceKey,
			CreatedAt:    result.LedgerEntry.CreatedAt,
		},
	}
}

func premiumWalletCreditResultFromRow(row *premiumWalletCreditResultRow) (*economy.CreditWalletResult, error) {
	if row == nil {
		return nil, nil
	}
	amount, err := foundation.NewMoney(row.LedgerEntry.Amount)
	if err != nil {
		return nil, err
	}
	return &economy.CreditWalletResult{
		Balance: row.Balance,
		LedgerEntry: economy.CurrencyLedgerEntry{
			LedgerID:     row.LedgerEntry.LedgerID,
			PlayerID:     row.LedgerEntry.PlayerID,
			Currency:     row.LedgerEntry.Currency,
			Amount:       amount,
			Action:       row.LedgerEntry.Action,
			BalanceAfter: row.LedgerEntry.BalanceAfter,
			Reason:       row.LedgerEntry.Reason,
			ReferenceKey: row.LedgerEntry.ReferenceKey,
			CreatedAt:    row.LedgerEntry.CreatedAt,
		},
		Duplicate: true,
	}, nil
}

func idempotencyErrorJSON(cause error) (json.RawMessage, error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	payload, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func (service *PremiumEntitlementService) replayProviderEntitlementLocked(entitlement Entitlement) error {
	if err := entitlement.validateCreate(); err != nil {
		return errors.Join(ErrPremiumProviderResult, err)
	}
	service.ensurePremiumIdempotencyMapsLocked()
	key := providerKey(entitlement.Provider)
	if existingID, ok := service.providerReferences[key]; ok && existingID != entitlement.ID {
		return economy.ErrIdempotencyKeyConflict
	}
	if existing, ok := service.entitlements[entitlement.ID]; ok && providerKey(existing.Provider) != key {
		return ErrDuplicateEntitlementID
	}
	service.entitlements[entitlement.ID] = entitlement
	service.providerReferences[key] = entitlement.ID
	return nil
}

type premiumProviderMutationSnapshot struct {
	entitlements            map[EntitlementID]Entitlement
	providerReferences      map[providerReferenceKey]EntitlementID
	providerIdempotencyRows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
}

type premiumClaimMutationSnapshot struct {
	entitlements                   map[EntitlementID]Entitlement
	claimResults                   map[claimReferenceKey]ClaimEntitlementResult
	claimIdempotencyRows           map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	loadoutSlotGrants              []LoadoutSlotGrant
	weeklyXCorePurchaseRightGrants []WeeklyXCorePurchaseRightGrant
	cosmeticGrants                 []CosmeticGrant
	badgeGrants                    []BadgeGrant
	wallet                         economy.WalletMutationSnapshot
}

func (service *PremiumEntitlementService) snapshotPremiumProviderMutationLocked() premiumProviderMutationSnapshot {
	service.ensurePremiumIdempotencyMapsLocked()
	return premiumProviderMutationSnapshot{
		entitlements:            cloneEntitlementMap(service.entitlements),
		providerReferences:      cloneProviderReferenceMap(service.providerReferences),
		providerIdempotencyRows: cloneIdempotencyKeyRowMap(service.providerIdempotencyRows),
	}
}

func (service *PremiumEntitlementService) restorePremiumProviderMutationLocked(snapshot premiumProviderMutationSnapshot) {
	service.entitlements = cloneEntitlementMap(snapshot.entitlements)
	service.providerReferences = cloneProviderReferenceMap(snapshot.providerReferences)
	service.providerIdempotencyRows = cloneIdempotencyKeyRowMap(snapshot.providerIdempotencyRows)
}

func (service *PremiumEntitlementService) snapshotPremiumClaimMutationLocked() premiumClaimMutationSnapshot {
	service.ensurePremiumIdempotencyMapsLocked()
	return premiumClaimMutationSnapshot{
		entitlements:                   cloneEntitlementMap(service.entitlements),
		claimResults:                   cloneClaimResultMap(service.claimResults),
		claimIdempotencyRows:           cloneIdempotencyKeyRowMap(service.claimIdempotencyRows),
		loadoutSlotGrants:              append([]LoadoutSlotGrant(nil), service.loadoutSlotGrants...),
		weeklyXCorePurchaseRightGrants: append([]WeeklyXCorePurchaseRightGrant(nil), service.weeklyXCorePurchaseRightGrants...),
		cosmeticGrants:                 append([]CosmeticGrant(nil), service.cosmeticGrants...),
		badgeGrants:                    append([]BadgeGrant(nil), service.badgeGrants...),
		wallet:                         service.wallet.SnapshotMutationState(),
	}
}

func (service *PremiumEntitlementService) restorePremiumClaimMutationLocked(snapshot premiumClaimMutationSnapshot) {
	service.entitlements = cloneEntitlementMap(snapshot.entitlements)
	service.claimResults = cloneClaimResultMap(snapshot.claimResults)
	service.claimIdempotencyRows = cloneIdempotencyKeyRowMap(snapshot.claimIdempotencyRows)
	service.loadoutSlotGrants = append([]LoadoutSlotGrant(nil), snapshot.loadoutSlotGrants...)
	service.weeklyXCorePurchaseRightGrants = append([]WeeklyXCorePurchaseRightGrant(nil), snapshot.weeklyXCorePurchaseRightGrants...)
	service.cosmeticGrants = append([]CosmeticGrant(nil), snapshot.cosmeticGrants...)
	service.badgeGrants = append([]BadgeGrant(nil), snapshot.badgeGrants...)
	service.wallet.RestoreMutationState(snapshot.wallet)
}

func (service *PremiumEntitlementService) ensurePremiumIdempotencyMapsLocked() {
	if service.providerIdempotencyRows == nil {
		service.providerIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
	}
	if service.claimIdempotencyRows == nil {
		service.claimIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
	}
}

func cloneEntitlementMap(entitlements map[EntitlementID]Entitlement) map[EntitlementID]Entitlement {
	if entitlements == nil {
		return nil
	}
	cloned := make(map[EntitlementID]Entitlement, len(entitlements))
	for id, entitlement := range entitlements {
		cloned[id] = entitlement
	}
	return cloned
}

func cloneProviderReferenceMap(references map[providerReferenceKey]EntitlementID) map[providerReferenceKey]EntitlementID {
	if references == nil {
		return nil
	}
	cloned := make(map[providerReferenceKey]EntitlementID, len(references))
	for key, entitlementID := range references {
		cloned[key] = entitlementID
	}
	return cloned
}

func cloneClaimResultMap(results map[claimReferenceKey]ClaimEntitlementResult) map[claimReferenceKey]ClaimEntitlementResult {
	if results == nil {
		return nil
	}
	cloned := make(map[claimReferenceKey]ClaimEntitlementResult, len(results))
	for key, result := range results {
		cloned[key] = cloneClaimEntitlementResult(result)
	}
	return cloned
}

func cloneIdempotencyKeyRowMap(rows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow) map[foundation.IdempotencyKey]economy.IdempotencyKeyRow {
	if rows == nil {
		return nil
	}
	cloned := make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow, len(rows))
	for key, row := range rows {
		cloned[key] = row.Clone()
	}
	return cloned
}

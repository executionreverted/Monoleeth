package premium

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// EntitlementID identifies a premium entitlement row.
type EntitlementID string

// EntitlementType declares what an entitlement grants when claimed.
type EntitlementType string

const (
	EntitlementTypePremiumCurrencyPack      EntitlementType = "premium_currency_pack"
	EntitlementTypeLoadoutSlot              EntitlementType = "loadout_slot"
	EntitlementTypeWeeklyXCorePurchaseRight EntitlementType = "weekly_x_core_purchase_right"
	EntitlementTypeCosmetic                 EntitlementType = "cosmetic"
	EntitlementTypeBadge                    EntitlementType = "badge"
)

// EntitlementState tracks the claim state machine.
type EntitlementState string

const (
	EntitlementStatePending EntitlementState = "pending"
	EntitlementStateClaimed EntitlementState = "claimed"
	EntitlementStateRevoked EntitlementState = "revoked"
)

// ProviderReference identifies the upstream payment/provider event that
// created an entitlement. Source and Reference are unique together.
type ProviderReference struct {
	Source    string `json:"source"`
	Reference string `json:"provider_reference"`
}

// EntitlementGrantPayload is a typed MVP payload for entitlement grants.
type EntitlementGrantPayload struct {
	CurrencyBucket economy.CurrencyBucket `json:"currency_bucket,omitempty"`
	Amount         int64                  `json:"amount,omitempty"`

	LoadoutSlotScope string `json:"loadout_slot_scope,omitempty"`
	LoadoutSlotCount int64  `json:"loadout_slot_count,omitempty"`

	WorldID   foundation.WorldID `json:"world_id,omitempty"`
	PeriodKey string             `json:"period_key,omitempty"`

	CosmeticID string `json:"cosmetic_id,omitempty"`
	BadgeID    string `json:"badge_id,omitempty"`
}

// Entitlement is a provider-created premium grant waiting to be claimed.
type Entitlement struct {
	ID                  EntitlementID           `json:"entitlement_id"`
	PlayerID            foundation.PlayerID     `json:"player_id"`
	Type                EntitlementType         `json:"type"`
	State               EntitlementState        `json:"state"`
	Provider            ProviderReference       `json:"provider"`
	Payload             EntitlementGrantPayload `json:"payload"`
	CreatedAt           time.Time               `json:"created_at"`
	ProviderConfirmedAt time.Time               `json:"provider_confirmed_at"`
	ClaimedAt           time.Time               `json:"claimed_at,omitempty"`
	ClaimRequestRef     string                  `json:"claim_request_reference,omitempty"`
}

// LoadoutSlotGrant records the skeleton state owned by the premium service.
type LoadoutSlotGrant struct {
	EntitlementID EntitlementID       `json:"entitlement_id"`
	PlayerID      foundation.PlayerID `json:"player_id"`
	Scope         string              `json:"scope"`
	Count         int64               `json:"count"`
	GrantedAt     time.Time           `json:"granted_at"`
}

// WeeklyXCorePurchaseRightGrant records a claimed weekly X Core purchase right.
type WeeklyXCorePurchaseRightGrant struct {
	EntitlementID EntitlementID       `json:"entitlement_id"`
	PlayerID      foundation.PlayerID `json:"player_id"`
	WorldID       foundation.WorldID  `json:"world_id"`
	PeriodKey     string              `json:"period_key"`
	GrantedAt     time.Time           `json:"granted_at"`
}

// CosmeticGrant records a claimed cosmetic skeleton grant.
type CosmeticGrant struct {
	EntitlementID EntitlementID       `json:"entitlement_id"`
	PlayerID      foundation.PlayerID `json:"player_id"`
	CosmeticID    string              `json:"cosmetic_id"`
	GrantedAt     time.Time           `json:"granted_at"`
}

// BadgeGrant records a claimed badge skeleton grant.
type BadgeGrant struct {
	EntitlementID EntitlementID       `json:"entitlement_id"`
	PlayerID      foundation.PlayerID `json:"player_id"`
	BadgeID       string              `json:"badge_id"`
	GrantedAt     time.Time           `json:"granted_at"`
}

// WeeklyXCoreStockRecord tracks one world's stock for one period.
type WeeklyXCoreStockRecord struct {
	WorldID        foundation.WorldID `json:"world_id"`
	PeriodKey      string             `json:"period_key"`
	StockTotal     int64              `json:"stock_total"`
	StockRemaining int64              `json:"stock_remaining"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// WeeklyXCorePurchase records the service-owned skeleton grant from a stock purchase.
type WeeklyXCorePurchase struct {
	PlayerID          foundation.PlayerID    `json:"player_id"`
	WorldID           foundation.WorldID     `json:"world_id"`
	PeriodKey         string                 `json:"period_key"`
	PurchaseReference string                 `json:"purchase_reference"`
	PaymentCurrency   economy.CurrencyBucket `json:"payment_currency"`
	GrantedAt         time.Time              `json:"granted_at"`
}

// SuspiciousTradeLog is a deterministic fraud-review event snapshot.
type SuspiciousTradeLog struct {
	LogID          string                 `json:"log_id"`
	ActorPlayerID  foundation.PlayerID    `json:"actor_player_id"`
	CounterpartyID foundation.PlayerID    `json:"counterparty_player_id"`
	Currency       economy.CurrencyBucket `json:"currency_type"`
	Amount         int64                  `json:"amount"`
	Reason         string                 `json:"reason"`
	Reference      string                 `json:"reference"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ProviderRiskLock records one provider fraud/chargeback lock.
type ProviderRiskLock struct {
	LockID        string              `json:"lock_id"`
	EntitlementID EntitlementID       `json:"entitlement_id"`
	PlayerID      foundation.PlayerID `json:"player_id"`
	Provider      ProviderReference   `json:"provider"`
	Reason        string              `json:"reason"`
	Reference     string              `json:"reference"`
	PreviousState EntitlementState    `json:"previous_state"`
	CurrentState  EntitlementState    `json:"current_state"`
	CreatedAt     time.Time           `json:"created_at"`
}

// String returns the stable entitlement id representation.
func (id EntitlementID) String() string {
	return string(id)
}

// Validate reports whether id is well formed.
func (id EntitlementID) Validate() error {
	return validateToken("entitlement id", string(id), ErrEmptyEntitlementID, ErrInvalidEntitlementID)
}

// IsZero reports whether id is the zero value.
func (id EntitlementID) IsZero() bool {
	return id == ""
}

// String returns the stable type representation.
func (entitlementType EntitlementType) String() string {
	return string(entitlementType)
}

// Validate reports whether entitlementType is supported.
func (entitlementType EntitlementType) Validate() error {
	switch entitlementType {
	case EntitlementTypePremiumCurrencyPack,
		EntitlementTypeLoadoutSlot,
		EntitlementTypeWeeklyXCorePurchaseRight,
		EntitlementTypeCosmetic,
		EntitlementTypeBadge:
		return nil
	default:
		return fmt.Errorf("entitlement type %q: %w", entitlementType, ErrInvalidEntitlementType)
	}
}

// String returns the stable state representation.
func (state EntitlementState) String() string {
	return string(state)
}

// Validate reports whether state is supported.
func (state EntitlementState) Validate() error {
	switch state {
	case EntitlementStatePending, EntitlementStateClaimed, EntitlementStateRevoked:
		return nil
	default:
		return fmt.Errorf("entitlement state %q: %w", state, ErrInvalidEntitlementState)
	}
}

func (provider ProviderReference) validate() error {
	if err := validateToken("provider source", provider.Source, ErrEmptyProviderSource, ErrInvalidProviderSource); err != nil {
		return err
	}
	if err := validateToken("provider reference", provider.Reference, ErrEmptyProviderReference, ErrInvalidProviderReference); err != nil {
		return err
	}
	return nil
}

// Validate reports whether provider is well formed.
func (provider ProviderReference) Validate() error {
	return provider.validate()
}

// ValidateSnapshot reports whether entitlement is a valid persisted service
// snapshot in any supported state.
func (entitlement Entitlement) ValidateSnapshot() error {
	if err := entitlement.ID.Validate(); err != nil {
		return err
	}
	if err := entitlement.PlayerID.Validate(); err != nil {
		return err
	}
	if err := entitlement.Type.Validate(); err != nil {
		return err
	}
	if err := entitlement.State.Validate(); err != nil {
		return err
	}
	if err := entitlement.Provider.validate(); err != nil {
		return err
	}
	if err := entitlement.Payload.validate(entitlement.Type); err != nil {
		return err
	}
	if entitlement.CreatedAt.IsZero() || entitlement.ProviderConfirmedAt.IsZero() {
		return ErrInvalidTimestamp
	}
	if entitlement.ProviderConfirmedAt.After(entitlement.CreatedAt) {
		return fmt.Errorf("provider confirmed at after created at: %w", ErrInvalidTimestamp)
	}
	switch entitlement.State {
	case EntitlementStatePending:
		if !entitlement.ClaimedAt.IsZero() || entitlement.ClaimRequestRef != "" {
			return fmt.Errorf("claimed fields on pending entitlement: %w", ErrInvalidEntitlementState)
		}
	case EntitlementStateClaimed:
		if entitlement.ClaimedAt.IsZero() {
			return fmt.Errorf("claimed_at missing: %w", ErrInvalidEntitlementState)
		}
		if err := validateRequestReference(entitlement.ClaimRequestRef); err != nil {
			return err
		}
	case EntitlementStateRevoked:
		if entitlement.ClaimedAt.IsZero() {
			if entitlement.ClaimRequestRef != "" {
				return fmt.Errorf("claim request without claimed_at: %w", ErrInvalidEntitlementState)
			}
			break
		}
		if err := validateRequestReference(entitlement.ClaimRequestRef); err != nil {
			return err
		}
	}
	return nil
}

func (entitlement Entitlement) validateCreate() error {
	if err := entitlement.ValidateSnapshot(); err != nil {
		return err
	}
	if entitlement.State != EntitlementStatePending {
		return fmt.Errorf("entitlement state %q: %w", entitlement.State, ErrInvalidEntitlementState)
	}
	return nil
}

func (payload EntitlementGrantPayload) validate(entitlementType EntitlementType) error {
	switch entitlementType {
	case EntitlementTypePremiumCurrencyPack:
		if err := validatePremiumGrantCurrency(payload.CurrencyBucket); err != nil {
			return err
		}
		if _, err := foundation.NewMoney(payload.Amount); err != nil {
			return err
		}
	case EntitlementTypeLoadoutSlot:
		if err := validateToken("loadout slot scope", payload.LoadoutSlotScope, ErrInvalidEntitlementGrant, ErrInvalidEntitlementGrant); err != nil {
			return err
		}
		if _, err := foundation.NewQuantity(payload.LoadoutSlotCount); err != nil {
			return err
		}
	case EntitlementTypeWeeklyXCorePurchaseRight:
		if err := payload.WorldID.Validate(); err != nil {
			return err
		}
		if err := validatePeriodKey(payload.PeriodKey); err != nil {
			return err
		}
	case EntitlementTypeCosmetic:
		if err := validateToken("cosmetic id", payload.CosmeticID, ErrInvalidEntitlementGrant, ErrInvalidEntitlementGrant); err != nil {
			return err
		}
	case EntitlementTypeBadge:
		if err := validateToken("badge id", payload.BadgeID, ErrInvalidEntitlementGrant, ErrInvalidEntitlementGrant); err != nil {
			return err
		}
	default:
		return fmt.Errorf("entitlement type %q: %w", entitlementType, ErrInvalidEntitlementType)
	}
	return nil
}

func validatePremiumGrantCurrency(currency economy.CurrencyBucket) error {
	switch currency {
	case economy.CurrencyBucketPremiumPaid,
		economy.CurrencyBucketPremiumEarned,
		economy.CurrencyBucketPremiumMarketAcquired:
		return nil
	default:
		if err := currency.Validate(); err != nil {
			return err
		}
		return fmt.Errorf("currency bucket %q: %w", currency, ErrInvalidEntitlementGrant)
	}
}

func validateRequestReference(reference string) error {
	return validateToken("request reference", reference, ErrEmptyRequestReference, ErrInvalidRequestReference)
}

func validatePurchaseReference(reference string) error {
	return validateToken("purchase reference", reference, ErrEmptyPurchaseReference, ErrInvalidPurchaseReference)
}

func validatePeriodKey(periodKey string) error {
	return validateToken("period key", periodKey, ErrEmptyPeriodKey, ErrInvalidPeriodKey)
}

func validateSuspiciousTradeReason(reason string) error {
	return validateToken("suspicious trade reason", reason, ErrEmptySuspiciousTradeReason, ErrInvalidSuspiciousTradeReason)
}

func validateSuspiciousTradeReference(reference string) error {
	return validateToken("suspicious trade reference", reference, ErrEmptySuspiciousTradeReference, ErrInvalidSuspiciousTradeReference)
}

func validateProviderRiskReason(reason string) error {
	return validateToken("provider risk reason", reason, ErrEmptyProviderRiskReason, ErrInvalidProviderRiskReason)
}

func validateProviderRiskReference(reference string) error {
	return validateToken("provider risk reference", reference, ErrEmptyProviderRiskReference, ErrInvalidProviderRiskReference)
}

func validateToken(kind string, value string, emptyErr error, invalidErr error) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: %w", kind, emptyErr)
	}
	if value != strings.TrimSpace(value) || strings.Contains(value, ":") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s %q: %w", kind, value, invalidErr)
	}
	return nil
}

package observability

// RetentionDisposition is a stable retention handling category for operational evidence.
type RetentionDisposition string

const (
	RetentionDispositionOperationalWindow               RetentionDisposition = "operational_window"
	RetentionDispositionLongTermOrSummarizedArchive     RetentionDisposition = "long_term_or_summarized_archive"
	RetentionDispositionAggregateAfterOperationalWindow RetentionDisposition = "aggregate_after_operational_window"
)

// ProtectedLedger names a value ledger that must remain available for support
// and fraud investigation instead of being silently deleted with short-lived logs.
type ProtectedLedger string

const (
	ProtectedLedgerWallet          ProtectedLedger = "wallet_ledger"
	ProtectedLedgerItem            ProtectedLedger = "item_ledger"
	ProtectedLedgerPremiumPurchase ProtectedLedger = "premium_purchase_ledger"
	ProtectedLedgerAuctionSale     ProtectedLedger = "auction_sale_history"
)

// DataRetentionGuidance records the Phase 12 default retention posture.
type DataRetentionGuidance struct {
	OperationalLogRetentionDays      int                  `json:"operational_log_retention_days"`
	OperationalLogDisposition        RetentionDisposition `json:"operational_log_disposition"`
	EconomySecurityLedgerDisposition RetentionDisposition `json:"economy_security_ledger_disposition"`
	ProtectedValueLedgers            []ProtectedLedger    `json:"protected_value_ledgers"`
	HighVolumeTelemetryDisposition   RetentionDisposition `json:"high_volume_telemetry_disposition"`
}

var requiredProtectedValueLedgers = []ProtectedLedger{
	ProtectedLedgerWallet,
	ProtectedLedgerItem,
	ProtectedLedgerPremiumPurchase,
	ProtectedLedgerAuctionSale,
}

// DefaultDataRetentionGuidance returns the retention posture from the Phase 12 spec.
func DefaultDataRetentionGuidance() DataRetentionGuidance {
	return DataRetentionGuidance{
		OperationalLogRetentionDays:      30,
		OperationalLogDisposition:        RetentionDispositionOperationalWindow,
		EconomySecurityLedgerDisposition: RetentionDispositionLongTermOrSummarizedArchive,
		ProtectedValueLedgers:            RequiredProtectedValueLedgers(),
		HighVolumeTelemetryDisposition:   RetentionDispositionAggregateAfterOperationalWindow,
	}
}

// RequiredProtectedValueLedgers returns ledgers that must survive short log retention.
func RequiredProtectedValueLedgers() []ProtectedLedger {
	return cloneProtectedLedgers(requiredProtectedValueLedgers)
}

// Validate fails closed when guidance would remove money/item evidence too early.
func (guidance DataRetentionGuidance) Validate() error {
	if guidance.OperationalLogRetentionDays <= 0 {
		return ErrInvalidRetentionGuidance
	}
	if guidance.OperationalLogDisposition != RetentionDispositionOperationalWindow {
		return ErrInvalidRetentionGuidance
	}
	if guidance.EconomySecurityLedgerDisposition != RetentionDispositionLongTermOrSummarizedArchive {
		return ErrInvalidRetentionGuidance
	}
	if guidance.HighVolumeTelemetryDisposition != RetentionDispositionAggregateAfterOperationalWindow {
		return ErrInvalidRetentionGuidance
	}
	if !containsAllProtectedLedgers(guidance.ProtectedValueLedgers, requiredProtectedValueLedgers) {
		return ErrInvalidRetentionGuidance
	}
	return nil
}

func containsAllProtectedLedgers(got []ProtectedLedger, required []ProtectedLedger) bool {
	present := make(map[ProtectedLedger]struct{}, len(got))
	for _, ledger := range got {
		present[ledger] = struct{}{}
	}
	for _, ledger := range required {
		if _, ok := present[ledger]; !ok {
			return false
		}
	}
	return true
}

func cloneProtectedLedgers(ledgers []ProtectedLedger) []ProtectedLedger {
	if len(ledgers) == 0 {
		return nil
	}
	cloned := make([]ProtectedLedger, len(ledgers))
	copy(cloned, ledgers)
	return cloned
}

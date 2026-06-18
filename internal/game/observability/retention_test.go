package observability

import (
	"errors"
	"testing"
)

func TestDefaultDataRetentionGuidanceMatchesPhase12Spec(t *testing.T) {
	guidance := DefaultDataRetentionGuidance()
	if err := guidance.Validate(); err != nil {
		t.Fatalf("default retention guidance failed validation: %v", err)
	}

	if guidance.OperationalLogRetentionDays != 30 {
		t.Fatalf("operational log retention days = %d, want 30", guidance.OperationalLogRetentionDays)
	}
	if guidance.OperationalLogDisposition != RetentionDispositionOperationalWindow {
		t.Fatalf("operational log disposition = %q, want %q", guidance.OperationalLogDisposition, RetentionDispositionOperationalWindow)
	}
	if guidance.EconomySecurityLedgerDisposition != RetentionDispositionLongTermOrSummarizedArchive {
		t.Fatalf("ledger disposition = %q, want %q", guidance.EconomySecurityLedgerDisposition, RetentionDispositionLongTermOrSummarizedArchive)
	}
	if guidance.HighVolumeTelemetryDisposition != RetentionDispositionAggregateAfterOperationalWindow {
		t.Fatalf("telemetry disposition = %q, want %q", guidance.HighVolumeTelemetryDisposition, RetentionDispositionAggregateAfterOperationalWindow)
	}
	assertProtectedLedgers(t, guidance.ProtectedValueLedgers, RequiredProtectedValueLedgers())
}

func TestDataRetentionGuidanceRejectsUnsafePolicies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DataRetentionGuidance)
	}{
		{
			name: "non-positive operational window",
			mutate: func(guidance *DataRetentionGuidance) {
				guidance.OperationalLogRetentionDays = 0
			},
		},
		{
			name: "wrong operational log disposition",
			mutate: func(guidance *DataRetentionGuidance) {
				guidance.OperationalLogDisposition = RetentionDispositionLongTermOrSummarizedArchive
			},
		},
		{
			name: "short ledger disposition",
			mutate: func(guidance *DataRetentionGuidance) {
				guidance.EconomySecurityLedgerDisposition = RetentionDispositionOperationalWindow
			},
		},
		{
			name: "missing item ledger protection",
			mutate: func(guidance *DataRetentionGuidance) {
				guidance.ProtectedValueLedgers = []ProtectedLedger{ProtectedLedgerWallet}
			},
		},
		{
			name: "unaggregated high volume telemetry",
			mutate: func(guidance *DataRetentionGuidance) {
				guidance.HighVolumeTelemetryDisposition = RetentionDispositionOperationalWindow
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guidance := DefaultDataRetentionGuidance()
			test.mutate(&guidance)

			if err := guidance.Validate(); !errors.Is(err, ErrInvalidRetentionGuidance) {
				t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidRetentionGuidance)
			}
		})
	}
}

func TestDataRetentionGuidanceSlicesAreCloneSafe(t *testing.T) {
	guidance := DefaultDataRetentionGuidance()
	guidance.ProtectedValueLedgers[0] = ProtectedLedger("mutated")
	if DefaultDataRetentionGuidance().ProtectedValueLedgers[0] != ProtectedLedgerWallet {
		t.Fatal("default retention guidance mutated through returned slice")
	}

	required := RequiredProtectedValueLedgers()
	required[0] = ProtectedLedger("mutated")
	if RequiredProtectedValueLedgers()[0] != ProtectedLedgerWallet {
		t.Fatal("required protected ledgers mutated through returned slice")
	}
}

func assertProtectedLedgers(t *testing.T, got, want []ProtectedLedger) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("protected ledgers length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("protected ledger[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

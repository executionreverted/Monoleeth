package observability

import "testing"

func TestReleaseGateReportFailsClosedWithMissingChecks(t *testing.T) {
	report := NewReleaseGateReport(map[ReleaseGateCheck]bool{
		ReleaseGateUnitTests: true,
		ReleaseGateMetrics:   true,
	})

	if report.Passed {
		t.Fatal("partial release gate report passed")
	}
	assertReleaseGateChecks(t, report.Missing, []ReleaseGateCheck{
		ReleaseGateIntegrationTransactionTests,
		ReleaseGateAbuseTests,
		ReleaseGateAdminInspection,
		ReleaseGateErrorCodes,
		ReleaseGateLedgerReason,
		ReleaseGateLoadTest,
		ReleaseGateGoTestAll,
		ReleaseGateGitDiffCheck,
	})

	nilReport := NewReleaseGateReport(nil)
	if nilReport.Passed {
		t.Fatal("nil release gate report passed")
	}
	assertReleaseGateChecks(t, nilReport.Missing, RequiredReleaseGateChecks())
}

func TestReleaseGateReportPassesWhenComplete(t *testing.T) {
	completed := map[ReleaseGateCheck]bool{}
	for _, check := range RequiredReleaseGateChecks() {
		completed[check] = true
	}

	report := NewReleaseGateReport(completed)
	if !report.Passed {
		t.Fatalf("complete release gate report failed: missing %#v", report.Missing)
	}
	if len(report.Missing) != 0 {
		t.Fatalf("complete release gate report missing %#v", report.Missing)
	}
}

func TestCommandSecurityReviewReportFailsClosedWithMissingChecks(t *testing.T) {
	report := NewCommandSecurityReviewReport(map[CommandSecurityCheck]bool{
		CommandSecurityIntentOnlyPayload:   true,
		CommandSecurityServerPlayerSession: true,
	})

	if report.Passed {
		t.Fatal("partial command security report passed")
	}
	assertCommandSecurityChecks(t, report.Missing, []CommandSecurityCheck{
		CommandSecurityOwnershipChecked,
		CommandSecurityPositiveBoundedAmounts,
		CommandSecurityVisibilityRangeChecked,
		CommandSecurityTransactionLock,
		CommandSecurityLedgerWrite,
		CommandSecurityIdempotency,
		CommandSecurityLeakSafeError,
		CommandSecurityBroadcastAfterCommit,
	})

	nilReport := NewCommandSecurityReviewReport(nil)
	if nilReport.Passed {
		t.Fatal("nil command security report passed")
	}
	assertCommandSecurityChecks(t, nilReport.Missing, RequiredCommandSecurityChecks())
}

func TestCommandSecurityReviewReportPassesWhenComplete(t *testing.T) {
	completed := map[CommandSecurityCheck]bool{}
	for _, check := range RequiredCommandSecurityChecks() {
		completed[check] = true
	}

	report := NewCommandSecurityReviewReport(completed)
	if !report.Passed {
		t.Fatalf("complete command security report failed: missing %#v", report.Missing)
	}
	if len(report.Missing) != 0 {
		t.Fatalf("complete command security report missing %#v", report.Missing)
	}
}

func TestRequiredCheckSlicesAreCloneSafe(t *testing.T) {
	releaseChecks := RequiredReleaseGateChecks()
	releaseChecks[0] = ReleaseGateCheck("mutated")
	if RequiredReleaseGateChecks()[0] != ReleaseGateUnitTests {
		t.Fatal("release gate checks mutated through returned slice")
	}

	securityChecks := RequiredCommandSecurityChecks()
	securityChecks[0] = CommandSecurityCheck("mutated")
	if RequiredCommandSecurityChecks()[0] != CommandSecurityIntentOnlyPayload {
		t.Fatal("command security checks mutated through returned slice")
	}
}

func assertReleaseGateChecks(t *testing.T, got, want []ReleaseGateCheck) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("release checks length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("release check[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func assertCommandSecurityChecks(t *testing.T, got, want []CommandSecurityCheck) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("security checks length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("security check[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

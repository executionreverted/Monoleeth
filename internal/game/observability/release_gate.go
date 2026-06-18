package observability

// ReleaseGateCheck is a stable production-readiness gate name.
type ReleaseGateCheck string

const (
	ReleaseGateUnitTests                   ReleaseGateCheck = "unit_tests"
	ReleaseGateIntegrationTransactionTests ReleaseGateCheck = "integration_transaction_tests"
	ReleaseGateAbuseTests                  ReleaseGateCheck = "abuse_tests"
	ReleaseGateMetrics                     ReleaseGateCheck = "metrics"
	ReleaseGateAdminInspection             ReleaseGateCheck = "admin_inspection"
	ReleaseGateErrorCodes                  ReleaseGateCheck = "error_codes"
	ReleaseGateLedgerReason                ReleaseGateCheck = "ledger_reason"
	ReleaseGateLoadTest                    ReleaseGateCheck = "load_test"
	ReleaseGateGoTestAll                   ReleaseGateCheck = "go_test_all"
	ReleaseGateGitDiffCheck                ReleaseGateCheck = "git_diff_check"
)

// CommandSecurityCheck is a stable command security review item.
type CommandSecurityCheck string

const (
	CommandSecurityIntentOnlyPayload      CommandSecurityCheck = "intent_only_payload"
	CommandSecurityServerPlayerSession    CommandSecurityCheck = "server_player_session"
	CommandSecurityOwnershipChecked       CommandSecurityCheck = "ownership_checked"
	CommandSecurityPositiveBoundedAmounts CommandSecurityCheck = "positive_bounded_amounts"
	CommandSecurityVisibilityRangeChecked CommandSecurityCheck = "visibility_range_checked"
	CommandSecurityTransactionLock        CommandSecurityCheck = "transaction_lock"
	CommandSecurityLedgerWrite            CommandSecurityCheck = "ledger_write"
	CommandSecurityIdempotency            CommandSecurityCheck = "idempotency"
	CommandSecurityLeakSafeError          CommandSecurityCheck = "leak_safe_error"
	CommandSecurityBroadcastAfterCommit   CommandSecurityCheck = "broadcast_after_commit"
)

// ReleaseGateReport reports whether all required Phase 12 release gates passed.
type ReleaseGateReport struct {
	Passed  bool               `json:"passed"`
	Missing []ReleaseGateCheck `json:"missing,omitempty"`
}

// CommandSecurityReviewReport reports whether one command passed security review.
type CommandSecurityReviewReport struct {
	Passed  bool                   `json:"passed"`
	Missing []CommandSecurityCheck `json:"missing,omitempty"`
}

var requiredReleaseGateChecks = []ReleaseGateCheck{
	ReleaseGateUnitTests,
	ReleaseGateIntegrationTransactionTests,
	ReleaseGateAbuseTests,
	ReleaseGateMetrics,
	ReleaseGateAdminInspection,
	ReleaseGateErrorCodes,
	ReleaseGateLedgerReason,
	ReleaseGateLoadTest,
	ReleaseGateGoTestAll,
	ReleaseGateGitDiffCheck,
}

var requiredCommandSecurityChecks = []CommandSecurityCheck{
	CommandSecurityIntentOnlyPayload,
	CommandSecurityServerPlayerSession,
	CommandSecurityOwnershipChecked,
	CommandSecurityPositiveBoundedAmounts,
	CommandSecurityVisibilityRangeChecked,
	CommandSecurityTransactionLock,
	CommandSecurityLedgerWrite,
	CommandSecurityIdempotency,
	CommandSecurityLeakSafeError,
	CommandSecurityBroadcastAfterCommit,
}

// RequiredReleaseGateChecks returns the Phase 12 release gates in stable order.
func RequiredReleaseGateChecks() []ReleaseGateCheck {
	return cloneReleaseGateChecks(requiredReleaseGateChecks)
}

// NewReleaseGateReport fails closed unless every required gate is true.
func NewReleaseGateReport(completed map[ReleaseGateCheck]bool) ReleaseGateReport {
	missing := make([]ReleaseGateCheck, 0)
	for _, check := range requiredReleaseGateChecks {
		if !completed[check] {
			missing = append(missing, check)
		}
	}
	return ReleaseGateReport{
		Passed:  len(missing) == 0,
		Missing: cloneReleaseGateChecks(missing),
	}
}

// RequiredCommandSecurityChecks returns the command security checklist in stable order.
func RequiredCommandSecurityChecks() []CommandSecurityCheck {
	return cloneCommandSecurityChecks(requiredCommandSecurityChecks)
}

// NewCommandSecurityReviewReport fails closed unless every required check is true.
func NewCommandSecurityReviewReport(completed map[CommandSecurityCheck]bool) CommandSecurityReviewReport {
	missing := make([]CommandSecurityCheck, 0)
	for _, check := range requiredCommandSecurityChecks {
		if !completed[check] {
			missing = append(missing, check)
		}
	}
	return CommandSecurityReviewReport{
		Passed:  len(missing) == 0,
		Missing: cloneCommandSecurityChecks(missing),
	}
}

func cloneReleaseGateChecks(checks []ReleaseGateCheck) []ReleaseGateCheck {
	if len(checks) == 0 {
		return nil
	}
	cloned := make([]ReleaseGateCheck, len(checks))
	copy(cloned, checks)
	return cloned
}

func cloneCommandSecurityChecks(checks []CommandSecurityCheck) []CommandSecurityCheck {
	if len(checks) == 0 {
		return nil
	}
	cloned := make([]CommandSecurityCheck, len(checks))
	copy(cloned, checks)
	return cloned
}

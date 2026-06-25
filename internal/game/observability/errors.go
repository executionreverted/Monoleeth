package observability

import "errors"

var (
	// ErrBlankOperation reports a missing gameplay command operation name.
	ErrBlankOperation = errors.New("blank operation")

	// ErrBlankMetricName reports a missing metric name.
	ErrBlankMetricName = errors.New("blank metric name")

	// ErrUnsafeMetricName reports a metric name outside the safe stable-name alphabet.
	ErrUnsafeMetricName = errors.New("unsafe metric name")

	// ErrInvalidDuration reports a negative duration.
	ErrInvalidDuration = errors.New("invalid duration")

	// ErrNegativeMetricValue reports a counter or gauge value below zero.
	ErrNegativeMetricValue = errors.New("negative metric value")

	// ErrInvalidTelemetryErrorReason reports an unsupported telemetry error bucket.
	ErrInvalidTelemetryErrorReason = errors.New("invalid telemetry error reason")

	// ErrUnsafeLabelName reports a blank or unsafe metric label name.
	ErrUnsafeLabelName = errors.New("unsafe label name")

	// ErrUnsafeLabelValue reports a blank or unsafe metric label value.
	ErrUnsafeLabelValue = errors.New("unsafe label value")

	// ErrMissingCommandLogIdentity reports a missing required command identity field.
	ErrMissingCommandLogIdentity = errors.New("missing command log identity field")

	// ErrBlankCommandStatus reports a missing command outcome status.
	ErrBlankCommandStatus = errors.New("blank command status")

	// ErrMissingCommandLogTimestamp reports a missing command log timestamp.
	ErrMissingCommandLogTimestamp = errors.New("missing command log timestamp")

	// ErrMissingCommandLogWriter reports a nil structured command log sink.
	ErrMissingCommandLogWriter = errors.New("missing command log writer")

	// ErrMissingAuthTransitionLogIdentity reports a missing success auth log identity field.
	ErrMissingAuthTransitionLogIdentity = errors.New("missing auth transition log identity field")

	// ErrMissingAuthTransitionLogTimestamp reports a missing auth transition log timestamp.
	ErrMissingAuthTransitionLogTimestamp = errors.New("missing auth transition log timestamp")

	// ErrMissingAuthTransitionLogWriter reports a nil structured auth transition log sink.
	ErrMissingAuthTransitionLogWriter = errors.New("missing auth transition log writer")

	// ErrMissingSettlementLogIdentity reports a missing required settlement log identity field.
	ErrMissingSettlementLogIdentity = errors.New("missing settlement log identity field")

	// ErrMissingSettlementLogTimestamp reports a missing settlement log timestamp.
	ErrMissingSettlementLogTimestamp = errors.New("missing settlement log timestamp")

	// ErrMissingSettlementLogWriter reports a nil structured settlement log sink.
	ErrMissingSettlementLogWriter = errors.New("missing settlement log writer")

	// ErrMissingPremiumTransitionLogIdentity reports a missing required premium transition log identity field.
	ErrMissingPremiumTransitionLogIdentity = errors.New("missing premium transition log identity field")

	// ErrMissingPremiumTransitionLogTimestamp reports a missing premium transition log timestamp.
	ErrMissingPremiumTransitionLogTimestamp = errors.New("missing premium transition log timestamp")

	// ErrMissingPremiumTransitionLogWriter reports a nil structured premium transition log sink.
	ErrMissingPremiumTransitionLogWriter = errors.New("missing premium transition log writer")

	// ErrUnsafePremiumTransitionLogReference reports an unsafe optional premium transition reference.
	ErrUnsafePremiumTransitionLogReference = errors.New("unsafe premium transition log reference")

	// ErrInvalidValueFlowDirection reports an unsupported economy flow direction.
	ErrInvalidValueFlowDirection = errors.New("invalid value flow direction")

	// ErrInvalidEconomyFlowValueKind reports an unsupported economy flow value kind.
	ErrInvalidEconomyFlowValueKind = errors.New("invalid economy flow value kind")

	// ErrMissingEconomyFlowValueIdentity reports a currency/item flow with no value identity.
	ErrMissingEconomyFlowValueIdentity = errors.New("missing economy flow value identity")

	// ErrAmbiguousEconomyFlowValueIdentity reports a flow that mixes currency and item identity.
	ErrAmbiguousEconomyFlowValueIdentity = errors.New("ambiguous economy flow value identity")

	// ErrMissingEconomyFlowReason reports a missing stable ledger reason.
	ErrMissingEconomyFlowReason = errors.New("missing economy flow reason")

	// ErrMissingEconomyFlowReference reports a missing domain idempotency reference.
	ErrMissingEconomyFlowReference = errors.New("missing economy flow reference")

	// ErrMissingEconomyFlowTimestamp reports a missing economy flow timestamp.
	ErrMissingEconomyFlowTimestamp = errors.New("missing economy flow timestamp")

	// ErrDuplicateEconomyFlowReference reports a duplicate value-flow reference.
	ErrDuplicateEconomyFlowReference = errors.New("duplicate economy flow reference")

	// ErrInvalidRetentionGuidance reports retention guidance that would drop support or fraud evidence too early.
	ErrInvalidRetentionGuidance = errors.New("invalid retention guidance")
)

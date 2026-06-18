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
)

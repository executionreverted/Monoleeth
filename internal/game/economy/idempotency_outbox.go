package economy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

const (
	IdempotencyScopeEconomy = "economy"

	IdempotencyStatusInProgress IdempotencyStatus = "in_progress"
	IdempotencyStatusCompleted  IdempotencyStatus = "completed"
	IdempotencyStatusFailed     IdempotencyStatus = "failed"

	OutboxStatusPending   OutboxStatus = "pending"
	OutboxStatusLeased    OutboxStatus = "leased"
	OutboxStatusPublished OutboxStatus = "published"
	OutboxStatusFailed    OutboxStatus = "failed"
	OutboxStatusDead      OutboxStatus = "dead"
)

var (
	ErrIdempotencyKeyConflict = errors.New("idempotency key conflict")
	ErrInvalidIdempotencyRow  = errors.New("invalid idempotency row")
	ErrInvalidOutboxRow       = errors.New("invalid outbox row")
)

type IdempotencyStatus string

func (status IdempotencyStatus) Validate() error {
	switch status {
	case IdempotencyStatusInProgress, IdempotencyStatusCompleted, IdempotencyStatusFailed:
		return nil
	default:
		return fmt.Errorf("idempotency status %q: %w", status, ErrInvalidIdempotencyRow)
	}
}

// IdempotencyKeyRow is the durable row contract for one economy domain mutation.
type IdempotencyKeyRow struct {
	Scope       string
	Key         foundation.IdempotencyKey
	Operation   string
	PlayerID    foundation.PlayerID
	RequestHash string
	Status      IdempotencyStatus
	ResultJSON  json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

func (row IdempotencyKeyRow) Validate() error {
	if strings.TrimSpace(row.Scope) == "" || row.Scope != strings.TrimSpace(row.Scope) {
		return fmt.Errorf("scope %q: %w", row.Scope, ErrInvalidIdempotencyRow)
	}
	if err := row.Key.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(row.Operation) == "" || row.Operation != strings.TrimSpace(row.Operation) {
		return fmt.Errorf("operation %q: %w", row.Operation, ErrInvalidIdempotencyRow)
	}
	if !row.PlayerID.IsZero() {
		if err := row.PlayerID.Validate(); err != nil {
			return err
		}
	}
	if err := row.Status.Validate(); err != nil {
		return err
	}
	if row.Status == IdempotencyStatusCompleted && row.CompletedAt.IsZero() {
		return fmt.Errorf("completed_at: %w", ErrInvalidIdempotencyRow)
	}
	if err := validateJSONObject("result_json", row.ResultJSON); err != nil {
		return err
	}
	return nil
}

func (row IdempotencyKeyRow) Clone() IdempotencyKeyRow {
	row.ResultJSON = cloneRawJSON(row.ResultJSON)
	return row
}

type IdempotencyClaimResult struct {
	Row       IdempotencyKeyRow
	Duplicate bool
}

// ResolveIdempotencyClaim returns a stable duplicate row or rejects conflicting
// reuse before any caller mutates value state.
func ResolveIdempotencyClaim(existing *IdempotencyKeyRow, candidate IdempotencyKeyRow) (IdempotencyClaimResult, error) {
	if err := candidate.Validate(); err != nil {
		return IdempotencyClaimResult{}, err
	}
	if existing == nil {
		return IdempotencyClaimResult{Row: candidate.Clone()}, nil
	}
	if err := existing.Validate(); err != nil {
		return IdempotencyClaimResult{}, err
	}
	if !sameIdempotencyClaim(*existing, candidate) {
		return IdempotencyClaimResult{}, ErrIdempotencyKeyConflict
	}
	return IdempotencyClaimResult{Row: existing.Clone(), Duplicate: true}, nil
}

func sameIdempotencyClaim(left IdempotencyKeyRow, right IdempotencyKeyRow) bool {
	return left.Scope == right.Scope &&
		left.Key == right.Key &&
		left.Operation == right.Operation &&
		left.PlayerID == right.PlayerID &&
		left.RequestHash == right.RequestHash
}

type OutboxStatus string

func (status OutboxStatus) Validate() error {
	switch status {
	case OutboxStatusPending, OutboxStatusLeased, OutboxStatusPublished, OutboxStatusFailed, OutboxStatusDead:
		return nil
	default:
		return fmt.Errorf("outbox status %q: %w", status, ErrInvalidOutboxRow)
	}
}

// OutboxRow is the durable event row contract for later after-commit publishers.
type OutboxRow struct {
	OutboxID         string
	Topic            string
	EventType        string
	AggregateType    string
	AggregateID      string
	IdempotencyScope string
	IdempotencyKey   foundation.IdempotencyKey
	PayloadJSON      json.RawMessage
	Status           OutboxStatus
	AvailableAt      time.Time
	LeaseOwner       string
	LeasedUntil      time.Time
	AttemptCount     int
	MaxAttempts      int
	LastError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	PublishedAt      time.Time
}

func NewOutboxRow(row OutboxRow) (OutboxRow, error) {
	if row.Status == "" {
		row.Status = OutboxStatusPending
	}
	if row.MaxAttempts == 0 {
		row.MaxAttempts = 20
	}
	if row.AvailableAt.IsZero() {
		row.AvailableAt = row.CreatedAt
	}
	row.PayloadJSON = cloneRawJSON(row.PayloadJSON)
	if err := row.Validate(); err != nil {
		return OutboxRow{}, err
	}
	return row, nil
}

func (row OutboxRow) Validate() error {
	if strings.TrimSpace(row.OutboxID) == "" || row.OutboxID != strings.TrimSpace(row.OutboxID) {
		return fmt.Errorf("outbox_id %q: %w", row.OutboxID, ErrInvalidOutboxRow)
	}
	if strings.TrimSpace(row.Topic) == "" || row.Topic != strings.TrimSpace(row.Topic) {
		return fmt.Errorf("topic %q: %w", row.Topic, ErrInvalidOutboxRow)
	}
	if strings.TrimSpace(row.EventType) == "" || row.EventType != strings.TrimSpace(row.EventType) {
		return fmt.Errorf("event_type %q: %w", row.EventType, ErrInvalidOutboxRow)
	}
	if row.IdempotencyScope != "" && row.IdempotencyScope != strings.TrimSpace(row.IdempotencyScope) {
		return fmt.Errorf("idempotency_scope %q: %w", row.IdempotencyScope, ErrInvalidOutboxRow)
	}
	if !row.IdempotencyKey.IsZero() {
		if err := row.IdempotencyKey.Validate(); err != nil {
			return err
		}
	}
	if err := validateJSONObject("payload_json", row.PayloadJSON); err != nil {
		return err
	}
	if err := row.Status.Validate(); err != nil {
		return err
	}
	if row.Status == OutboxStatusLeased && (strings.TrimSpace(row.LeaseOwner) == "" || row.LeasedUntil.IsZero()) {
		return fmt.Errorf("lease: %w", ErrInvalidOutboxRow)
	}
	if row.Status == OutboxStatusPublished && row.PublishedAt.IsZero() {
		return fmt.Errorf("published_at: %w", ErrInvalidOutboxRow)
	}
	if row.AttemptCount < 0 || row.MaxAttempts <= 0 {
		return fmt.Errorf("attempts: %w", ErrInvalidOutboxRow)
	}
	return nil
}

func (row OutboxRow) Clone() OutboxRow {
	row.PayloadJSON = cloneRawJSON(row.PayloadJSON)
	return row
}

type IdempotencyStore interface {
	ClaimIdempotencyKey(ctx context.Context, row IdempotencyKeyRow) (IdempotencyClaimResult, error)
	CompleteIdempotencyKey(ctx context.Context, row IdempotencyKeyRow) (IdempotencyKeyRow, error)
}

type OutboxStore interface {
	InsertOutboxRow(ctx context.Context, row OutboxRow) error
	LoadOutboxRow(ctx context.Context, outboxID string) (OutboxRow, bool, error)
}

func validateJSONObject(field string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s: %w", field, ErrInvalidOutboxRow)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if err := decoder.Decode(&value); err != io.EOF {
		return fmt.Errorf("%s: %w", field, ErrInvalidOutboxRow)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("%s: %w", field, ErrInvalidOutboxRow)
	}
	return nil
}

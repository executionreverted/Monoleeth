package content

import (
	"encoding/json"
	"time"
)

const (
	DefaultAuditLogLimit = 50
	MaxAuditLogLimit     = 200
)

type PublishDraftInput struct {
	Version        string
	Notes          string
	BalanceTag     string
	ActorAccountID string
}

type RollbackInput struct {
	TargetVersionID string
	Version         string
	Notes           string
	BalanceTag      string
	ActorAccountID  string
	IdempotencyKey  string
}

type PublishDraftResult struct {
	Published      bool
	Version        VersionSummary
	Validation     DraftValidationReport
	RowCount       int
	Idempotent     bool
	IdempotencyKey string
}

type SnapshotVersionRecord struct {
	ID                   string
	Version              string
	Status               string
	Current              bool
	Snapshot             Snapshot
	ValidationReportJSON json.RawMessage
	Notes                string
	BalanceTag           string
	CreatedBy            string
	CreatedAt            time.Time
	PublishedBy          string
	PublishedAt          time.Time
	RolledBackFrom       string
}

type PublishSnapshotInput struct {
	ID                   string
	Version              string
	Snapshot             Snapshot
	ValidationReportJSON json.RawMessage
	IdempotencyKey       string
	ExpectedCurrentID    string
	Notes                string
	BalanceTag           string
	CreatedBy            string
	PublishedBy          string
	PublishedAt          time.Time
	RolledBackFrom       string
	AuditEntries         []AuditLogEntryInput
}

type PublishSnapshotResult struct {
	Record     SnapshotVersionRecord
	Idempotent bool
}

type AuditLogInput struct {
	VersionID   string
	ContentType ContentType
	ContentID   ContentID
	Limit       int
	Offset      int
}

type AuditLogEntryInput struct {
	ID               string
	ContentVersionID string
	ContentType      ContentType
	ContentID        ContentID
	FieldPath        string
	OldValueJSON     json.RawMessage
	NewValueJSON     json.RawMessage
	ActorAccountID   string
	Note             string
	BalanceTag       string
}

type AuditLogEntry struct {
	ID               string
	ContentVersionID string
	ContentType      ContentType
	ContentID        ContentID
	FieldPath        string
	OldValueJSON     json.RawMessage
	NewValueJSON     json.RawMessage
	ActorAccountID   string
	Note             string
	BalanceTag       string
	CreatedAt        time.Time
}

type AuditLog struct {
	Entries     []AuditLogEntry
	Total       int
	Limit       int
	Offset      int
	GeneratedAt time.Time
}

func NormalizeAuditLogInput(input AuditLogInput) AuditLogInput {
	if input.Limit <= 0 {
		input.Limit = DefaultAuditLogLimit
	}
	if input.Limit > MaxAuditLogLimit {
		input.Limit = MaxAuditLogLimit
	}
	if input.Offset < 0 {
		input.Offset = 0
	}
	return input
}

func VersionSummaryFromRecord(record SnapshotVersionRecord) VersionSummary {
	return VersionSummary{
		ID:             record.ID,
		Version:        record.Version,
		Status:         record.Status,
		Current:        record.Current,
		Notes:          record.Notes,
		BalanceTag:     record.BalanceTag,
		CreatedBy:      record.CreatedBy,
		CreatedAt:      record.CreatedAt,
		PublishedBy:    record.PublishedBy,
		PublishedAt:    record.PublishedAt,
		RolledBackFrom: record.RolledBackFrom,
	}
}

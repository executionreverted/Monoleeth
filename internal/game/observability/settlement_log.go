package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// SettlementLogEntry is the safe structured log shape for domain settlements.
// It deliberately omits request payloads, ledger payloads, and internal causes.
type SettlementLogEntry struct {
	RequestID      foundation.RequestID        `json:"request_id"`
	PlayerID       foundation.PlayerID         `json:"player_id"`
	Operation      Operation                   `json:"op"`
	ErrorCode      foundation.Code             `json:"error_code"`
	IdempotencyKey foundation.IdempotencyKey   `json:"idempotency_key"`
	ReferenceIDs   []foundation.IdempotencyKey `json:"-"`
	Duration       time.Duration               `json:"-"`
	Status         CommandStatus               `json:"result"`
	Timestamp      time.Time                   `json:"timestamp"`
}

// SettlementLogger records domain settlement transition logs.
type SettlementLogger interface {
	RecordSettlement(SettlementLogEntry) error
}

// MemorySettlementLogger stores validated settlement log entries in memory.
type MemorySettlementLogger struct {
	mu      sync.Mutex
	entries []SettlementLogEntry
}

// JSONSettlementLogger writes one safe structured settlement log line.
type JSONSettlementLogger struct {
	mu     sync.Mutex
	writer io.Writer
}

// Validate reports whether entry has required safe settlement fields.
func (entry SettlementLogEntry) Validate() error {
	if entry.RequestID.IsZero() {
		return fmt.Errorf("request_id: %w", ErrMissingSettlementLogIdentity)
	}
	if err := entry.RequestID.Validate(); err != nil {
		return err
	}
	if entry.PlayerID.IsZero() {
		return fmt.Errorf("player_id: %w", ErrMissingSettlementLogIdentity)
	}
	if err := entry.PlayerID.Validate(); err != nil {
		return err
	}
	if err := entry.Operation.Validate(); err != nil {
		return err
	}
	if err := entry.Status.Validate(); err != nil {
		return err
	}
	if entry.IdempotencyKey.IsZero() {
		return fmt.Errorf("idempotency_key: %w", ErrMissingSettlementLogIdentity)
	}
	if err := entry.IdempotencyKey.Validate(); err != nil {
		return err
	}
	for _, referenceID := range entry.ReferenceIDs {
		if referenceID.IsZero() {
			continue
		}
		if err := referenceID.Validate(); err != nil {
			return err
		}
	}
	if entry.Duration < 0 {
		return fmt.Errorf("duration %s: %w", entry.Duration, ErrInvalidDuration)
	}
	if entry.Timestamp.IsZero() {
		return ErrMissingSettlementLogTimestamp
	}
	return nil
}

// MarshalJSON encodes only safe settlement log fields.
func (entry SettlementLogEntry) MarshalJSON() ([]byte, error) {
	type settlementLogJSON struct {
		RequestID      foundation.RequestID        `json:"request_id"`
		PlayerID       foundation.PlayerID         `json:"player_id"`
		Operation      Operation                   `json:"op"`
		Result         CommandStatus               `json:"result"`
		ErrorCode      foundation.Code             `json:"error_code"`
		IdempotencyKey foundation.IdempotencyKey   `json:"idempotency_key"`
		RefIDs         []foundation.IdempotencyKey `json:"ref_ids"`
		DurationMS     int64                       `json:"duration_ms"`
		Timestamp      time.Time                   `json:"timestamp"`
	}

	return json.Marshal(settlementLogJSON{
		RequestID:      entry.RequestID,
		PlayerID:       entry.PlayerID,
		Operation:      entry.Operation,
		Result:         entry.Status,
		ErrorCode:      entry.ErrorCode,
		IdempotencyKey: entry.IdempotencyKey,
		RefIDs:         settlementLogReferenceIDs(entry.IdempotencyKey, entry.ReferenceIDs),
		DurationMS:     entry.Duration.Milliseconds(),
		Timestamp:      entry.Timestamp,
	})
}

// NewMemorySettlementLogger returns an empty in-memory settlement logger.
func NewMemorySettlementLogger() *MemorySettlementLogger {
	return &MemorySettlementLogger{}
}

// NewJSONSettlementLogger returns a structured JSON-line settlement logger.
func NewJSONSettlementLogger(writer io.Writer) (*JSONSettlementLogger, error) {
	if writer == nil {
		return nil, ErrMissingSettlementLogWriter
	}
	return &JSONSettlementLogger{writer: writer}, nil
}

// RecordSettlement validates and stores a cloned entry.
func (logger *MemorySettlementLogger) RecordSettlement(entry SettlementLogEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()

	logger.entries = append(logger.entries, cloneSettlementLogEntry(entry))
	return nil
}

// RecordSettlement validates and writes one JSON line.
func (logger *JSONSettlementLogger) RecordSettlement(entry SettlementLogEntry) error {
	if logger == nil || logger.writer == nil {
		return ErrMissingSettlementLogWriter
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()

	if _, err := logger.writer.Write(payload); err != nil {
		return err
	}
	_, err = logger.writer.Write([]byte("\n"))
	return err
}

// Snapshot returns a deterministic clone of stored entries.
func (logger *MemorySettlementLogger) Snapshot() []SettlementLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	entries := make([]SettlementLogEntry, len(logger.entries))
	for i, entry := range logger.entries {
		entries[i] = cloneSettlementLogEntry(entry)
	}
	sortSettlementLogEntries(entries)
	return entries
}

func cloneSettlementLogEntry(entry SettlementLogEntry) SettlementLogEntry {
	entry.ReferenceIDs = append([]foundation.IdempotencyKey(nil), entry.ReferenceIDs...)
	return entry
}

func sortSettlementLogEntries(entries []SettlementLogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if !left.Timestamp.Equal(right.Timestamp) {
			return left.Timestamp.Before(right.Timestamp)
		}
		if left.Operation != right.Operation {
			return left.Operation < right.Operation
		}
		if left.RequestID != right.RequestID {
			return left.RequestID < right.RequestID
		}
		return left.PlayerID < right.PlayerID
	})
}

func settlementLogReferenceIDs(
	idempotencyKey foundation.IdempotencyKey,
	referenceIDs []foundation.IdempotencyKey,
) []foundation.IdempotencyKey {
	seen := make(map[foundation.IdempotencyKey]struct{}, 1+len(referenceIDs))
	result := make([]foundation.IdempotencyKey, 0, 1+len(referenceIDs))
	for _, referenceID := range append([]foundation.IdempotencyKey{idempotencyKey}, referenceIDs...) {
		if referenceID.IsZero() {
			continue
		}
		if _, ok := seen[referenceID]; ok {
			continue
		}
		seen[referenceID] = struct{}{}
		result = append(result, referenceID)
	}
	return result
}

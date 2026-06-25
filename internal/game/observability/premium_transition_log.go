package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"gameproject/internal/game/foundation"
)

// PremiumTransitionLogEntry is the safe structured log shape for premium
// provider and claim transitions. It omits request bodies and provider secrets.
type PremiumTransitionLogEntry struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	RequestRef     string                    `json:"request_id,omitempty"`
	Operation      Operation                 `json:"op"`
	ErrorCode      foundation.Code           `json:"error_code"`
	IdempotencyKey foundation.IdempotencyKey `json:"idempotency_key,omitempty"`
	ReferenceIDs   []string                  `json:"-"`
	ProviderRefIDs []string                  `json:"-"`
	Duration       time.Duration             `json:"-"`
	Status         CommandStatus             `json:"result"`
	Timestamp      time.Time                 `json:"timestamp"`
}

// PremiumTransitionLogger records premium transition logs.
type PremiumTransitionLogger interface {
	RecordPremiumTransition(PremiumTransitionLogEntry) error
}

// MemoryPremiumTransitionLogger stores validated premium transition log entries in memory.
type MemoryPremiumTransitionLogger struct {
	mu      sync.Mutex
	entries []PremiumTransitionLogEntry
}

// JSONPremiumTransitionLogger writes one safe structured premium transition log line.
type JSONPremiumTransitionLogger struct {
	mu     sync.Mutex
	writer io.Writer
}

// Validate reports whether entry has required safe premium transition fields.
func (entry PremiumTransitionLogEntry) Validate() error {
	if entry.PlayerID.IsZero() {
		return fmt.Errorf("player_id: %w", ErrMissingPremiumTransitionLogIdentity)
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
	if entry.RequestRef != "" && !safePremiumTransitionReference(entry.RequestRef) {
		return fmt.Errorf("request_id: %w", ErrUnsafePremiumTransitionLogReference)
	}
	if !entry.IdempotencyKey.IsZero() {
		if err := entry.IdempotencyKey.Validate(); err != nil {
			return err
		}
		if !safePremiumTransitionReference(entry.IdempotencyKey.String()) {
			return fmt.Errorf("idempotency_key: %w", ErrUnsafePremiumTransitionLogReference)
		}
	}
	for _, referenceID := range entry.ReferenceIDs {
		if !safePremiumTransitionReference(referenceID) {
			return fmt.Errorf("ref_ids: %w", ErrUnsafePremiumTransitionLogReference)
		}
	}
	for _, providerRefID := range entry.ProviderRefIDs {
		if !safePremiumTransitionReference(providerRefID) {
			return fmt.Errorf("provider_ref_ids: %w", ErrUnsafePremiumTransitionLogReference)
		}
	}
	if entry.Duration < 0 {
		return fmt.Errorf("duration %s: %w", entry.Duration, ErrInvalidDuration)
	}
	if entry.Timestamp.IsZero() {
		return ErrMissingPremiumTransitionLogTimestamp
	}
	return nil
}

// MarshalJSON encodes only safe premium transition log fields.
func (entry PremiumTransitionLogEntry) MarshalJSON() ([]byte, error) {
	entry = sanitizePremiumTransitionLogEntry(entry)
	type premiumTransitionLogJSON struct {
		PlayerID       foundation.PlayerID       `json:"player_id"`
		RequestRef     string                    `json:"request_id,omitempty"`
		Operation      Operation                 `json:"op"`
		Result         CommandStatus             `json:"result"`
		ErrorCode      foundation.Code           `json:"error_code"`
		IdempotencyKey foundation.IdempotencyKey `json:"idempotency_key,omitempty"`
		RefIDs         []string                  `json:"ref_ids,omitempty"`
		ProviderRefIDs []string                  `json:"provider_ref_ids,omitempty"`
		DurationMS     int64                     `json:"duration_ms"`
		Timestamp      time.Time                 `json:"timestamp"`
	}

	return json.Marshal(premiumTransitionLogJSON{
		PlayerID:       entry.PlayerID,
		RequestRef:     entry.RequestRef,
		Operation:      entry.Operation,
		Result:         entry.Status,
		ErrorCode:      entry.ErrorCode,
		IdempotencyKey: entry.IdempotencyKey,
		RefIDs:         premiumTransitionReferenceIDs(entry.IdempotencyKey, entry.ReferenceIDs),
		ProviderRefIDs: entry.ProviderRefIDs,
		DurationMS:     entry.Duration.Milliseconds(),
		Timestamp:      entry.Timestamp,
	})
}

// NewMemoryPremiumTransitionLogger returns an empty in-memory premium transition logger.
func NewMemoryPremiumTransitionLogger() *MemoryPremiumTransitionLogger {
	return &MemoryPremiumTransitionLogger{}
}

// NewJSONPremiumTransitionLogger returns a structured JSON-line premium transition logger.
func NewJSONPremiumTransitionLogger(writer io.Writer) (*JSONPremiumTransitionLogger, error) {
	if writer == nil {
		return nil, ErrMissingPremiumTransitionLogWriter
	}
	return &JSONPremiumTransitionLogger{writer: writer}, nil
}

// RecordPremiumTransition validates and stores a cloned, sanitized entry.
func (logger *MemoryPremiumTransitionLogger) RecordPremiumTransition(entry PremiumTransitionLogEntry) error {
	entry = sanitizePremiumTransitionLogEntry(entry)
	if err := entry.Validate(); err != nil {
		return err
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()

	logger.entries = append(logger.entries, clonePremiumTransitionLogEntry(entry))
	return nil
}

// RecordPremiumTransition validates and writes one JSON line.
func (logger *JSONPremiumTransitionLogger) RecordPremiumTransition(entry PremiumTransitionLogEntry) error {
	if logger == nil || logger.writer == nil {
		return ErrMissingPremiumTransitionLogWriter
	}
	entry = sanitizePremiumTransitionLogEntry(entry)
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
func (logger *MemoryPremiumTransitionLogger) Snapshot() []PremiumTransitionLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	entries := make([]PremiumTransitionLogEntry, len(logger.entries))
	for i, entry := range logger.entries {
		entries[i] = clonePremiumTransitionLogEntry(entry)
	}
	sortPremiumTransitionLogEntries(entries)
	return entries
}

func clonePremiumTransitionLogEntry(entry PremiumTransitionLogEntry) PremiumTransitionLogEntry {
	entry.ReferenceIDs = append([]string(nil), entry.ReferenceIDs...)
	entry.ProviderRefIDs = append([]string(nil), entry.ProviderRefIDs...)
	return entry
}

func sortPremiumTransitionLogEntries(entries []PremiumTransitionLogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if !left.Timestamp.Equal(right.Timestamp) {
			return left.Timestamp.Before(right.Timestamp)
		}
		if left.Operation != right.Operation {
			return left.Operation < right.Operation
		}
		if left.RequestRef != right.RequestRef {
			return left.RequestRef < right.RequestRef
		}
		return left.PlayerID < right.PlayerID
	})
}

func sanitizePremiumTransitionLogEntry(entry PremiumTransitionLogEntry) PremiumTransitionLogEntry {
	if !safePremiumTransitionReference(entry.RequestRef) {
		entry.RequestRef = ""
	}
	if !entry.IdempotencyKey.IsZero() && !safePremiumTransitionReference(entry.IdempotencyKey.String()) {
		entry.IdempotencyKey = ""
	}
	entry.ReferenceIDs = safePremiumTransitionReferences(entry.ReferenceIDs)
	entry.ProviderRefIDs = safePremiumTransitionReferences(entry.ProviderRefIDs)
	return entry
}

func premiumTransitionReferenceIDs(idempotencyKey foundation.IdempotencyKey, referenceIDs []string) []string {
	refs := make([]string, 0, 1+len(referenceIDs))
	if !idempotencyKey.IsZero() {
		refs = append(refs, idempotencyKey.String())
	}
	refs = append(refs, referenceIDs...)
	return safePremiumTransitionReferences(refs)
}

func safePremiumTransitionReferences(referenceIDs []string) []string {
	seen := make(map[string]struct{}, len(referenceIDs))
	result := make([]string, 0, len(referenceIDs))
	for _, referenceID := range referenceIDs {
		if !safePremiumTransitionReference(referenceID) {
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

func safePremiumTransitionReference(referenceID string) bool {
	value := strings.TrimSpace(referenceID)
	if value == "" || value != referenceID {
		return false
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"password",
		"passwd",
		"token",
		"secret",
		"cookie",
		"hash",
		"bearer",
		"credential",
	} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}

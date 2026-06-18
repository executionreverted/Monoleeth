package observability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// SessionID identifies an authenticated realtime session for command logs.
type SessionID string

// Operation names a server-side command or query operation.
type Operation string

// CommandStatus records the safe public outcome bucket for a command.
type CommandStatus string

const (
	// CommandStatusOK records a completed command.
	CommandStatusOK CommandStatus = "ok"

	// CommandStatusError records a rejected or failed command.
	CommandStatusError CommandStatus = "error"
)

// CommandLogEntry is the safe structured log shape for one gameplay command.
type CommandLogEntry struct {
	RequestID   foundation.RequestID      `json:"request_id"`
	PlayerID    foundation.PlayerID       `json:"player_id"`
	SessionID   SessionID                 `json:"session_id"`
	WorldID     foundation.WorldID        `json:"world_id,omitempty"`
	ZoneID      foundation.ZoneID         `json:"zone_id,omitempty"`
	Operation   Operation                 `json:"operation"`
	ErrorCode   foundation.Code           `json:"error_code,omitempty"`
	ReferenceID foundation.IdempotencyKey `json:"reference_id,omitempty"`
	Duration    time.Duration             `json:"duration"`
	Status      CommandStatus             `json:"status"`
	Timestamp   time.Time                 `json:"timestamp"`
}

// String returns the stable session id representation.
func (id SessionID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id SessionID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return fmt.Errorf("session_id: %w", ErrMissingCommandLogIdentity)
	}
	return nil
}

// IsZero reports whether id is the zero value.
func (id SessionID) IsZero() bool {
	return id == ""
}

// String returns the stable operation representation.
func (op Operation) String() string {
	return string(op)
}

// Validate reports whether op is non-blank.
func (op Operation) Validate() error {
	if strings.TrimSpace(string(op)) == "" {
		return ErrBlankOperation
	}
	return nil
}

// IsZero reports whether op is the zero value.
func (op Operation) IsZero() bool {
	return op == ""
}

// String returns the stable command status representation.
func (status CommandStatus) String() string {
	return string(status)
}

// Validate reports whether status is non-blank.
func (status CommandStatus) Validate() error {
	if strings.TrimSpace(string(status)) == "" {
		return ErrBlankCommandStatus
	}
	return nil
}

// IsZero reports whether status is the zero value.
func (status CommandStatus) IsZero() bool {
	return status == ""
}

// Validate reports whether entry has the required safe command log fields.
func (entry CommandLogEntry) Validate() error {
	if entry.RequestID.IsZero() {
		return fmt.Errorf("request_id: %w", ErrMissingCommandLogIdentity)
	}
	if err := entry.RequestID.Validate(); err != nil {
		return err
	}
	if entry.PlayerID.IsZero() {
		return fmt.Errorf("player_id: %w", ErrMissingCommandLogIdentity)
	}
	if err := entry.PlayerID.Validate(); err != nil {
		return err
	}
	if err := entry.SessionID.Validate(); err != nil {
		return err
	}
	if err := entry.Operation.Validate(); err != nil {
		return err
	}
	if err := entry.Status.Validate(); err != nil {
		return err
	}
	if !entry.WorldID.IsZero() {
		if err := entry.WorldID.Validate(); err != nil {
			return err
		}
	}
	if !entry.ZoneID.IsZero() {
		if err := entry.ZoneID.Validate(); err != nil {
			return err
		}
	}
	if !entry.ReferenceID.IsZero() {
		if err := entry.ReferenceID.Validate(); err != nil {
			return err
		}
	}
	if entry.Duration < 0 {
		return fmt.Errorf("duration %s: %w", entry.Duration, ErrInvalidDuration)
	}
	if entry.Timestamp.IsZero() {
		return ErrMissingCommandLogTimestamp
	}
	return nil
}

// MarshalJSON encodes only the safe public command log fields.
func (entry CommandLogEntry) MarshalJSON() ([]byte, error) {
	type commandLogJSON struct {
		RequestID   foundation.RequestID      `json:"request_id"`
		PlayerID    foundation.PlayerID       `json:"player_id"`
		SessionID   SessionID                 `json:"session_id"`
		WorldID     foundation.WorldID        `json:"world_id,omitempty"`
		ZoneID      foundation.ZoneID         `json:"zone_id,omitempty"`
		Operation   Operation                 `json:"operation"`
		ErrorCode   foundation.Code           `json:"error_code,omitempty"`
		ReferenceID foundation.IdempotencyKey `json:"reference_id,omitempty"`
		Duration    time.Duration             `json:"duration"`
		Status      CommandStatus             `json:"status"`
		Timestamp   time.Time                 `json:"timestamp"`
	}

	return json.Marshal(commandLogJSON{
		RequestID:   entry.RequestID,
		PlayerID:    entry.PlayerID,
		SessionID:   entry.SessionID,
		WorldID:     entry.WorldID,
		ZoneID:      entry.ZoneID,
		Operation:   entry.Operation,
		ErrorCode:   entry.ErrorCode,
		ReferenceID: entry.ReferenceID,
		Duration:    entry.Duration,
		Status:      entry.Status,
		Timestamp:   entry.Timestamp,
	})
}

// MemoryCommandLogger stores validated command log entries in memory.
type MemoryCommandLogger struct {
	mu      sync.Mutex
	entries []CommandLogEntry
}

// NewMemoryCommandLogger returns an empty in-memory command logger.
func NewMemoryCommandLogger() *MemoryCommandLogger {
	return &MemoryCommandLogger{}
}

// Record validates and stores a cloned entry.
func (logger *MemoryCommandLogger) Record(entry CommandLogEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()

	logger.entries = append(logger.entries, cloneCommandLogEntry(entry))
	return nil
}

// Snapshot returns a deterministic clone of stored entries.
func (logger *MemoryCommandLogger) Snapshot() []CommandLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	entries := make([]CommandLogEntry, len(logger.entries))
	for i, entry := range logger.entries {
		entries[i] = cloneCommandLogEntry(entry)
	}
	sortCommandLogEntries(entries)
	return entries
}

func cloneCommandLogEntry(entry CommandLogEntry) CommandLogEntry {
	return entry
}

func sortCommandLogEntries(entries []CommandLogEntry) {
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
		if left.PlayerID != right.PlayerID {
			return left.PlayerID < right.PlayerID
		}
		return left.SessionID < right.SessionID
	})
}

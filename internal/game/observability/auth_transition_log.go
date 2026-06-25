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

// AuthTransitionRequest is the safe request summary for an auth transition log.
// It deliberately omits request bodies, headers, cookies, and credentials.
type AuthTransitionRequest struct {
	Operation Operation `json:"op"`
}

// AuthTransitionLogEntry is the safe structured log shape for login/register.
type AuthTransitionLogEntry struct {
	RequestID foundation.RequestID  `json:"request_id,omitempty"`
	Request   AuthTransitionRequest `json:"request"`
	PlayerID  foundation.PlayerID   `json:"-"`
	SessionID SessionID             `json:"-"`
	ErrorCode foundation.Code       `json:"error_code"`
	Duration  time.Duration         `json:"-"`
	Status    CommandStatus         `json:"result"`
	Timestamp time.Time             `json:"timestamp"`
}

// AuthTransitionLogger records auth transition logs.
type AuthTransitionLogger interface {
	RecordAuthTransition(AuthTransitionLogEntry) error
}

// MemoryAuthTransitionLogger stores validated auth transition log entries.
type MemoryAuthTransitionLogger struct {
	mu      sync.Mutex
	entries []AuthTransitionLogEntry
}

// JSONAuthTransitionLogger writes one safe structured auth transition log line.
type JSONAuthTransitionLogger struct {
	mu     sync.Mutex
	writer io.Writer
}

// Validate reports whether request contains a stable operation name.
func (request AuthTransitionRequest) Validate() error {
	return request.Operation.Validate()
}

// Validate reports whether entry has required safe auth transition fields.
func (entry AuthTransitionLogEntry) Validate() error {
	if !entry.RequestID.IsZero() {
		if err := entry.RequestID.Validate(); err != nil {
			return err
		}
	}
	if err := entry.Request.Validate(); err != nil {
		return err
	}
	if err := entry.Status.Validate(); err != nil {
		return err
	}
	if entry.Status == CommandStatusOK {
		if entry.PlayerID.IsZero() {
			return fmt.Errorf("player_id: %w", ErrMissingAuthTransitionLogIdentity)
		}
		if entry.SessionID.IsZero() {
			return fmt.Errorf("session_id: %w", ErrMissingAuthTransitionLogIdentity)
		}
	}
	if !entry.PlayerID.IsZero() {
		if err := entry.PlayerID.Validate(); err != nil {
			return err
		}
	}
	if !entry.SessionID.IsZero() {
		if err := entry.SessionID.Validate(); err != nil {
			return err
		}
	}
	if entry.Duration < 0 {
		return fmt.Errorf("duration %s: %w", entry.Duration, ErrInvalidDuration)
	}
	if entry.Timestamp.IsZero() {
		return ErrMissingAuthTransitionLogTimestamp
	}
	return nil
}

// MarshalJSON encodes only safe auth transition log fields.
func (entry AuthTransitionLogEntry) MarshalJSON() ([]byte, error) {
	type authTransitionLogJSON struct {
		RequestID  foundation.RequestID  `json:"request_id,omitempty"`
		Request    AuthTransitionRequest `json:"request"`
		PlayerID   foundation.PlayerID   `json:"player_id,omitempty"`
		SessionID  SessionID             `json:"session_id,omitempty"`
		Operation  Operation             `json:"op"`
		Result     CommandStatus         `json:"result"`
		ErrorCode  foundation.Code       `json:"error_code"`
		DurationMS int64                 `json:"duration_ms"`
		Timestamp  time.Time             `json:"timestamp"`
	}

	return json.Marshal(authTransitionLogJSON{
		RequestID:  entry.RequestID,
		Request:    entry.Request,
		PlayerID:   entry.PlayerID,
		SessionID:  entry.SessionID,
		Operation:  entry.Request.Operation,
		Result:     entry.Status,
		ErrorCode:  entry.ErrorCode,
		DurationMS: entry.Duration.Milliseconds(),
		Timestamp:  entry.Timestamp,
	})
}

// NewMemoryAuthTransitionLogger returns an empty in-memory auth transition logger.
func NewMemoryAuthTransitionLogger() *MemoryAuthTransitionLogger {
	return &MemoryAuthTransitionLogger{}
}

// NewJSONAuthTransitionLogger returns a structured JSON-line auth transition logger.
func NewJSONAuthTransitionLogger(writer io.Writer) (*JSONAuthTransitionLogger, error) {
	if writer == nil {
		return nil, ErrMissingAuthTransitionLogWriter
	}
	return &JSONAuthTransitionLogger{writer: writer}, nil
}

// RecordAuthTransition validates and stores a cloned entry.
func (logger *MemoryAuthTransitionLogger) RecordAuthTransition(entry AuthTransitionLogEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()

	logger.entries = append(logger.entries, cloneAuthTransitionLogEntry(entry))
	return nil
}

// RecordAuthTransition validates and writes one JSON line.
func (logger *JSONAuthTransitionLogger) RecordAuthTransition(entry AuthTransitionLogEntry) error {
	if logger == nil || logger.writer == nil {
		return ErrMissingAuthTransitionLogWriter
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
func (logger *MemoryAuthTransitionLogger) Snapshot() []AuthTransitionLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()

	entries := make([]AuthTransitionLogEntry, len(logger.entries))
	for i, entry := range logger.entries {
		entries[i] = cloneAuthTransitionLogEntry(entry)
	}
	sortAuthTransitionLogEntries(entries)
	return entries
}

func cloneAuthTransitionLogEntry(entry AuthTransitionLogEntry) AuthTransitionLogEntry {
	return entry
}

func sortAuthTransitionLogEntries(entries []AuthTransitionLogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if !left.Timestamp.Equal(right.Timestamp) {
			return left.Timestamp.Before(right.Timestamp)
		}
		if left.Request.Operation != right.Request.Operation {
			return left.Request.Operation < right.Request.Operation
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

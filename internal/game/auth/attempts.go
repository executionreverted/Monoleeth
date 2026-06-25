package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthAttemptMaxFailures = 3
	defaultAuthAttemptWindow      = 5 * time.Minute
	defaultAuthAttemptLockout     = time.Minute
)

type AuthAttemptOperation string

const (
	AuthAttemptLogin    AuthAttemptOperation = "auth.login"
	AuthAttemptRegister AuthAttemptOperation = "auth.register"
)

// AuthAttemptPolicy controls short-lived auth abuse backoff.
type AuthAttemptPolicy struct {
	MaxFailures int
	Window      time.Duration
	Lockout     time.Duration
}

// AuthAttemptDecision reports whether an auth attempt is currently throttled.
type AuthAttemptDecision struct {
	Limited     bool
	LockedUntil time.Time
}

// AuthAttemptTracker stores failed auth attempts outside account/session truth.
type AuthAttemptTracker interface {
	Check(operation AuthAttemptOperation, subject string, now time.Time) (AuthAttemptDecision, error)
	RecordFailure(operation AuthAttemptOperation, subject string, now time.Time) (AuthAttemptDecision, error)
	Reset(operation AuthAttemptOperation, subject string, now time.Time) error
}

// InMemoryAuthAttemptTracker is a dev-safe volatile tracker for auth abuse.
type InMemoryAuthAttemptTracker struct {
	mu      sync.Mutex
	policy  AuthAttemptPolicy
	records map[authAttemptKey]authAttemptRecord
}

type authAttemptKey struct {
	operation AuthAttemptOperation
	subject   string
}

type authAttemptRecord struct {
	failures        int
	windowStartedAt time.Time
	lockedUntil     time.Time
}

// NewInMemoryAuthAttemptTracker returns a volatile auth attempt tracker.
func NewInMemoryAuthAttemptTracker(policy AuthAttemptPolicy) *InMemoryAuthAttemptTracker {
	return &InMemoryAuthAttemptTracker{
		policy:  authAttemptPolicyWithDefaults(policy),
		records: make(map[authAttemptKey]authAttemptRecord),
	}
}

func (tracker *InMemoryAuthAttemptTracker) Check(operation AuthAttemptOperation, subject string, now time.Time) (AuthAttemptDecision, error) {
	if tracker == nil {
		return AuthAttemptDecision{}, nil
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	key := authAttemptKey{operation: operation, subject: subject}
	record, ok := tracker.records[key]
	if !ok {
		return AuthAttemptDecision{}, nil
	}
	record, ok = tracker.activeRecord(record, now)
	if !ok {
		delete(tracker.records, key)
		return AuthAttemptDecision{}, nil
	}
	tracker.records[key] = record
	if now.Before(record.lockedUntil) {
		return AuthAttemptDecision{Limited: true, LockedUntil: record.lockedUntil}, nil
	}
	return AuthAttemptDecision{}, nil
}

func (tracker *InMemoryAuthAttemptTracker) RecordFailure(operation AuthAttemptOperation, subject string, now time.Time) (AuthAttemptDecision, error) {
	if tracker == nil {
		return AuthAttemptDecision{}, nil
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	key := authAttemptKey{operation: operation, subject: subject}
	record, ok := tracker.records[key]
	if !ok {
		record = authAttemptRecord{windowStartedAt: now}
	} else if record, ok = tracker.activeRecord(record, now); !ok {
		record = authAttemptRecord{windowStartedAt: now}
	}
	if now.Before(record.lockedUntil) {
		tracker.records[key] = record
		return AuthAttemptDecision{Limited: true, LockedUntil: record.lockedUntil}, nil
	}
	record.failures++
	if record.failures >= tracker.policy.MaxFailures {
		record.failures = 0
		record.windowStartedAt = now
		record.lockedUntil = now.Add(tracker.policy.Lockout)
		tracker.records[key] = record
		return AuthAttemptDecision{Limited: true, LockedUntil: record.lockedUntil}, nil
	}
	tracker.records[key] = record
	return AuthAttemptDecision{}, nil
}

func (tracker *InMemoryAuthAttemptTracker) Reset(operation AuthAttemptOperation, subject string, _ time.Time) error {
	if tracker == nil {
		return nil
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	delete(tracker.records, authAttemptKey{operation: operation, subject: subject})
	return nil
}

func (tracker *InMemoryAuthAttemptTracker) activeRecord(record authAttemptRecord, now time.Time) (authAttemptRecord, bool) {
	if !record.lockedUntil.IsZero() {
		if now.Before(record.lockedUntil) {
			return record, true
		}
		return authAttemptRecord{}, false
	}
	if !record.windowStartedAt.IsZero() && now.Sub(record.windowStartedAt) >= tracker.policy.Window {
		return authAttemptRecord{}, false
	}
	return record, true
}

func authAttemptPolicyWithDefaults(policy AuthAttemptPolicy) AuthAttemptPolicy {
	if policy.MaxFailures <= 0 {
		policy.MaxFailures = defaultAuthAttemptMaxFailures
	}
	if policy.Window <= 0 {
		policy.Window = defaultAuthAttemptWindow
	}
	if policy.Lockout <= 0 {
		policy.Lockout = defaultAuthAttemptLockout
	}
	return policy
}

func authAttemptSubject(rawEmail string) string {
	normalized := strings.ToLower(strings.TrimSpace(rawEmail))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

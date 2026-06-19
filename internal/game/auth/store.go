package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// Store is the persistence boundary for accounts, player profiles, and
// sessions. The MVP implementation is in-memory and documented as volatile.
type Store interface {
	InsertAccount(context.Context, Account, PlayerProfile) error
	UpdateAccount(context.Context, Account) error
	AccountByEmail(context.Context, Email) (Account, PlayerProfile, error)
	AccountByID(context.Context, foundation.AccountID) (Account, PlayerProfile, error)
	InsertSession(context.Context, Session) error
	SessionByTokenHash(context.Context, string) (Session, error)
	SessionByID(context.Context, SessionID) (Session, error)
	RevokeSession(context.Context, SessionID, time.Time) error
}

// InMemoryStore is the volatile MVP auth repository.
type InMemoryStore struct {
	mu               sync.RWMutex
	accountsByID     map[foundation.AccountID]Account
	accountsByEmail  map[Email]foundation.AccountID
	playersByAccount map[foundation.AccountID]PlayerProfile
	sessionsByID     map[SessionID]Session
	sessionIDByHash  map[string]SessionID
}

// NewInMemoryStore returns an empty volatile auth store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		accountsByID:     make(map[foundation.AccountID]Account),
		accountsByEmail:  make(map[Email]foundation.AccountID),
		playersByAccount: make(map[foundation.AccountID]PlayerProfile),
		sessionsByID:     make(map[SessionID]Session),
		sessionIDByHash:  make(map[string]SessionID),
	}
}

// InsertAccount stores a new account and profile atomically.
func (store *InMemoryStore) InsertAccount(_ context.Context, account Account, player PlayerProfile) error {
	if store == nil {
		return ErrNilAuthStore
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.accountsByEmail[account.Email]; exists {
		return ErrDuplicateEmail
	}
	if _, exists := store.accountsByID[account.ID]; exists {
		return fmt.Errorf("account id %q: %w", account.ID, ErrDuplicateEmail)
	}
	store.accountsByID[account.ID] = cloneAccount(account)
	store.accountsByEmail[account.Email] = account.ID
	store.playersByAccount[account.ID] = clonePlayer(player)
	return nil
}

// UpdateAccount replaces mutable account fields for an existing account.
func (store *InMemoryStore) UpdateAccount(_ context.Context, account Account) error {
	if store == nil {
		return ErrNilAuthStore
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	existing, ok := store.accountsByID[account.ID]
	if !ok {
		return ErrAccountNotFound
	}
	if existing.Email != account.Email {
		return ErrInvalidEmail
	}
	store.accountsByID[account.ID] = cloneAccount(account)
	return nil
}

// AccountByEmail returns account and player profile by normalized email.
func (store *InMemoryStore) AccountByEmail(_ context.Context, email Email) (Account, PlayerProfile, error) {
	if store == nil {
		return Account{}, PlayerProfile{}, ErrNilAuthStore
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	accountID, ok := store.accountsByEmail[email]
	if !ok {
		return Account{}, PlayerProfile{}, ErrAccountNotFound
	}
	account := store.accountsByID[accountID]
	player := store.playersByAccount[accountID]
	return cloneAccount(account), clonePlayer(player), nil
}

// AccountByID returns account and player profile by account id.
func (store *InMemoryStore) AccountByID(_ context.Context, accountID foundation.AccountID) (Account, PlayerProfile, error) {
	if store == nil {
		return Account{}, PlayerProfile{}, ErrNilAuthStore
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	account, ok := store.accountsByID[accountID]
	if !ok {
		return Account{}, PlayerProfile{}, ErrAccountNotFound
	}
	player := store.playersByAccount[accountID]
	return cloneAccount(account), clonePlayer(player), nil
}

// InsertSession stores a new session by id and token hash.
func (store *InMemoryStore) InsertSession(_ context.Context, session Session) error {
	if store == nil {
		return ErrNilAuthStore
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.sessionsByID[session.ID]; exists {
		return ErrDuplicateSession
	}
	if _, exists := store.sessionIDByHash[session.TokenHash]; exists {
		return ErrDuplicateSession
	}
	store.sessionsByID[session.ID] = cloneSession(session)
	store.sessionIDByHash[session.TokenHash] = session.ID
	return nil
}

// SessionByTokenHash returns a session by stored token hash.
func (store *InMemoryStore) SessionByTokenHash(_ context.Context, hash string) (Session, error) {
	if store == nil {
		return Session{}, ErrNilAuthStore
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	sessionID, ok := store.sessionIDByHash[hash]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return cloneSession(store.sessionsByID[sessionID]), nil
}

// SessionByID returns a session by server-owned session id.
func (store *InMemoryStore) SessionByID(_ context.Context, sessionID SessionID) (Session, error) {
	if store == nil {
		return Session{}, ErrNilAuthStore
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	session, ok := store.sessionsByID[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

// RevokeSession marks a session revoked.
func (store *InMemoryStore) RevokeSession(_ context.Context, sessionID SessionID, revokedAt time.Time) error {
	if store == nil {
		return ErrNilAuthStore
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	session, ok := store.sessionsByID[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	t := revokedAt.UTC()
	session.RevokedAt = &t
	store.sessionsByID[sessionID] = cloneSession(session)
	return nil
}

func cloneAccount(account Account) Account {
	account.Roles = cloneRoles(account.Roles)
	return account
}

func clonePlayer(player PlayerProfile) PlayerProfile {
	return player
}

func cloneSession(session Session) Session {
	session.Roles = cloneRoles(session.Roles)
	if session.RevokedAt != nil {
		revokedAt := *session.RevokedAt
		session.RevokedAt = &revokedAt
	}
	return session
}

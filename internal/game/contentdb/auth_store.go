package contentdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
)

type AuthStore struct {
	store *Store
}

var _ auth.Store = (*AuthStore)(nil)

func NewAuthStore(store *Store) (*AuthStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &AuthStore{store: store}, nil
}

func (store *AuthStore) InsertAccount(ctx context.Context, account auth.Account, player auth.PlayerProfile) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return auth.ErrNilAuthStore
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO accounts(id, email, password_hash, roles, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, account.ID.String(), account.Email.String(), string(account.PasswordHash), postgresTextArray(account.Roles), account.CreatedAt.UTC(), account.UpdatedAt.UTC()); err != nil {
		return mapAuthWriteError(err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO players(id, account_id, callsign, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, player.ID.String(), player.AccountID.String(), player.Callsign, player.CreatedAt.UTC(), player.UpdatedAt.UTC()); err != nil {
		return mapAuthWriteError(err)
	}
	err = tx.Commit()
	return err
}

func (store *AuthStore) UpdateAccount(ctx context.Context, account auth.Account) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return auth.ErrNilAuthStore
	}
	result, err := store.store.db.ExecContext(ctx, `
		UPDATE accounts
		SET password_hash = $2, roles = $3, updated_at = $4
		WHERE id = $1 AND email = $5
	`, account.ID.String(), string(account.PasswordHash), postgresTextArray(account.Roles), account.UpdatedAt.UTC(), account.Email.String())
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return auth.ErrAccountNotFound
	}
	return nil
}

func (store *AuthStore) AccountByEmail(ctx context.Context, email auth.Email) (auth.Account, auth.PlayerProfile, error) {
	return store.accountByLookup(ctx, `WHERE accounts.email = $1`, email.String())
}

func (store *AuthStore) AccountByID(ctx context.Context, accountID foundation.AccountID) (auth.Account, auth.PlayerProfile, error) {
	return store.accountByLookup(ctx, `WHERE accounts.id = $1`, accountID.String())
}

func (store *AuthStore) InsertSession(ctx context.Context, session auth.Session) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return auth.ErrNilAuthStore
	}
	_, err := store.store.db.ExecContext(ctx, `
		INSERT INTO auth_sessions(id, account_id, player_id, token_hash, roles, created_at, expires_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, session.ID.String(), session.AccountID.String(), session.PlayerID.String(), session.TokenHash, postgresTextArray(session.Roles), session.CreatedAt.UTC(), session.ExpiresAt.UTC(), nullableTime(session.RevokedAt))
	if err != nil {
		return mapAuthSessionWriteError(err)
	}
	return nil
}

func (store *AuthStore) SessionByTokenHash(ctx context.Context, hash string) (auth.Session, error) {
	return store.sessionByLookup(ctx, `WHERE token_hash = $1`, hash)
}

func (store *AuthStore) SessionByID(ctx context.Context, sessionID auth.SessionID) (auth.Session, error) {
	return store.sessionByLookup(ctx, `WHERE id = $1`, sessionID.String())
}

func (store *AuthStore) RevokeSession(ctx context.Context, sessionID auth.SessionID, revokedAt time.Time) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return auth.ErrNilAuthStore
	}
	result, err := store.store.db.ExecContext(ctx, `
		UPDATE auth_sessions
		SET revoked_at = $2
		WHERE id = $1
	`, sessionID.String(), revokedAt.UTC())
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return auth.ErrSessionNotFound
	}
	return nil
}

func (store *AuthStore) accountByLookup(ctx context.Context, where string, arg any) (auth.Account, auth.PlayerProfile, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return auth.Account{}, auth.PlayerProfile{}, auth.ErrNilAuthStore
	}
	row := store.store.db.QueryRowContext(ctx, `
		SELECT accounts.id, accounts.email, accounts.password_hash, accounts.roles::text, accounts.created_at, accounts.updated_at,
			players.id, players.account_id, players.callsign, players.created_at, players.updated_at
		FROM accounts
		JOIN players ON players.account_id = accounts.id
		`+where+`
	`, arg)
	var accountID string
	var email string
	var passwordHash string
	var roles string
	var accountCreatedAt time.Time
	var accountUpdatedAt time.Time
	var playerID string
	var playerAccountID string
	var callsign string
	var playerCreatedAt time.Time
	var playerUpdatedAt time.Time
	if err := row.Scan(&accountID, &email, &passwordHash, &roles, &accountCreatedAt, &accountUpdatedAt, &playerID, &playerAccountID, &callsign, &playerCreatedAt, &playerUpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.Account{}, auth.PlayerProfile{}, auth.ErrAccountNotFound
		}
		return auth.Account{}, auth.PlayerProfile{}, err
	}
	return auth.Account{
			ID:           foundation.AccountID(accountID),
			Email:        auth.Email(email),
			PasswordHash: auth.PasswordHash(passwordHash),
			Roles:        parsePostgresTextArray(roles),
			CreatedAt:    accountCreatedAt.UTC(),
			UpdatedAt:    accountUpdatedAt.UTC(),
		}, auth.PlayerProfile{
			ID:        foundation.PlayerID(playerID),
			AccountID: foundation.AccountID(playerAccountID),
			Callsign:  callsign,
			CreatedAt: playerCreatedAt.UTC(),
			UpdatedAt: playerUpdatedAt.UTC(),
		}, nil
}

func (store *AuthStore) sessionByLookup(ctx context.Context, where string, arg any) (auth.Session, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return auth.Session{}, auth.ErrNilAuthStore
	}
	row := store.store.db.QueryRowContext(ctx, `
		SELECT id, account_id, player_id, token_hash, roles::text, created_at, expires_at, revoked_at
		FROM auth_sessions
		`+where+`
	`, arg)
	var sessionID string
	var accountID string
	var playerID string
	var tokenHash string
	var roles string
	var createdAt time.Time
	var expiresAt time.Time
	var revokedAt sql.NullTime
	if err := row.Scan(&sessionID, &accountID, &playerID, &tokenHash, &roles, &createdAt, &expiresAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.Session{}, auth.ErrSessionNotFound
		}
		return auth.Session{}, err
	}
	session := auth.Session{
		ID:        auth.SessionID(sessionID),
		AccountID: foundation.AccountID(accountID),
		PlayerID:  foundation.PlayerID(playerID),
		TokenHash: tokenHash,
		Roles:     parsePostgresTextArray(roles),
		CreatedAt: createdAt.UTC(),
		ExpiresAt: expiresAt.UTC(),
	}
	if revokedAt.Valid {
		revoked := revokedAt.Time.UTC()
		session.RevokedAt = &revoked
	}
	return session, nil
}

func mapAuthWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return auth.ErrDuplicateEmail
	}
	return err
}

func mapAuthSessionWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return auth.ErrDuplicateSession
	}
	return err
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func postgresTextArray[T ~string](values []T) string {
	if len(values) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Quote(string(value)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func parsePostgresTextArray(value string) []auth.Role {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "{}" {
		return nil
	}
	trimmed = strings.TrimPrefix(strings.TrimSuffix(trimmed, "}"), "{")
	parts := strings.Split(trimmed, ",")
	roles := make([]auth.Role, 0, len(parts))
	for _, part := range parts {
		unquoted, err := strconv.Unquote(part)
		if err != nil {
			unquoted = part
		}
		unquoted = strings.TrimSpace(unquoted)
		if unquoted == "" {
			continue
		}
		roles = append(roles, auth.Role(unquoted))
	}
	return roles
}

func (store *AuthStore) String() string {
	return fmt.Sprintf("%T", store)
}

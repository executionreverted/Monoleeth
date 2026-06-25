package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/premium"
)

type PremiumEntitlementStore struct {
	store *Store
}

var _ premium.PremiumEntitlementRepository = (*PremiumEntitlementStore)(nil)
var _ premium.PremiumEntitlementTransactionRepository = (*PremiumEntitlementStore)(nil)

type PremiumStore = PremiumEntitlementStore

type PremiumEntitlementTx struct {
	tx *sql.Tx
}

var _ premium.PremiumEntitlementTransaction = (*PremiumEntitlementTx)(nil)

type premiumEntitlementSQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type premiumEntitlementSQLQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewPremiumEntitlementStore(store *Store) (*PremiumEntitlementStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &PremiumEntitlementStore{store: store}, nil
}

func NewPremiumStore(store *Store) (*PremiumStore, error) {
	return NewPremiumEntitlementStore(store)
}

func (store *PremiumEntitlementStore) WithTransaction(ctx context.Context, fn func(*PremiumEntitlementTx) error) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if fn == nil {
		return errors.New("nil premium entitlement transaction function")
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if err = fn(&PremiumEntitlementTx{tx: tx}); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (store *PremiumEntitlementStore) WithPremiumEntitlementTransaction(
	ctx context.Context,
	fn func(premium.PremiumEntitlementTransaction) error,
) error {
	if fn == nil {
		return errors.New("nil premium entitlement transaction function")
	}
	return store.WithTransaction(ctx, func(tx *PremiumEntitlementTx) error {
		return fn(tx)
	})
}

func (store *PremiumEntitlementStore) SavePremiumEntitlement(ctx context.Context, entitlement premium.Entitlement) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	return upsertPremiumEntitlement(ctx, store.store.db, entitlement)
}

func (tx *PremiumEntitlementTx) SavePremiumEntitlement(ctx context.Context, entitlement premium.Entitlement) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return upsertPremiumEntitlement(ctx, tx.tx, entitlement)
}

func (tx *PremiumEntitlementTx) CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return commitWalletMutation(ctx, tx.tx, commit)
}

func (tx *PremiumEntitlementTx) ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error) {
	if tx == nil || tx.tx == nil {
		return economy.IdempotencyClaimResult{}, ErrNilDatabase
	}
	return claimIdempotencyKey(ctx, tx.tx, row)
}

func (tx *PremiumEntitlementTx) CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error) {
	if tx == nil || tx.tx == nil {
		return economy.IdempotencyKeyRow{}, ErrNilDatabase
	}
	return completeIdempotencyKey(ctx, tx.tx, row)
}

func (tx *PremiumEntitlementTx) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return insertOutboxRow(ctx, tx.tx, row)
}

func (store *PremiumEntitlementStore) LoadPremiumEntitlement(ctx context.Context, entitlementID premium.EntitlementID) (premium.Entitlement, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return premium.Entitlement{}, false, ErrNilDatabase
	}
	return loadPremiumEntitlement(ctx, store.store.db, entitlementID, false)
}

func (tx *PremiumEntitlementTx) LoadPremiumEntitlementForUpdate(ctx context.Context, entitlementID premium.EntitlementID) (premium.Entitlement, bool, error) {
	if tx == nil || tx.tx == nil {
		return premium.Entitlement{}, false, ErrNilDatabase
	}
	return loadPremiumEntitlement(ctx, tx.tx, entitlementID, true)
}

func (store *PremiumEntitlementStore) LoadPremiumEntitlementByProvider(ctx context.Context, provider premium.ProviderReference) (premium.Entitlement, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return premium.Entitlement{}, false, ErrNilDatabase
	}
	return loadPremiumEntitlementByProvider(ctx, store.store.db, provider, false)
}

func (tx *PremiumEntitlementTx) LoadPremiumEntitlementByProviderForUpdate(ctx context.Context, provider premium.ProviderReference) (premium.Entitlement, bool, error) {
	if tx == nil || tx.tx == nil {
		return premium.Entitlement{}, false, ErrNilDatabase
	}
	return loadPremiumEntitlementByProvider(ctx, tx.tx, provider, true)
}

func (store *PremiumEntitlementStore) LoadPremiumEntitlements(ctx context.Context) ([]premium.Entitlement, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, premiumEntitlementSelectSQL()+`
		ORDER BY entitlement_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entitlements := make([]premium.Entitlement, 0)
	for rows.Next() {
		entitlement, err := scanPremiumEntitlement(rows)
		if err != nil {
			return nil, err
		}
		entitlements = append(entitlements, entitlement)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(entitlements, func(i, j int) bool {
		return entitlements[i].ID < entitlements[j].ID
	})
	return entitlements, nil
}

func upsertPremiumEntitlement(ctx context.Context, execer premiumEntitlementSQLExecer, entitlement premium.Entitlement) error {
	if err := entitlement.ValidateSnapshot(); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(entitlement.Payload)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO premium_entitlements(
			entitlement_id, player_id, entitlement_type, state, provider_source,
			provider_reference, payload_json, created_at, provider_confirmed_at,
			claimed_at, claim_request_ref
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11)
		ON CONFLICT (entitlement_id) DO UPDATE
		SET player_id = EXCLUDED.player_id,
			entitlement_type = EXCLUDED.entitlement_type,
			state = EXCLUDED.state,
			provider_source = EXCLUDED.provider_source,
			provider_reference = EXCLUDED.provider_reference,
			payload_json = EXCLUDED.payload_json,
			created_at = EXCLUDED.created_at,
			provider_confirmed_at = EXCLUDED.provider_confirmed_at,
			claimed_at = EXCLUDED.claimed_at,
			claim_request_ref = EXCLUDED.claim_request_ref
	`, entitlement.ID.String(), entitlement.PlayerID.String(), entitlement.Type.String(),
		entitlement.State.String(), entitlement.Provider.Source, entitlement.Provider.Reference,
		string(payloadJSON), entitlement.CreatedAt.UTC(), entitlement.ProviderConfirmedAt.UTC(),
		premiumClaimedAtAsNilPointer(entitlement.ClaimedAt), entitlement.ClaimRequestRef)
	return err
}

func loadPremiumEntitlement(ctx context.Context, querier premiumEntitlementSQLQuerier, entitlementID premium.EntitlementID, forUpdate bool) (premium.Entitlement, bool, error) {
	if err := entitlementID.Validate(); err != nil {
		return premium.Entitlement{}, false, err
	}
	query := premiumEntitlementSelectSQL() + `
		WHERE entitlement_id = $1
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	entitlement, err := scanPremiumEntitlement(querier.QueryRowContext(ctx, query, entitlementID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return premium.Entitlement{}, false, nil
	}
	if err != nil {
		return premium.Entitlement{}, false, err
	}
	return entitlement, true, nil
}

func loadPremiumEntitlementByProvider(ctx context.Context, querier premiumEntitlementSQLQuerier, provider premium.ProviderReference, forUpdate bool) (premium.Entitlement, bool, error) {
	if err := provider.Validate(); err != nil {
		return premium.Entitlement{}, false, err
	}
	query := premiumEntitlementSelectSQL() + `
		WHERE provider_source = $1 AND provider_reference = $2
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	entitlement, err := scanPremiumEntitlement(querier.QueryRowContext(ctx, query, provider.Source, provider.Reference))
	if errors.Is(err, sql.ErrNoRows) {
		return premium.Entitlement{}, false, nil
	}
	if err != nil {
		return premium.Entitlement{}, false, err
	}
	return entitlement, true, nil
}

func scanPremiumEntitlement(scanner rowScanner) (premium.Entitlement, error) {
	var entitlement premium.Entitlement
	var entitlementID string
	var playerID string
	var entitlementType string
	var state string
	var providerSource string
	var providerReference string
	var payloadJSON []byte
	var claimedAt sql.NullTime
	if err := scanner.Scan(
		&entitlementID,
		&playerID,
		&entitlementType,
		&state,
		&providerSource,
		&providerReference,
		&payloadJSON,
		&entitlement.CreatedAt,
		&entitlement.ProviderConfirmedAt,
		&claimedAt,
		&entitlement.ClaimRequestRef,
	); err != nil {
		return premium.Entitlement{}, err
	}
	var payload premium.EntitlementGrantPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return premium.Entitlement{}, err
	}
	entitlement.ID = premium.EntitlementID(entitlementID)
	entitlement.PlayerID = foundation.PlayerID(playerID)
	entitlement.Type = premium.EntitlementType(entitlementType)
	entitlement.State = premium.EntitlementState(state)
	entitlement.Provider = premium.ProviderReference{Source: providerSource, Reference: providerReference}
	entitlement.Payload = payload
	entitlement.CreatedAt = entitlement.CreatedAt.UTC()
	entitlement.ProviderConfirmedAt = entitlement.ProviderConfirmedAt.UTC()
	if claimedAt.Valid {
		entitlement.ClaimedAt = claimedAt.Time.UTC()
	}
	if err := entitlement.ValidateSnapshot(); err != nil {
		return premium.Entitlement{}, err
	}
	return entitlement, nil
}

func premiumEntitlementSelectSQL() string {
	return `
		SELECT
			entitlement_id,
			player_id,
			entitlement_type,
			state,
			provider_source,
			provider_reference,
			payload_json,
			created_at,
			provider_confirmed_at,
			claimed_at,
			claim_request_ref
		FROM premium_entitlements
	`
}

func premiumClaimedAtAsNilPointer(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

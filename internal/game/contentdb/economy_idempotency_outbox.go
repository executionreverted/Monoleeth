package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

var (
	_ economy.IdempotencyStore = (*Store)(nil)
	_ economy.OutboxStore      = (*Store)(nil)
)

type idempotencySQLRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type outboxSQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (store *Store) ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error) {
	if store == nil || store.db == nil {
		return economy.IdempotencyClaimResult{}, ErrNilDatabase
	}
	return claimIdempotencyKey(ctx, store.db, row)
}

func claimIdempotencyKey(ctx context.Context, runner idempotencySQLRunner, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error) {
	if runner == nil {
		return economy.IdempotencyClaimResult{}, ErrNilDatabase
	}
	row = normalizeIdempotencyKeyRow(row)
	if err := row.Validate(); err != nil {
		return economy.IdempotencyClaimResult{}, err
	}

	result, err := runner.ExecContext(ctx, `
		INSERT INTO idempotency_keys(
			scope, idempotency_key, operation, player_id, request_hash, status,
			result_json, created_at, updated_at, completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
		ON CONFLICT (scope, idempotency_key) DO NOTHING
	`, row.Scope, row.Key.String(), row.Operation, row.PlayerID.String(), row.RequestHash,
		string(row.Status), string(row.ResultJSON), row.CreatedAt.UTC(), row.UpdatedAt.UTC(),
		zeroTimeAsNil(row.CompletedAt))
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	stored, ok, err := loadIdempotencyKeyRow(ctx, runner, row.Scope, row.Key)
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	if !ok {
		return economy.IdempotencyClaimResult{}, sql.ErrNoRows
	}
	if rows == 1 {
		return economy.IdempotencyClaimResult{Row: stored}, nil
	}
	return economy.ResolveIdempotencyClaim(&stored, row)
}

func (store *Store) CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error) {
	if store == nil || store.db == nil {
		return economy.IdempotencyKeyRow{}, ErrNilDatabase
	}
	return completeIdempotencyKey(ctx, store.db, row)
}

func completeIdempotencyKey(ctx context.Context, runner idempotencySQLRunner, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error) {
	if runner == nil {
		return economy.IdempotencyKeyRow{}, ErrNilDatabase
	}
	row = normalizeIdempotencyKeyRow(row)
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}

	stored, err := scanIdempotencyKeyRow(runner.QueryRowContext(ctx, `
		INSERT INTO idempotency_keys(
			scope, idempotency_key, operation, player_id, request_hash, status,
			result_json, created_at, updated_at, completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
		ON CONFLICT (scope, idempotency_key) DO UPDATE
		SET status = EXCLUDED.status,
			result_json = EXCLUDED.result_json,
			updated_at = EXCLUDED.updated_at,
			completed_at = EXCLUDED.completed_at
		WHERE idempotency_keys.operation = EXCLUDED.operation
			AND idempotency_keys.player_id = EXCLUDED.player_id
			AND idempotency_keys.request_hash = EXCLUDED.request_hash
		RETURNING
			scope, idempotency_key, operation, player_id, request_hash, status,
			result_json, created_at, updated_at, completed_at
	`, row.Scope, row.Key.String(), row.Operation, row.PlayerID.String(), row.RequestHash,
		string(row.Status), string(row.ResultJSON), row.CreatedAt.UTC(), row.UpdatedAt.UTC(),
		zeroTimeAsNil(row.CompletedAt)))
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return economy.IdempotencyKeyRow{}, err
	}

	existing, ok, loadErr := loadIdempotencyKeyRow(ctx, runner, row.Scope, row.Key)
	if loadErr != nil {
		return economy.IdempotencyKeyRow{}, loadErr
	}
	if !ok {
		return economy.IdempotencyKeyRow{}, sql.ErrNoRows
	}
	_, resolveErr := economy.ResolveIdempotencyClaim(&existing, row)
	if errors.Is(resolveErr, economy.ErrIdempotencyKeyConflict) {
		return economy.IdempotencyKeyRow{}, resolveErr
	}
	if resolveErr != nil {
		return economy.IdempotencyKeyRow{}, resolveErr
	}
	return economy.IdempotencyKeyRow{}, fmt.Errorf("idempotency key %q unchanged: %w", row.Key, sql.ErrNoRows)
}

func (store *Store) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if store == nil || store.db == nil {
		return ErrNilDatabase
	}
	return insertOutboxRow(ctx, store.db, row)
}

func insertOutboxRow(ctx context.Context, execer outboxSQLExecer, row economy.OutboxRow) error {
	if execer == nil {
		return ErrNilDatabase
	}
	row = normalizeOutboxRowTimes(row)
	row, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	row = normalizeOutboxRowTimes(row)
	if err := row.Validate(); err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		INSERT INTO outbox(
			outbox_id, topic, event_type, aggregate_type, aggregate_id,
			idempotency_scope, idempotency_key, payload_json, status, available_at,
			lease_owner, leased_until, attempt_count, max_attempts, last_error,
			created_at, updated_at, published_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, row.OutboxID, row.Topic, row.EventType, row.AggregateType, row.AggregateID,
		row.IdempotencyScope, row.IdempotencyKey.String(), string(row.PayloadJSON),
		string(row.Status), row.AvailableAt.UTC(), row.LeaseOwner, zeroTimeAsNil(row.LeasedUntil),
		row.AttemptCount, row.MaxAttempts, row.LastError, row.CreatedAt.UTC(), row.UpdatedAt.UTC(),
		zeroTimeAsNil(row.PublishedAt))
	return err
}

func (store *Store) LoadOutboxRow(ctx context.Context, outboxID string) (economy.OutboxRow, bool, error) {
	if store == nil || store.db == nil {
		return economy.OutboxRow{}, false, ErrNilDatabase
	}
	if strings.TrimSpace(outboxID) == "" || outboxID != strings.TrimSpace(outboxID) {
		return economy.OutboxRow{}, false, economy.ErrInvalidOutboxRow
	}
	row, err := scanOutboxRow(store.db.QueryRowContext(ctx, outboxSelectSQL()+`
		WHERE outbox_id = $1
	`, outboxID))
	if errors.Is(err, sql.ErrNoRows) {
		return economy.OutboxRow{}, false, nil
	}
	if err != nil {
		return economy.OutboxRow{}, false, err
	}
	return row, true, nil
}

func (store *Store) LoadDueOutboxRows(ctx context.Context, query economy.OutboxDueRowsQuery) ([]economy.OutboxRow, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	query.Now = query.Now.UTC()
	if err := query.Validate(); err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, outboxSelectSQL()+`
		WHERE status IN ($2, $3)
			AND available_at <= $1
		ORDER BY available_at ASC, created_at ASC, outbox_id ASC
		LIMIT $4
	`, query.Now, string(economy.OutboxStatusPending), string(economy.OutboxStatusFailed), query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	due := make([]economy.OutboxRow, 0, query.Limit)
	for rows.Next() {
		row, err := scanOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		due = append(due, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return due, nil
}

func (store *Store) LeaseOutboxRow(ctx context.Context, input economy.OutboxLeaseInput) (economy.OutboxRow, bool, error) {
	if store == nil || store.db == nil {
		return economy.OutboxRow{}, false, ErrNilDatabase
	}
	input.Now = input.Now.UTC()
	input.LeasedUntil = input.LeasedUntil.UTC()
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	row, err := scanOutboxRow(store.db.QueryRowContext(ctx, `
		UPDATE outbox
		SET status = $2,
			lease_owner = $3,
			leased_until = $4,
			updated_at = $5
		WHERE outbox_id = $1
			AND (
				(status IN ($6, $7) AND available_at <= $5)
				OR (status = $2 AND leased_until <= $5)
			)
		`+outboxReturningSQL()+`
	`, input.OutboxID, string(economy.OutboxStatusLeased), input.LeaseOwner,
		input.LeasedUntil, input.Now, string(economy.OutboxStatusPending), string(economy.OutboxStatusFailed)))
	if errors.Is(err, sql.ErrNoRows) {
		return economy.OutboxRow{}, false, nil
	}
	if err != nil {
		return economy.OutboxRow{}, false, err
	}
	return row, true, nil
}

func (store *Store) MarkOutboxPublished(ctx context.Context, input economy.OutboxPublishInput) (economy.OutboxRow, bool, error) {
	if store == nil || store.db == nil {
		return economy.OutboxRow{}, false, ErrNilDatabase
	}
	input.Now = input.Now.UTC()
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	row, err := scanOutboxRow(store.db.QueryRowContext(ctx, `
		UPDATE outbox
		SET status = $2,
			lease_owner = '',
			leased_until = NULL,
			attempt_count = attempt_count + 1,
			last_error = '',
			updated_at = $4,
			published_at = $4
		WHERE outbox_id = $1
			AND status = $3
			AND lease_owner = $5
			AND leased_until > $4
		`+outboxReturningSQL()+`
	`, input.OutboxID, string(economy.OutboxStatusPublished), string(economy.OutboxStatusLeased), input.Now, input.LeaseOwner))
	if errors.Is(err, sql.ErrNoRows) {
		return economy.OutboxRow{}, false, nil
	}
	if err != nil {
		return economy.OutboxRow{}, false, err
	}
	return row, true, nil
}

func (store *Store) MarkOutboxFailed(ctx context.Context, input economy.OutboxFailureInput) (economy.OutboxRow, bool, error) {
	if store == nil || store.db == nil {
		return economy.OutboxRow{}, false, ErrNilDatabase
	}
	input.Now = input.Now.UTC()
	input.AvailableAt = input.AvailableAt.UTC()
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	row, err := scanOutboxRow(store.db.QueryRowContext(ctx, `
		UPDATE outbox
		SET status = CASE
				WHEN attempt_count + 1 >= max_attempts THEN $2
				ELSE $3
			END,
			available_at = $4,
			lease_owner = '',
			leased_until = NULL,
			attempt_count = attempt_count + 1,
			last_error = $5,
			updated_at = $6
		WHERE outbox_id = $1
			AND status = $7
			AND lease_owner = $8
			AND leased_until > $6
		`+outboxReturningSQL()+`
	`, input.OutboxID, string(economy.OutboxStatusDead), string(economy.OutboxStatusFailed),
		input.AvailableAt, input.LastError, input.Now, string(economy.OutboxStatusLeased), input.LeaseOwner))
	if errors.Is(err, sql.ErrNoRows) {
		return economy.OutboxRow{}, false, nil
	}
	if err != nil {
		return economy.OutboxRow{}, false, err
	}
	return row, true, nil
}

func (store *Store) loadIdempotencyKeyRow(ctx context.Context, scope string, key foundation.IdempotencyKey) (economy.IdempotencyKeyRow, bool, error) {
	if store == nil || store.db == nil {
		return economy.IdempotencyKeyRow{}, false, ErrNilDatabase
	}
	return loadIdempotencyKeyRow(ctx, store.db, scope, key)
}

func loadIdempotencyKeyRow(ctx context.Context, runner idempotencySQLRunner, scope string, key foundation.IdempotencyKey) (economy.IdempotencyKeyRow, bool, error) {
	if runner == nil {
		return economy.IdempotencyKeyRow{}, false, ErrNilDatabase
	}
	row, err := scanIdempotencyKeyRow(runner.QueryRowContext(ctx, idempotencyKeySelectSQL()+`
		FROM idempotency_keys
		WHERE scope = $1 AND idempotency_key = $2
	`, scope, key.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return economy.IdempotencyKeyRow{}, false, nil
	}
	if err != nil {
		return economy.IdempotencyKeyRow{}, false, err
	}
	return row, true, nil
}

func scanIdempotencyKeyRow(scanner rowScanner) (economy.IdempotencyKeyRow, error) {
	var row economy.IdempotencyKeyRow
	var key string
	var playerID string
	var status string
	var resultJSON []byte
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&row.Scope,
		&key,
		&row.Operation,
		&playerID,
		&row.RequestHash,
		&status,
		&resultJSON,
		&row.CreatedAt,
		&row.UpdatedAt,
		&completedAt,
	); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	row.Key = foundation.IdempotencyKey(key)
	row.PlayerID = foundation.PlayerID(playerID)
	row.Status = economy.IdempotencyStatus(status)
	row.ResultJSON = append(json.RawMessage(nil), resultJSON...)
	row.CreatedAt = row.CreatedAt.UTC()
	row.UpdatedAt = row.UpdatedAt.UTC()
	if completedAt.Valid {
		row.CompletedAt = completedAt.Time.UTC()
	}
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	return row, nil
}

func scanOutboxRow(scanner rowScanner) (economy.OutboxRow, error) {
	var row economy.OutboxRow
	var idempotencyKey string
	var payloadJSON []byte
	var status string
	var leasedUntil sql.NullTime
	var publishedAt sql.NullTime
	if err := scanner.Scan(
		&row.OutboxID,
		&row.Topic,
		&row.EventType,
		&row.AggregateType,
		&row.AggregateID,
		&row.IdempotencyScope,
		&idempotencyKey,
		&payloadJSON,
		&status,
		&row.AvailableAt,
		&row.LeaseOwner,
		&leasedUntil,
		&row.AttemptCount,
		&row.MaxAttempts,
		&row.LastError,
		&row.CreatedAt,
		&row.UpdatedAt,
		&publishedAt,
	); err != nil {
		return economy.OutboxRow{}, err
	}
	row.IdempotencyKey = foundation.IdempotencyKey(idempotencyKey)
	row.PayloadJSON = append(json.RawMessage(nil), payloadJSON...)
	row.Status = economy.OutboxStatus(status)
	row.AvailableAt = row.AvailableAt.UTC()
	row.CreatedAt = row.CreatedAt.UTC()
	row.UpdatedAt = row.UpdatedAt.UTC()
	if leasedUntil.Valid {
		row.LeasedUntil = leasedUntil.Time.UTC()
	}
	if publishedAt.Valid {
		row.PublishedAt = publishedAt.Time.UTC()
	}
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, err
	}
	return row, nil
}

func idempotencyKeySelectSQL() string {
	return `
		SELECT
			scope,
			idempotency_key,
			operation,
			player_id,
			request_hash,
			status,
			result_json,
			created_at,
			updated_at,
			completed_at
	`
}

func outboxSelectSQL() string {
	return `
		SELECT
			` + outboxColumnsSQL() + `
		FROM outbox
	`
}

func outboxReturningSQL() string {
	return `
		RETURNING
			` + outboxColumnsSQL() + `
	`
}

func outboxColumnsSQL() string {
	return `
			outbox_id,
			topic,
			event_type,
			aggregate_type,
			aggregate_id,
			idempotency_scope,
			idempotency_key,
			payload_json,
			status,
			available_at,
			lease_owner,
			leased_until,
			attempt_count,
			max_attempts,
			last_error,
			created_at,
			updated_at,
			published_at
	`
}

func normalizeIdempotencyKeyRow(row economy.IdempotencyKeyRow) economy.IdempotencyKeyRow {
	row = row.Clone()
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	} else {
		row.CreatedAt = row.CreatedAt.UTC()
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	} else {
		row.UpdatedAt = row.UpdatedAt.UTC()
	}
	if !row.CompletedAt.IsZero() {
		row.CompletedAt = row.CompletedAt.UTC()
	}
	return row
}

func normalizeOutboxRowTimes(row economy.OutboxRow) economy.OutboxRow {
	row = row.Clone()
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	} else {
		row.CreatedAt = row.CreatedAt.UTC()
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	} else {
		row.UpdatedAt = row.UpdatedAt.UTC()
	}
	if row.AvailableAt.IsZero() {
		row.AvailableAt = row.CreatedAt
	} else {
		row.AvailableAt = row.AvailableAt.UTC()
	}
	if !row.LeasedUntil.IsZero() {
		row.LeasedUntil = row.LeasedUntil.UTC()
	}
	if !row.PublishedAt.IsZero() {
		row.PublishedAt = row.PublishedAt.UTC()
	}
	return row
}

func zeroTimeAsNil(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

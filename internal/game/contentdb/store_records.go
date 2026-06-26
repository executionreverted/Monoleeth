package contentdb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/content"
)

type VersionStatus string

const (
	VersionStatusDraft      VersionStatus = "draft"
	VersionStatusPublished  VersionStatus = "published"
	VersionStatusArchived   VersionStatus = "archived"
	VersionStatusRolledBack VersionStatus = "rolled_back"
)

type PublishedSnapshotInput = content.PublishSnapshotInput

type PublishedSnapshot struct {
	ID                   string
	Version              string
	Snapshot             content.Snapshot
	ValidationReportJSON json.RawMessage
	Notes                string
	BalanceTag           string
	PublishedAt          time.Time
}

func (store *Store) ListContentVersions(ctx context.Context, input content.VersionListInput) (content.VersionList, error) {
	if store == nil || store.db == nil {
		return content.VersionList{}, ErrNilDatabase
	}
	input = content.NormalizeVersionListInput(input)
	var total int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM content_versions`).Scan(&total); err != nil {
		return content.VersionList{}, err
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			id::text,
			version,
			status,
			is_current,
			notes,
			balance_tag,
			COALESCE(created_by, ''),
			created_at,
			COALESCE(published_by, ''),
			published_at,
			COALESCE(rolled_back_from::text, '')
		FROM content_versions
		ORDER BY is_current DESC, published_at DESC NULLS LAST, created_at DESC, version DESC
		LIMIT $1 OFFSET $2
	`, input.Limit, input.Offset)
	if err != nil {
		return content.VersionList{}, err
	}
	defer rows.Close()
	out := content.VersionList{
		Total:  total,
		Limit:  input.Limit,
		Offset: input.Offset,
	}
	for rows.Next() {
		var version content.VersionSummary
		var publishedAt sql.NullTime
		if err := rows.Scan(
			&version.ID,
			&version.Version,
			&version.Status,
			&version.Current,
			&version.Notes,
			&version.BalanceTag,
			&version.CreatedBy,
			&version.CreatedAt,
			&version.PublishedBy,
			&publishedAt,
			&version.RolledBackFrom,
		); err != nil {
			return content.VersionList{}, err
		}
		if publishedAt.Valid {
			version.PublishedAt = publishedAt.Time
		}
		out.Versions = append(out.Versions, version)
	}
	if err := rows.Err(); err != nil {
		return content.VersionList{}, err
	}
	return out, nil
}

type DraftContentRow = content.DraftRow

type AuditEntry struct {
	ID               string
	ContentVersionID string
	ContentType      content.ContentType
	ContentID        content.ContentID
	Action           string
	FieldPath        string
	OldValueJSON     json.RawMessage
	NewValueJSON     json.RawMessage
	ActorAccountID   string
	Note             string
	BalanceTag       string
}

func (store *Store) HasAnyContent(ctx context.Context) (bool, error) {
	if store == nil || store.db == nil {
		return false, ErrNilDatabase
	}
	var exists bool
	if err := store.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM content_versions)`).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (store *Store) LoadCurrentPublishedSnapshot(ctx context.Context) (PublishedSnapshot, error) {
	record, err := store.LoadCurrentContentSnapshot(ctx)
	if err != nil {
		return PublishedSnapshot{}, err
	}
	return PublishedSnapshot{
		ID:                   record.ID,
		Version:              record.Version,
		Snapshot:             record.Snapshot,
		ValidationReportJSON: record.ValidationReportJSON,
		Notes:                record.Notes,
		BalanceTag:           record.BalanceTag,
		PublishedAt:          record.PublishedAt,
	}, nil
}

func (store *Store) LoadCurrentContentSnapshot(ctx context.Context) (content.SnapshotVersionRecord, error) {
	if store == nil || store.db == nil {
		return content.SnapshotVersionRecord{}, ErrNilDatabase
	}
	record, err := scanSnapshotVersionRecord(store.db.QueryRowContext(ctx, `
		SELECT
			id::text,
			version,
			status,
			is_current,
			snapshot_json,
			validation_report_json,
			notes,
			balance_tag,
			COALESCE(created_by, ''),
			created_at,
			COALESCE(published_by, ''),
			published_at,
			COALESCE(rolled_back_from::text, '')
		FROM content_versions
		WHERE is_current = true AND status = 'published'
	`))
	if err == sql.ErrNoRows {
		return content.SnapshotVersionRecord{}, ErrCurrentContentNotFound
	}
	if err != nil {
		return content.SnapshotVersionRecord{}, err
	}
	return record, nil
}

func (store *Store) LoadContentSnapshotByID(ctx context.Context, id string) (content.SnapshotVersionRecord, error) {
	if store == nil || store.db == nil {
		return content.SnapshotVersionRecord{}, ErrNilDatabase
	}
	if err := content.ValidateContentID("content version", id); err != nil {
		return content.SnapshotVersionRecord{}, err
	}
	record, err := scanSnapshotVersionRecord(store.db.QueryRowContext(ctx, `
		SELECT
			id::text,
			version,
			status,
			is_current,
			snapshot_json,
			validation_report_json,
			notes,
			balance_tag,
			COALESCE(created_by, ''),
			created_at,
			COALESCE(published_by, ''),
			published_at,
			COALESCE(rolled_back_from::text, '')
		FROM content_versions
		WHERE id = $1::uuid
	`, id))
	if err == sql.ErrNoRows {
		return content.SnapshotVersionRecord{}, ErrCurrentContentNotFound
	}
	return record, err
}

func (store *Store) InsertPublishedSnapshot(ctx context.Context, input PublishedSnapshotInput) error {
	_, err := store.PublishContentSnapshot(ctx, content.PublishSnapshotInput(input))
	return err
}

func (store *Store) PublishContentSnapshot(ctx context.Context, input content.PublishSnapshotInput) (result content.PublishSnapshotResult, err error) {
	if store == nil || store.db == nil {
		return content.PublishSnapshotResult{}, ErrNilDatabase
	}
	if err := content.ValidateContentID("content version row", input.ID); err != nil {
		return content.PublishSnapshotResult{}, err
	}
	if err := input.Snapshot.Validate(); err != nil {
		return content.PublishSnapshotResult{}, err
	}
	if input.Version == "" {
		input.Version = input.Snapshot.Version
	}
	if err := content.ValidateContentID("content version", input.Version); err != nil {
		return content.PublishSnapshotResult{}, err
	}
	if len(input.ValidationReportJSON) == 0 {
		input.ValidationReportJSON = json.RawMessage(`{}`)
	}
	if input.PublishedAt.IsZero() {
		input.PublishedAt = time.Now().UTC()
	}
	snapshotJSON, err := json.Marshal(input.Snapshot)
	if err != nil {
		return content.PublishSnapshotResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return content.PublishSnapshotResult{}, err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('content_publish_current'))`); err != nil {
		return content.PublishSnapshotResult{}, err
	}
	if input.IdempotencyKey != "" {
		existing, ok, loadErr := loadSnapshotVersionByIdempotencyTx(ctx, tx, input.IdempotencyKey)
		if loadErr != nil {
			err = loadErr
			return content.PublishSnapshotResult{}, err
		}
		if ok {
			if commitErr := tx.Commit(); commitErr != nil {
				err = commitErr
				return content.PublishSnapshotResult{}, err
			}
			return content.PublishSnapshotResult{Record: existing, Idempotent: true}, nil
		}
	}
	if input.ExpectedCurrentID != "" {
		current, loadErr := loadCurrentSnapshotVersionTx(ctx, tx)
		if loadErr != nil {
			err = loadErr
			return content.PublishSnapshotResult{}, err
		}
		if current.ID != input.ExpectedCurrentID {
			err = ErrContentPublishConflict
			return content.PublishSnapshotResult{}, err
		}
	}
	insertResult, err := tx.ExecContext(ctx, `
		INSERT INTO content_versions(
			id, version, status, is_current, idempotency_key, snapshot_json,
			validation_report_json, notes, balance_tag, created_by, published_by, published_at,
			rolled_back_from
		)
		VALUES ($1::uuid, $2, 'published', false, NULLIF($3, ''), $4::jsonb, $5::jsonb, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10, NULLIF($11, '')::uuid)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, input.ID, input.Version, input.IdempotencyKey, snapshotJSON, input.ValidationReportJSON, input.Notes, input.BalanceTag, input.CreatedBy, input.PublishedBy, input.PublishedAt, input.RolledBackFrom)
	if err != nil {
		return content.PublishSnapshotResult{}, err
	}
	affected, err := insertResult.RowsAffected()
	if err != nil {
		return content.PublishSnapshotResult{}, err
	}
	if affected == 0 {
		existing, ok, loadErr := loadSnapshotVersionByIdempotencyTx(ctx, tx, input.IdempotencyKey)
		if loadErr != nil {
			err = loadErr
			return content.PublishSnapshotResult{}, err
		}
		if !ok {
			err = sql.ErrNoRows
			return content.PublishSnapshotResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return content.PublishSnapshotResult{}, err
		}
		return content.PublishSnapshotResult{Record: existing, Idempotent: true}, nil
	}
	previousStatus := string(VersionStatusArchived)
	if input.RolledBackFrom != "" {
		previousStatus = string(VersionStatusRolledBack)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE content_versions SET is_current = false, status = $1 WHERE is_current = true`, previousStatus); err != nil {
		return content.PublishSnapshotResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE content_versions SET is_current = true WHERE id = $1::uuid`, input.ID); err != nil {
		return content.PublishSnapshotResult{}, err
	}
	for _, entry := range input.AuditEntries {
		if err = insertAuditTx(ctx, tx, entry); err != nil {
			return content.PublishSnapshotResult{}, err
		}
	}
	record, err := loadSnapshotVersionByIDTx(ctx, tx, input.ID)
	if err != nil {
		return content.PublishSnapshotResult{}, err
	}
	err = tx.Commit()
	if err != nil {
		return content.PublishSnapshotResult{}, err
	}
	return content.PublishSnapshotResult{Record: record}, nil
}

func (store *Store) UpsertDraftRow(ctx context.Context, contentType content.ContentType, row DraftContentRow) error {
	return store.UpsertDraftRows(ctx, contentType, row.DraftVersion, []content.SnapshotRow{{
		ContentID:   row.ContentID,
		Enabled:     row.Enabled,
		DisplayJSON: row.DisplayJSON,
		DataJSON:    row.DataJSON,
	}}, row.UpdatedBy)
}

func (store *Store) UpsertDraftRows(ctx context.Context, contentType content.ContentType, draftVersion string, rows []content.SnapshotRow, updatedBy string) error {
	if store == nil || store.db == nil {
		return ErrNilDatabase
	}
	tableName, err := ContentTableName(contentType)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	query := fmt.Sprintf(`
		INSERT INTO %s(content_id, draft_version, enabled, display_json, data_json, updated_by)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4::jsonb, $5::jsonb, NULLIF($6, ''))
		ON CONFLICT (content_id) DO UPDATE SET
			draft_version = EXCLUDED.draft_version,
			enabled = EXCLUDED.enabled,
			display_json = EXCLUDED.display_json,
			data_json = EXCLUDED.data_json,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
	`, tableName)
	for _, row := range rows {
		if validateErr := content.ValidateContentID(string(contentType), string(row.ContentID)); validateErr != nil {
			err = validateErr
			return err
		}
		displayJSON := row.DisplayJSON
		if len(displayJSON) == 0 {
			displayJSON = json.RawMessage(`{}`)
		}
		if _, err = tx.ExecContext(ctx, query, row.ContentID, draftVersion, row.Enabled, displayJSON, row.DataJSON, updatedBy); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (store *Store) LoadDraftRows(ctx context.Context, contentType content.ContentType) ([]DraftContentRow, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	tableName, err := ContentTableName(contentType)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT content_id, COALESCE(draft_version::text, ''), enabled, display_json, data_json, COALESCE(updated_by, '')
		FROM %s
		ORDER BY content_id
	`, tableName)
	rows, err := store.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DraftContentRow
	for rows.Next() {
		var row DraftContentRow
		var contentID string
		var displayJSON []byte
		var dataJSON []byte
		if err := rows.Scan(&contentID, &row.DraftVersion, &row.Enabled, &displayJSON, &dataJSON, &row.UpdatedBy); err != nil {
			return nil, err
		}
		row.ContentID = content.ContentID(contentID)
		row.DisplayJSON = append(json.RawMessage(nil), displayJSON...)
		row.DataJSON = append(json.RawMessage(nil), dataJSON...)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (store *Store) InsertAudit(ctx context.Context, entry AuditEntry) error {
	if store == nil || store.db == nil {
		return ErrNilDatabase
	}
	return insertAuditExec(ctx, store.db, content.AuditLogEntryInput{
		ID:               entry.ID,
		ContentVersionID: entry.ContentVersionID,
		ContentType:      entry.ContentType,
		ContentID:        entry.ContentID,
		Action:           entry.Action,
		FieldPath:        entry.FieldPath,
		OldValueJSON:     entry.OldValueJSON,
		NewValueJSON:     entry.NewValueJSON,
		ActorAccountID:   entry.ActorAccountID,
		Note:             entry.Note,
		BalanceTag:       entry.BalanceTag,
	})
}

func (store *Store) ListContentAudit(ctx context.Context, input content.AuditLogInput) (content.AuditLog, error) {
	if store == nil || store.db == nil {
		return content.AuditLog{}, ErrNilDatabase
	}
	input = content.NormalizeAuditLogInput(input)
	if input.ContentType != "" {
		if _, err := ContentTableName(input.ContentType); err != nil {
			return content.AuditLog{}, err
		}
	}
	var total int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM content_audit_log
		WHERE ($1 = '' OR COALESCE(content_version_id::text, '') = $1)
			AND ($2 = '' OR content_type = $2)
			AND ($3 = '' OR content_id = $3)
			AND ($4 = '' OR action = $4)
	`, input.VersionID, input.ContentType, input.ContentID, input.Action).Scan(&total); err != nil {
		return content.AuditLog{}, err
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT
			id::text,
			COALESCE(content_version_id::text, ''),
			content_type,
			content_id,
			action,
			field_path,
			old_value_json,
			new_value_json,
			COALESCE(actor_account_id, ''),
			note,
			balance_tag,
			created_at
		FROM content_audit_log
		WHERE ($1 = '' OR COALESCE(content_version_id::text, '') = $1)
			AND ($2 = '' OR content_type = $2)
			AND ($3 = '' OR content_id = $3)
			AND ($4 = '' OR action = $4)
		ORDER BY created_at DESC, id DESC
		LIMIT $5 OFFSET $6
	`, input.VersionID, input.ContentType, input.ContentID, input.Action, input.Limit, input.Offset)
	if err != nil {
		return content.AuditLog{}, err
	}
	defer rows.Close()
	out := content.AuditLog{
		Total:  total,
		Limit:  input.Limit,
		Offset: input.Offset,
	}
	for rows.Next() {
		var entry content.AuditLogEntry
		var contentType string
		var contentID string
		var oldValue []byte
		var newValue []byte
		if err := rows.Scan(
			&entry.ID,
			&entry.ContentVersionID,
			&contentType,
			&contentID,
			&entry.Action,
			&entry.FieldPath,
			&oldValue,
			&newValue,
			&entry.ActorAccountID,
			&entry.Note,
			&entry.BalanceTag,
			&entry.CreatedAt,
		); err != nil {
			return content.AuditLog{}, err
		}
		entry.ContentType = content.ContentType(contentType)
		entry.ContentID = content.ContentID(contentID)
		entry.OldValueJSON = append(json.RawMessage(nil), oldValue...)
		entry.NewValueJSON = append(json.RawMessage(nil), newValue...)
		out.Entries = append(out.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return content.AuditLog{}, err
	}
	return out, nil
}

type auditExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertAuditTx(ctx context.Context, tx *sql.Tx, entry content.AuditLogEntryInput) error {
	return insertAuditExec(ctx, tx, entry)
}

func insertAuditExec(ctx context.Context, execer auditExecer, entry content.AuditLogEntryInput) error {
	if err := content.ValidateContentID("content audit", entry.ID); err != nil {
		return err
	}
	if _, err := ContentTableName(entry.ContentType); err != nil {
		return err
	}
	if err := content.ValidateContentID(string(entry.ContentType), string(entry.ContentID)); err != nil {
		return err
	}
	action := entry.Action
	if action == "" {
		action = content.AuditActionPublish
	}
	if !content.IsKnownAuditAction(action) {
		return fmt.Errorf("content audit action %q: %w", action, ErrUnknownAuditAction)
	}
	oldValue, err := nullableAuditJSON("old_value_json", entry.OldValueJSON)
	if err != nil {
		return err
	}
	newValue, err := nullableAuditJSON("new_value_json", entry.NewValueJSON)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO content_audit_log(
			id, content_version_id, content_type, content_id, action, field_path, old_value_json,
			new_value_json, actor_account_id, note, balance_tag
		)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7::jsonb, $8::jsonb, NULLIF($9, ''), $10, $11)
	`, entry.ID, entry.ContentVersionID, entry.ContentType, entry.ContentID, action, entry.FieldPath, oldValue, newValue, entry.ActorAccountID, entry.Note, entry.BalanceTag)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSnapshotVersionRecord(row rowScanner) (content.SnapshotVersionRecord, error) {
	var record content.SnapshotVersionRecord
	var snapshotJSON []byte
	var validationJSON []byte
	var publishedAt sql.NullTime
	if err := row.Scan(
		&record.ID,
		&record.Version,
		&record.Status,
		&record.Current,
		&snapshotJSON,
		&validationJSON,
		&record.Notes,
		&record.BalanceTag,
		&record.CreatedBy,
		&record.CreatedAt,
		&record.PublishedBy,
		&publishedAt,
		&record.RolledBackFrom,
	); err != nil {
		return content.SnapshotVersionRecord{}, err
	}
	if publishedAt.Valid {
		record.PublishedAt = publishedAt.Time
	}
	if err := json.Unmarshal(snapshotJSON, &record.Snapshot); err != nil {
		return content.SnapshotVersionRecord{}, err
	}
	if err := record.Snapshot.Validate(); err != nil {
		return content.SnapshotVersionRecord{}, err
	}
	record.ValidationReportJSON = append(json.RawMessage(nil), validationJSON...)
	return record, nil
}

func loadSnapshotVersionByIDTx(ctx context.Context, tx *sql.Tx, id string) (content.SnapshotVersionRecord, error) {
	return scanSnapshotVersionRecord(tx.QueryRowContext(ctx, snapshotVersionSelectSQL()+` WHERE id = $1::uuid`, id))
}

func loadCurrentSnapshotVersionTx(ctx context.Context, tx *sql.Tx) (content.SnapshotVersionRecord, error) {
	record, err := scanSnapshotVersionRecord(tx.QueryRowContext(ctx, snapshotVersionSelectSQL()+` WHERE is_current = true AND status = 'published' FOR UPDATE`))
	if err == sql.ErrNoRows {
		return content.SnapshotVersionRecord{}, ErrCurrentContentNotFound
	}
	return record, err
}

func loadSnapshotVersionByIdempotencyTx(ctx context.Context, tx *sql.Tx, idempotencyKey string) (content.SnapshotVersionRecord, bool, error) {
	record, err := scanSnapshotVersionRecord(tx.QueryRowContext(ctx, snapshotVersionSelectSQL()+` WHERE idempotency_key = $1`, idempotencyKey))
	if err == sql.ErrNoRows {
		return content.SnapshotVersionRecord{}, false, nil
	}
	if err != nil {
		return content.SnapshotVersionRecord{}, false, err
	}
	return record, true, nil
}

func snapshotVersionSelectSQL() string {
	return `
		SELECT
			id::text,
			version,
			status,
			is_current,
			snapshot_json,
			validation_report_json,
			notes,
			balance_tag,
			COALESCE(created_by, ''),
			created_at,
			COALESCE(published_by, ''),
			published_at,
			COALESCE(rolled_back_from::text, '')
		FROM content_versions
	`
}

func ContentTableName(contentType content.ContentType) (string, error) {
	table, ok := contentTableNames[contentType]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownContentType, contentType)
	}
	return table, nil
}

var contentTableNames = map[content.ContentType]string{
	content.ContentTypeItem:               "content_items",
	content.ContentTypeModule:             "content_modules",
	content.ContentTypeShip:               "content_ships",
	content.ContentTypeShopProduct:        "content_shop_products",
	content.ContentTypeNPCTemplate:        "content_npc_templates",
	content.ContentTypeSpawnArea:          "content_spawn_areas",
	content.ContentTypeEnemyPool:          "content_enemy_pools",
	content.ContentTypeNPCDropProfile:     "content_npc_drop_profiles",
	content.ContentTypeNPCAggroProfile:    "content_npc_aggro_profiles",
	content.ContentTypeNPCLeashProfile:    "content_npc_leash_profiles",
	content.ContentTypeNPCEventSpawn:      "content_npc_event_spawns",
	content.ContentTypeLootTable:          "content_loot_tables",
	content.ContentTypeCraftRecipe:        "content_craft_recipes",
	content.ContentTypeProductionBuilding: "content_production_buildings",
	content.ContentTypeQuestTemplate:      "content_quest_templates",
	content.ContentTypeQuestRewardTable:   "content_quest_reward_tables",
}

func nullableAuditJSON(path string, raw json.RawMessage) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if !json.Valid(trimmed) {
		return nil, fmt.Errorf("%s: %w", path, content.ErrInvalidContentJSON)
	}
	return []byte(trimmed), nil
}

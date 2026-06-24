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

type PublishedSnapshotInput struct {
	ID                   string
	Version              string
	Snapshot             content.Snapshot
	ValidationReportJSON json.RawMessage
	IdempotencyKey       string
	Notes                string
	BalanceTag           string
	CreatedBy            string
	PublishedBy          string
	PublishedAt          time.Time
}

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

type DraftContentRow struct {
	ContentID    content.ContentID
	DraftVersion string
	Enabled      bool
	DisplayJSON  json.RawMessage
	DataJSON     json.RawMessage
	UpdatedBy    string
}

type AuditEntry struct {
	ID               string
	ContentVersionID string
	ContentType      content.ContentType
	ContentID        content.ContentID
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
	if store == nil || store.db == nil {
		return PublishedSnapshot{}, ErrNilDatabase
	}
	var record PublishedSnapshot
	var snapshotJSON []byte
	var validationJSON []byte
	err := store.db.QueryRowContext(ctx, `
		SELECT id::text, version, snapshot_json, validation_report_json, notes, balance_tag, published_at
		FROM content_versions
		WHERE is_current = true AND status = 'published'
	`).Scan(&record.ID, &record.Version, &snapshotJSON, &validationJSON, &record.Notes, &record.BalanceTag, &record.PublishedAt)
	if err == sql.ErrNoRows {
		return PublishedSnapshot{}, ErrCurrentContentNotFound
	}
	if err != nil {
		return PublishedSnapshot{}, err
	}
	if err := json.Unmarshal(snapshotJSON, &record.Snapshot); err != nil {
		return PublishedSnapshot{}, err
	}
	if err := record.Snapshot.Validate(); err != nil {
		return PublishedSnapshot{}, err
	}
	record.ValidationReportJSON = append(json.RawMessage(nil), validationJSON...)
	return record, nil
}

func (store *Store) InsertPublishedSnapshot(ctx context.Context, input PublishedSnapshotInput) error {
	if store == nil || store.db == nil {
		return ErrNilDatabase
	}
	if err := content.ValidateContentID("content version row", input.ID); err != nil {
		return err
	}
	if err := input.Snapshot.Validate(); err != nil {
		return err
	}
	if input.Version == "" {
		input.Version = input.Snapshot.Version
	}
	if err := content.ValidateContentID("content version", input.Version); err != nil {
		return err
	}
	if len(input.ValidationReportJSON) == 0 {
		input.ValidationReportJSON = json.RawMessage(`{}`)
	}
	if input.PublishedAt.IsZero() {
		input.PublishedAt = time.Now().UTC()
	}
	snapshotJSON, err := json.Marshal(input.Snapshot)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if _, err = tx.ExecContext(ctx, `UPDATE content_versions SET is_current = false WHERE is_current = true`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO content_versions(
			id, version, status, is_current, idempotency_key, snapshot_json,
			validation_report_json, notes, balance_tag, created_by, published_by, published_at
		)
		VALUES ($1::uuid, $2, 'published', true, NULLIF($3, ''), $4::jsonb, $5::jsonb, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10)
	`, input.ID, input.Version, input.IdempotencyKey, snapshotJSON, input.ValidationReportJSON, input.Notes, input.BalanceTag, input.CreatedBy, input.PublishedBy, input.PublishedAt); err != nil {
		return err
	}
	err = tx.Commit()
	return err
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
	if err := content.ValidateContentID("content audit", entry.ID); err != nil {
		return err
	}
	if _, err := ContentTableName(entry.ContentType); err != nil {
		return err
	}
	if err := content.ValidateContentID(string(entry.ContentType), string(entry.ContentID)); err != nil {
		return err
	}
	oldValue, err := nullableAuditJSON("old_value_json", entry.OldValueJSON)
	if err != nil {
		return err
	}
	newValue, err := nullableAuditJSON("new_value_json", entry.NewValueJSON)
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, `
		INSERT INTO content_audit_log(
			id, content_version_id, content_type, content_id, field_path, old_value_json,
			new_value_json, actor_account_id, note, balance_tag
		)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6::jsonb, $7::jsonb, NULLIF($8, ''), $9, $10)
	`, entry.ID, entry.ContentVersionID, entry.ContentType, entry.ContentID, entry.FieldPath, oldValue, newValue, entry.ActorAccountID, entry.Note, entry.BalanceTag)
	return err
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

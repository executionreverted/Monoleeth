package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
)

type adminContentVersionsPayload struct {
	Versions    []adminContentVersionPayload `json:"versions"`
	Total       int                          `json:"total"`
	Limit       int                          `json:"limit"`
	Offset      int                          `json:"offset"`
	GeneratedAt int64                        `json:"generated_at"`
}

type adminContentVersionPayload struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	Status         string `json:"status"`
	Current        bool   `json:"current"`
	Notes          string `json:"notes,omitempty"`
	BalanceTag     string `json:"balance_tag,omitempty"`
	CreatedBy      string `json:"created_by,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	PublishedBy    string `json:"published_by,omitempty"`
	PublishedAt    int64  `json:"published_at,omitempty"`
	RolledBackFrom string `json:"rolled_back_from,omitempty"`
}

type adminContentDraftListPayload struct {
	ContentType string                        `json:"content_type"`
	Rows        []adminContentDraftRowPayload `json:"rows"`
	Total       int                           `json:"total"`
	Limit       int                           `json:"limit"`
	Offset      int                           `json:"offset"`
	GeneratedAt int64                         `json:"generated_at"`
}

type adminContentDraftRowPayload struct {
	ContentType  string          `json:"content_type,omitempty"`
	ContentID    string          `json:"content_id"`
	DraftVersion string          `json:"draft_version,omitempty"`
	Enabled      bool            `json:"enabled"`
	DisplayJSON  json.RawMessage `json:"display_json"`
	DataJSON     json.RawMessage `json:"data_json"`
	UpdatedBy    string          `json:"updated_by,omitempty"`
}

type adminContentDraftValidationPayload struct {
	Valid     bool                                      `json:"valid"`
	Version   string                                    `json:"version"`
	CheckedAt int64                                     `json:"checked_at"`
	Issues    []adminContentDraftValidationIssuePayload `json:"issues"`
}

type adminContentDraftValidationIssuePayload struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type adminContentPublishPayload struct {
	Published  bool                               `json:"published"`
	Idempotent bool                               `json:"idempotent"`
	RowCount   int                                `json:"row_count"`
	Version    adminContentVersionPayload         `json:"version"`
	Validation adminContentDraftValidationPayload `json:"validation"`
}

type adminContentRollbackPayload struct {
	RolledBack      bool                               `json:"rolled_back"`
	Idempotent      bool                               `json:"idempotent"`
	TargetVersionID string                             `json:"target_version_id"`
	Version         adminContentVersionPayload         `json:"version"`
	Validation      adminContentDraftValidationPayload `json:"validation"`
}

type adminContentAuditLogPayload struct {
	Entries     []adminContentAuditEntryPayload `json:"entries"`
	Total       int                             `json:"total"`
	Limit       int                             `json:"limit"`
	Offset      int                             `json:"offset"`
	GeneratedAt int64                           `json:"generated_at"`
}

type adminContentAuditEntryPayload struct {
	ID               string          `json:"id"`
	ContentVersionID string          `json:"content_version_id,omitempty"`
	ContentType      string          `json:"content_type"`
	ContentID        string          `json:"content_id"`
	FieldPath        string          `json:"field_path"`
	OldValueJSON     json.RawMessage `json:"old_value_json,omitempty"`
	NewValueJSON     json.RawMessage `json:"new_value_json,omitempty"`
	ActorRef         string          `json:"actor_ref,omitempty"`
	Note             string          `json:"note,omitempty"`
	BalanceTag       string          `json:"balance_tag,omitempty"`
	CreatedAt        int64           `json:"created_at"`
}

func (runtime *Runtime) handleAdminContentList(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Content rows are restricted."); err != nil {
		return nil, err
	}
	var payload struct {
		ContentType string `json:"content_type"`
		Limit       int    `json:"limit,omitempty"`
		Offset      int    `json:"offset,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	contentType := content.ContentType(payload.ContentType)
	list, err := runtime.ContentAdmin.ListDraftRows(context.Background(), content.DraftListInput{
		ContentType: contentType,
		Limit:       payload.Limit,
		Offset:      payload.Offset,
	})
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content rows unavailable.")
	}
	return marshalPayload(map[string]any{"content": adminContentDraftListPayloadFromList(list)})
}

func (runtime *Runtime) handleAdminContentGet(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Content rows are restricted."); err != nil {
		return nil, err
	}
	var payload struct {
		ContentType string `json:"content_type"`
		ContentID   string `json:"content_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	contentType := content.ContentType(payload.ContentType)
	row, err := runtime.ContentAdmin.GetDraftRow(context.Background(), contentType, content.ContentID(payload.ContentID))
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content row unavailable.")
	}
	return marshalPayload(map[string]any{"content_row": adminContentDraftRowPayloadFromRow(contentType, row)})
}

func (runtime *Runtime) handleAdminContentUpdateDraft(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectAdminContentControlPayload(request.Payload); err != nil {
		return nil, err
	}
	resolved, err := runtime.requireAdmin(ctx, "Content draft edits are restricted.")
	if err != nil {
		return nil, err
	}
	var payload struct {
		ContentType string          `json:"content_type"`
		ContentID   string          `json:"content_id"`
		Enabled     *bool           `json:"enabled"`
		DisplayJSON json.RawMessage `json:"display_json,omitempty"`
		DataJSON    json.RawMessage `json:"data_json"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.Enabled == nil {
		return nil, invalidPayload("Content enabled flag is required.", nil)
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	contentType := content.ContentType(payload.ContentType)
	row, err := runtime.ContentAdmin.UpdateDraftRow(context.Background(), content.DraftUpdateInput{
		ContentType: contentType,
		ContentID:   content.ContentID(payload.ContentID),
		Enabled:     *payload.Enabled,
		DisplayJSON: payload.DisplayJSON,
		DataJSON:    payload.DataJSON,
		UpdatedBy:   string(resolved.AccountID),
	})
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content draft update failed.")
	}
	return marshalPayload(map[string]any{"content_row": adminContentDraftRowPayloadFromRow(contentType, row)})
}

func (runtime *Runtime) handleAdminContentValidateDraft(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectAdminContentControlPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Content draft validation is restricted."); err != nil {
		return nil, err
	}
	var payload struct {
		Version string `json:"version,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	report, err := runtime.ContentAdmin.ValidateDraft(context.Background(), content.DraftValidationInput{Version: payload.Version})
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content draft validation failed.")
	}
	return marshalPayload(map[string]any{"validation": adminContentDraftValidationPayloadFromReport(report)})
}

func (runtime *Runtime) handleAdminContentPublish(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	resolved, err := runtime.requireAdmin(ctx, "Content publish is restricted.")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Version    string `json:"version,omitempty"`
		Notes      string `json:"notes,omitempty"`
		BalanceTag string `json:"balance_tag,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	result, err := runtime.ContentAdmin.PublishDraft(context.Background(), content.PublishDraftInput{
		Version:        payload.Version,
		Notes:          payload.Notes,
		BalanceTag:     payload.BalanceTag,
		ActorAccountID: string(resolved.AccountID),
	})
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content publish failed.")
	}
	return marshalPayload(map[string]any{"content_publish": adminContentPublishPayloadFromResult(result)})
}

func (runtime *Runtime) handleAdminContentRollback(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	resolved, err := runtime.requireAdmin(ctx, "Content rollback is restricted.")
	if err != nil {
		return nil, err
	}
	var payload struct {
		TargetVersionID string `json:"target_version_id"`
		Version         string `json:"version,omitempty"`
		Notes           string `json:"notes,omitempty"`
		BalanceTag      string `json:"balance_tag,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	result, err := runtime.ContentAdmin.Rollback(context.Background(), content.RollbackInput{
		TargetVersionID: payload.TargetVersionID,
		Version:         payload.Version,
		Notes:           payload.Notes,
		BalanceTag:      payload.BalanceTag,
		ActorAccountID:  string(resolved.AccountID),
		IdempotencyKey:  fmt.Sprintf("content_rollback:%s:%s", strings.TrimSpace(payload.TargetVersionID), request.RequestID),
	})
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content rollback failed.")
	}
	return marshalPayload(map[string]any{"content_rollback": adminContentRollbackPayloadFromResult(payload.TargetVersionID, result)})
}

func (runtime *Runtime) handleAdminContentAuditLog(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Content audit log is restricted."); err != nil {
		return nil, err
	}
	var payload struct {
		VersionID   string `json:"version_id,omitempty"`
		ContentType string `json:"content_type,omitempty"`
		ContentID   string `json:"content_id,omitempty"`
		Limit       int    `json:"limit,omitempty"`
		Offset      int    `json:"offset,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	log, err := runtime.ContentAdmin.AuditLog(context.Background(), content.AuditLogInput{
		VersionID:   payload.VersionID,
		ContentType: content.ContentType(payload.ContentType),
		ContentID:   content.ContentID(payload.ContentID),
		Limit:       payload.Limit,
		Offset:      payload.Offset,
	})
	if err != nil {
		return nil, domainErrorForContentAdmin(err, "Content audit log unavailable.")
	}
	return marshalPayload(map[string]any{"content_audit_log": adminContentAuditLogPayloadFromLog(log)})
}

func (runtime *Runtime) handleAdminContentVersions(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Content versions are restricted."); err != nil {
		return nil, err
	}
	var payload struct {
		Limit  int `json:"limit,omitempty"`
		Offset int `json:"offset,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.ContentAdmin == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content admin service unavailable.")
	}
	versions, err := runtime.ContentAdmin.ListVersions(context.Background(), content.VersionListInput{
		Limit:  payload.Limit,
		Offset: payload.Offset,
	})
	if err != nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Content versions unavailable.", foundation.WithCause(err))
	}
	return marshalPayload(map[string]any{"content_versions": adminContentVersionsPayloadFromList(versions)})
}

func adminContentDraftListPayloadFromList(list content.DraftList) adminContentDraftListPayload {
	payload := adminContentDraftListPayload{
		ContentType: string(list.ContentType),
		Total:       list.Total,
		Limit:       list.Limit,
		Offset:      list.Offset,
		GeneratedAt: list.GeneratedAt.UTC().UnixMilli(),
		Rows:        make([]adminContentDraftRowPayload, 0, len(list.Rows)),
	}
	for _, row := range list.Rows {
		payload.Rows = append(payload.Rows, adminContentDraftRowPayloadFromRow(list.ContentType, row))
	}
	return payload
}

func adminContentDraftValidationPayloadFromReport(report content.DraftValidationReport) adminContentDraftValidationPayload {
	payload := adminContentDraftValidationPayload{
		Valid:     report.Valid,
		Version:   report.Version,
		CheckedAt: report.CheckedAt.UTC().UnixMilli(),
		Issues:    make([]adminContentDraftValidationIssuePayload, 0, len(report.Issues)),
	}
	for _, issue := range report.Issues {
		payload.Issues = append(payload.Issues, adminContentDraftValidationIssuePayload{
			Path:    issue.Path,
			Code:    issue.Code,
			Message: issue.Message,
		})
	}
	return payload
}

func adminContentPublishPayloadFromResult(result content.PublishDraftResult) adminContentPublishPayload {
	return adminContentPublishPayload{
		Published:  result.Published,
		Idempotent: result.Idempotent,
		RowCount:   result.RowCount,
		Version:    adminContentVersionPayloadFromSummary(result.Version),
		Validation: adminContentDraftValidationPayloadFromReport(result.Validation),
	}
}

func adminContentRollbackPayloadFromResult(targetVersionID string, result content.PublishDraftResult) adminContentRollbackPayload {
	return adminContentRollbackPayload{
		RolledBack:      result.Published,
		Idempotent:      result.Idempotent,
		TargetVersionID: targetVersionID,
		Version:         adminContentVersionPayloadFromSummary(result.Version),
		Validation:      adminContentDraftValidationPayloadFromReport(result.Validation),
	}
}

func adminContentAuditLogPayloadFromLog(log content.AuditLog) adminContentAuditLogPayload {
	payload := adminContentAuditLogPayload{
		Total:       log.Total,
		Limit:       log.Limit,
		Offset:      log.Offset,
		GeneratedAt: log.GeneratedAt.UTC().UnixMilli(),
		Entries:     make([]adminContentAuditEntryPayload, 0, len(log.Entries)),
	}
	for _, entry := range log.Entries {
		payload.Entries = append(payload.Entries, adminContentAuditEntryPayload{
			ID:               entry.ID,
			ContentVersionID: entry.ContentVersionID,
			ContentType:      string(entry.ContentType),
			ContentID:        string(entry.ContentID),
			FieldPath:        entry.FieldPath,
			OldValueJSON:     append(json.RawMessage(nil), entry.OldValueJSON...),
			NewValueJSON:     append(json.RawMessage(nil), entry.NewValueJSON...),
			ActorRef:         entry.ActorAccountID,
			Note:             entry.Note,
			BalanceTag:       entry.BalanceTag,
			CreatedAt:        entry.CreatedAt.UTC().UnixMilli(),
		})
	}
	return payload
}

func domainErrorForContentAdmin(err error, fallback string) error {
	if errors.Is(err, admin.ErrContentDraftNotFound) {
		return foundation.NewDomainError(foundation.CodeNotFound, "Content row was not found.", foundation.WithCause(err))
	}
	if errors.Is(err, contentdb.ErrCurrentContentNotFound) {
		return foundation.NewDomainError(foundation.CodeNotFound, "Content version was not found.", foundation.WithCause(err))
	}
	if errors.Is(err, contentdb.ErrContentPublishConflict) {
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Content version changed. Reload and retry.", foundation.WithCause(err))
	}
	if errors.Is(err, contentdb.ErrUnknownContentType) || errors.Is(err, content.ErrUnknownContentType) {
		return invalidPayload("Content type is invalid.", err)
	}
	if errors.Is(err, content.ErrInvalidContentJSON) ||
		errors.Is(err, content.ErrForbiddenContentField) ||
		errors.Is(err, content.ErrInvalidContentSnapshot) ||
		errors.Is(err, content.ErrDuplicateContentID) ||
		errors.Is(err, foundation.ErrEmptyID) ||
		errors.Is(err, foundation.ErrInvalidID) {
		return invalidPayload("Content draft payload is invalid.", err)
	}
	return foundation.NewDomainError(foundation.CodeInternal, fallback, foundation.WithCause(err))
}

func rejectAdminContentControlPayload(payload json.RawMessage) error {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return invalidPayload("Invalid payload.", err)
	}
	if found := findAdminContentTrustedPayloadKey(value, 0); found != "" {
		return invalidPayload(fmt.Sprintf("Payload field %q is server-owned.", found), nil)
	}
	return nil
}

func findAdminContentTrustedPayloadKey(value any, depth int) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(key)
			if normalized == "display_json" || normalized == "data_json" {
				continue
			}
			if _, forbidden := trustedClientPayloadKeys[normalized]; forbidden {
				return key
			}
			if normalized == "updated_by" {
				return key
			}
			if found := findAdminContentTrustedPayloadKey(child, depth+1); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findAdminContentTrustedPayloadKey(child, depth+1); found != "" {
				return found
			}
		}
	}
	return ""
}

func adminContentDraftRowPayloadFromRow(contentType content.ContentType, row content.DraftRow) adminContentDraftRowPayload {
	displayJSON := row.DisplayJSON
	if len(displayJSON) == 0 {
		displayJSON = json.RawMessage(`{}`)
	}
	dataJSON := row.DataJSON
	if len(dataJSON) == 0 {
		dataJSON = json.RawMessage(`{}`)
	}
	return adminContentDraftRowPayload{
		ContentType:  string(contentType),
		ContentID:    string(row.ContentID),
		DraftVersion: row.DraftVersion,
		Enabled:      row.Enabled,
		DisplayJSON:  append(json.RawMessage(nil), displayJSON...),
		DataJSON:     append(json.RawMessage(nil), dataJSON...),
		UpdatedBy:    row.UpdatedBy,
	}
}

func adminContentVersionPayloadFromSummary(version content.VersionSummary) adminContentVersionPayload {
	item := adminContentVersionPayload{
		ID:             version.ID,
		Version:        version.Version,
		Status:         version.Status,
		Current:        version.Current,
		Notes:          version.Notes,
		BalanceTag:     version.BalanceTag,
		CreatedBy:      version.CreatedBy,
		CreatedAt:      version.CreatedAt.UTC().UnixMilli(),
		PublishedBy:    version.PublishedBy,
		RolledBackFrom: version.RolledBackFrom,
	}
	if !version.PublishedAt.IsZero() {
		item.PublishedAt = version.PublishedAt.UTC().UnixMilli()
	}
	return item
}

func adminContentVersionsPayloadFromList(list content.VersionList) adminContentVersionsPayload {
	payload := adminContentVersionsPayload{
		Total:       list.Total,
		Limit:       list.Limit,
		Offset:      list.Offset,
		GeneratedAt: list.GeneratedAt.UTC().UnixMilli(),
		Versions:    make([]adminContentVersionPayload, 0, len(list.Versions)),
	}
	for _, version := range list.Versions {
		payload.Versions = append(payload.Versions, adminContentVersionPayloadFromSummary(version))
	}
	return payload
}

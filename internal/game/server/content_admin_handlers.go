package server

import (
	"context"
	"encoding/json"
	"errors"

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

func domainErrorForContentAdmin(err error, fallback string) error {
	if errors.Is(err, admin.ErrContentDraftNotFound) {
		return foundation.NewDomainError(foundation.CodeNotFound, "Content row was not found.", foundation.WithCause(err))
	}
	if errors.Is(err, contentdb.ErrUnknownContentType) {
		return invalidPayload("Content type is invalid.", err)
	}
	return foundation.NewDomainError(foundation.CodeInternal, fallback, foundation.WithCause(err))
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

func adminContentVersionsPayloadFromList(list content.VersionList) adminContentVersionsPayload {
	payload := adminContentVersionsPayload{
		Total:       list.Total,
		Limit:       list.Limit,
		Offset:      list.Offset,
		GeneratedAt: list.GeneratedAt.UTC().UnixMilli(),
		Versions:    make([]adminContentVersionPayload, 0, len(list.Versions)),
	}
	for _, version := range list.Versions {
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
		payload.Versions = append(payload.Versions, item)
	}
	return payload
}

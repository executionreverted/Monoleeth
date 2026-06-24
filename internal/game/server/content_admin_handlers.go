package server

import (
	"context"
	"encoding/json"

	"gameproject/internal/game/content"
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

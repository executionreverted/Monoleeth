package admin

import (
	"context"
	"errors"

	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

var ErrMissingContentVersionStore = errors.New("missing admin content version store")
var ErrMissingContentDraftStore = errors.New("missing admin content draft store")
var ErrContentDraftNotFound = errors.New("content draft row not found")

type ContentVersionStore interface {
	ListContentVersions(context.Context, content.VersionListInput) (content.VersionList, error)
}

type ContentDraftStore interface {
	LoadDraftRows(context.Context, content.ContentType) ([]content.DraftRow, error)
}

type ContentServiceConfig struct {
	Versions ContentVersionStore
	Drafts   ContentDraftStore
	Clock    foundation.Clock
}

type ContentService struct {
	versions ContentVersionStore
	drafts   ContentDraftStore
	clock    foundation.Clock
}

func NewContentService(config ContentServiceConfig) *ContentService {
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &ContentService{versions: config.Versions, drafts: config.Drafts, clock: clock}
}

func (service *ContentService) ListVersions(ctx context.Context, input content.VersionListInput) (content.VersionList, error) {
	if service == nil || service.versions == nil {
		return content.VersionList{}, ErrMissingContentVersionStore
	}
	input = content.NormalizeVersionListInput(input)
	list, err := service.versions.ListContentVersions(ctx, input)
	if err != nil {
		return content.VersionList{}, err
	}
	list.Limit = input.Limit
	list.Offset = input.Offset
	list.GeneratedAt = service.clock.Now().UTC()
	return list, nil
}

func (service *ContentService) ListDraftRows(ctx context.Context, input content.DraftListInput) (content.DraftList, error) {
	if service == nil || service.drafts == nil {
		return content.DraftList{}, ErrMissingContentDraftStore
	}
	input = content.NormalizeDraftListInput(input)
	rows, err := service.drafts.LoadDraftRows(ctx, input.ContentType)
	if err != nil {
		return content.DraftList{}, err
	}
	total := len(rows)
	start := min(input.Offset, total)
	end := min(start+input.Limit, total)
	return content.DraftList{
		ContentType: input.ContentType,
		Rows:        cloneDraftRows(rows[start:end]),
		Total:       total,
		Limit:       input.Limit,
		Offset:      input.Offset,
		GeneratedAt: service.clock.Now().UTC(),
	}, nil
}

func (service *ContentService) GetDraftRow(ctx context.Context, contentType content.ContentType, contentID content.ContentID) (content.DraftRow, error) {
	if service == nil || service.drafts == nil {
		return content.DraftRow{}, ErrMissingContentDraftStore
	}
	if err := content.ValidateContentID(string(contentType), string(contentID)); err != nil {
		return content.DraftRow{}, err
	}
	rows, err := service.drafts.LoadDraftRows(ctx, contentType)
	if err != nil {
		return content.DraftRow{}, err
	}
	for _, row := range rows {
		if row.ContentID == contentID {
			return cloneDraftRow(row), nil
		}
	}
	return content.DraftRow{}, ErrContentDraftNotFound
}

func cloneDraftRows(rows []content.DraftRow) []content.DraftRow {
	if len(rows) == 0 {
		return nil
	}
	cloned := make([]content.DraftRow, len(rows))
	for index, row := range rows {
		cloned[index] = cloneDraftRow(row)
	}
	return cloned
}

func cloneDraftRow(row content.DraftRow) content.DraftRow {
	row.DisplayJSON = append([]byte(nil), row.DisplayJSON...)
	row.DataJSON = append([]byte(nil), row.DataJSON...)
	return row
}

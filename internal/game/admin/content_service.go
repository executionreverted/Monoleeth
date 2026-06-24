package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

var ErrMissingContentVersionStore = errors.New("missing admin content version store")
var ErrMissingContentDraftStore = errors.New("missing admin content draft store")
var ErrMissingContentDraftWriter = errors.New("missing admin content draft writer")
var ErrMissingContentDraftValidator = errors.New("missing admin content draft validator")
var ErrContentDraftNotFound = errors.New("content draft row not found")

type ContentVersionStore interface {
	ListContentVersions(context.Context, content.VersionListInput) (content.VersionList, error)
}

type ContentDraftStore interface {
	LoadDraftRows(context.Context, content.ContentType) ([]content.DraftRow, error)
}

type ContentDraftWriter interface {
	UpsertDraftRow(context.Context, content.ContentType, content.DraftRow) error
}

type ContentDraftValidator interface {
	ValidateContentSnapshot(context.Context, content.Snapshot) error
}

type ContentServiceConfig struct {
	Versions  ContentVersionStore
	Drafts    ContentDraftStore
	Writer    ContentDraftWriter
	Validator ContentDraftValidator
	Clock     foundation.Clock
}

type ContentService struct {
	versions  ContentVersionStore
	drafts    ContentDraftStore
	writer    ContentDraftWriter
	validator ContentDraftValidator
	clock     foundation.Clock
}

func NewContentService(config ContentServiceConfig) *ContentService {
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	writer := config.Writer
	if writer == nil {
		if draftWriter, ok := config.Drafts.(ContentDraftWriter); ok {
			writer = draftWriter
		}
	}
	return &ContentService{
		versions:  config.Versions,
		drafts:    config.Drafts,
		writer:    writer,
		validator: config.Validator,
		clock:     clock,
	}
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

func (service *ContentService) UpdateDraftRow(ctx context.Context, input content.DraftUpdateInput) (content.DraftRow, error) {
	if service == nil || service.writer == nil {
		return content.DraftRow{}, ErrMissingContentDraftWriter
	}
	if !content.IsKnownContentType(input.ContentType) {
		return content.DraftRow{}, fmt.Errorf("%s: %w", input.ContentType, content.ErrUnknownContentType)
	}
	if err := content.ValidateContentID(string(input.ContentType), string(input.ContentID)); err != nil {
		return content.DraftRow{}, err
	}
	displayJSON := input.DisplayJSON
	if len(displayJSON) == 0 {
		displayJSON = []byte(`{}`)
	}
	row := content.DraftRow{
		ContentID:    input.ContentID,
		DraftVersion: strings.TrimSpace(input.DraftVersion),
		Enabled:      input.Enabled,
		DisplayJSON:  append([]byte(nil), displayJSON...),
		DataJSON:     append([]byte(nil), input.DataJSON...),
		UpdatedBy:    strings.TrimSpace(input.UpdatedBy),
	}
	if err := content.ValidateSnapshotRow(input.ContentType, content.SnapshotRow{
		ContentID:   row.ContentID,
		Enabled:     row.Enabled,
		DisplayJSON: row.DisplayJSON,
		DataJSON:    row.DataJSON,
	}); err != nil {
		return content.DraftRow{}, err
	}
	if err := service.writer.UpsertDraftRow(ctx, input.ContentType, row); err != nil {
		return content.DraftRow{}, err
	}
	return cloneDraftRow(row), nil
}

func (service *ContentService) ValidateDraft(ctx context.Context, input content.DraftValidationInput) (content.DraftValidationReport, error) {
	if service == nil || service.drafts == nil {
		return content.DraftValidationReport{}, ErrMissingContentDraftStore
	}
	if service.validator == nil {
		return content.DraftValidationReport{}, ErrMissingContentDraftValidator
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		version = "draft_validation"
	}
	report := content.DraftValidationReport{
		Valid:     true,
		Version:   version,
		CheckedAt: service.clock.Now().UTC(),
	}
	snapshot := content.Snapshot{Version: version}
	for _, contentType := range content.AllContentTypes() {
		rows, err := service.drafts.LoadDraftRows(ctx, contentType)
		if err != nil {
			return content.DraftValidationReport{}, err
		}
		if err := snapshot.SetRows(contentType, content.SnapshotRowsFromDraftRows(rows)); err != nil {
			return content.DraftValidationReport{}, err
		}
	}
	if err := snapshot.Validate(); err != nil {
		report.Valid = false
		report.Issues = append(report.Issues, validationIssue("snapshot", "invalid_snapshot", err))
		return report, nil
	}
	if err := service.validator.ValidateContentSnapshot(ctx, snapshot); err != nil {
		report.Valid = false
		report.Issues = append(report.Issues, validationIssue("runtime_catalog", "invalid_runtime_catalog", err))
		return report, nil
	}
	return report, nil
}

func validationIssue(path string, code string, err error) content.DraftValidationIssue {
	return content.DraftValidationIssue{
		Path:    path,
		Code:    code,
		Message: err.Error(),
	}
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

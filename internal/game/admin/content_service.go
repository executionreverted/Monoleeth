package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

var ErrMissingContentVersionStore = errors.New("missing admin content version store")
var ErrMissingContentDraftStore = errors.New("missing admin content draft store")
var ErrMissingContentDraftWriter = errors.New("missing admin content draft writer")
var ErrMissingContentDraftValidator = errors.New("missing admin content draft validator")
var ErrMissingContentPublisher = errors.New("missing admin content publisher")
var ErrMissingContentSnapshotReader = errors.New("missing admin content snapshot reader")
var ErrMissingContentAuditStore = errors.New("missing admin content audit store")
var ErrContentDraftNotFound = errors.New("content draft row not found")
var ErrMissingContentPublishNotes = errors.New("missing content publish notes")
var ErrInvalidContentBalanceTag = errors.New("invalid content balance tag")
var ErrContentPublishActiveCraftDefinition = errors.New("content publish blocked by active craft recipe definition")
var ErrContentPublishActiveProductionDefinition = errors.New("content publish blocked by active production building definition")

const (
	maxAuditRowJSONBytes = 32 * 1024
	maxBalanceTagLength  = 64
)

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

type ContentPublisher interface {
	PublishContentSnapshot(context.Context, content.PublishSnapshotInput) (content.PublishSnapshotResult, error)
}

type ContentSnapshotReader interface {
	LoadCurrentContentSnapshot(context.Context) (content.SnapshotVersionRecord, error)
	LoadContentSnapshotByID(context.Context, string) (content.SnapshotVersionRecord, error)
}

type ContentAuditStore interface {
	ListContentAudit(context.Context, content.AuditLogInput) (content.AuditLog, error)
}

type ActiveCraftJobReader interface {
	ActiveCraftJobs(context.Context) ([]crafting.CraftJob, error)
}

type ActiveProductionBuildingReader interface {
	ActiveProductionBuildings(context.Context) ([]production.PlanetBuilding, error)
}

type ContentServiceConfig struct {
	Versions         ContentVersionStore
	Drafts           ContentDraftStore
	Writer           ContentDraftWriter
	Validator        ContentDraftValidator
	Publisher        ContentPublisher
	Snapshots        ContentSnapshotReader
	Audit            ContentAuditStore
	ActiveCraft      ActiveCraftJobReader
	ActiveProduction ActiveProductionBuildingReader
	Clock            foundation.Clock
}

type ContentService struct {
	versions         ContentVersionStore
	drafts           ContentDraftStore
	writer           ContentDraftWriter
	validator        ContentDraftValidator
	publisher        ContentPublisher
	snapshots        ContentSnapshotReader
	audit            ContentAuditStore
	activeCraft      ActiveCraftJobReader
	activeProduction ActiveProductionBuildingReader
	clock            foundation.Clock
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
	publisher := config.Publisher
	if publisher == nil {
		if draftPublisher, ok := config.Drafts.(ContentPublisher); ok {
			publisher = draftPublisher
		}
	}
	snapshots := config.Snapshots
	if snapshots == nil {
		if snapshotReader, ok := config.Drafts.(ContentSnapshotReader); ok {
			snapshots = snapshotReader
		}
	}
	audit := config.Audit
	if audit == nil {
		if auditStore, ok := config.Drafts.(ContentAuditStore); ok {
			audit = auditStore
		}
	}
	return &ContentService{
		versions:         config.Versions,
		drafts:           config.Drafts,
		writer:           writer,
		validator:        config.Validator,
		publisher:        publisher,
		snapshots:        snapshots,
		audit:            audit,
		activeCraft:      config.ActiveCraft,
		activeProduction: config.ActiveProduction,
		clock:            clock,
	}
}

func (service *ContentService) SetPublishSafetyReaders(craft ActiveCraftJobReader, production ActiveProductionBuildingReader) {
	if service == nil {
		return
	}
	service.activeCraft = craft
	service.activeProduction = production
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
	snapshot, _, err := service.draftSnapshot(ctx, version)
	if err != nil {
		return content.DraftValidationReport{}, err
	}
	return service.validateSnapshot(ctx, snapshot)
}

func (service *ContentService) PublishDraft(ctx context.Context, input content.PublishDraftInput) (content.PublishDraftResult, error) {
	if service == nil || service.publisher == nil {
		return content.PublishDraftResult{}, ErrMissingContentPublisher
	}
	if service.snapshots == nil {
		return content.PublishDraftResult{}, ErrMissingContentSnapshotReader
	}
	notes, balanceTag, err := normalizePublishMetadata(input.Notes, input.BalanceTag)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		version = fmt.Sprintf("content_publish_%d", service.clock.Now().UTC().UnixMilli())
	}
	snapshot, rowCount, err := service.draftSnapshot(ctx, version)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	report, err := service.validateSnapshot(ctx, snapshot)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	result := content.PublishDraftResult{Validation: report, RowCount: rowCount}
	if !report.Valid {
		return result, nil
	}
	current, err := service.snapshots.LoadCurrentContentSnapshot(ctx)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	if err := service.validatePublishSafety(ctx, current.Snapshot, snapshot); err != nil {
		return content.PublishDraftResult{}, err
	}
	validationJSON, err := json.Marshal(report)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	idempotencyKey, err := publishIdempotencyKey(snapshot, notes, balanceTag)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	versionID := deterministicContentUUID("content_publish", idempotencyKey)
	auditEntries, err := buildAuditEntries(versionID, current.Snapshot, snapshot, input.ActorAccountID, notes, balanceTag)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	published, err := service.publisher.PublishContentSnapshot(ctx, content.PublishSnapshotInput{
		ID:                   versionID,
		Version:              version,
		Snapshot:             snapshot,
		ValidationReportJSON: validationJSON,
		IdempotencyKey:       idempotencyKey,
		ExpectedCurrentID:    current.ID,
		Notes:                notes,
		BalanceTag:           balanceTag,
		CreatedBy:            strings.TrimSpace(input.ActorAccountID),
		PublishedBy:          strings.TrimSpace(input.ActorAccountID),
		PublishedAt:          service.clock.Now().UTC(),
		AuditEntries:         auditEntries,
	})
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	result.Published = true
	result.Version = content.VersionSummaryFromRecord(published.Record)
	result.Idempotent = published.Idempotent
	return result, nil
}

func (service *ContentService) Rollback(ctx context.Context, input content.RollbackInput) (content.PublishDraftResult, error) {
	if service == nil || service.publisher == nil {
		return content.PublishDraftResult{}, ErrMissingContentPublisher
	}
	if service.snapshots == nil {
		return content.PublishDraftResult{}, ErrMissingContentSnapshotReader
	}
	targetID := strings.TrimSpace(input.TargetVersionID)
	if err := content.ValidateContentID("content rollback target", targetID); err != nil {
		return content.PublishDraftResult{}, err
	}
	notes, balanceTag, err := normalizePublishMetadata(input.Notes, input.BalanceTag)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	target, err := service.snapshots.LoadContentSnapshotByID(ctx, targetID)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		version = fmt.Sprintf("content_rollback_%d", service.clock.Now().UTC().UnixMilli())
	}
	snapshot := target.Snapshot
	snapshot.Version = version
	report, err := service.validateSnapshot(ctx, snapshot)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	result := content.PublishDraftResult{Validation: report, RowCount: snapshotRowCount(snapshot)}
	if !report.Valid {
		return result, nil
	}
	current, err := service.snapshots.LoadCurrentContentSnapshot(ctx)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	validationJSON, err := json.Marshal(report)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("content_rollback:%s:%s", targetID, version)
	}
	versionID := deterministicContentUUID("content_rollback", idempotencyKey)
	auditEntries, err := buildAuditEntries(versionID, current.Snapshot, snapshot, input.ActorAccountID, notes, balanceTag)
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	published, err := service.publisher.PublishContentSnapshot(ctx, content.PublishSnapshotInput{
		ID:                   versionID,
		Version:              version,
		Snapshot:             snapshot,
		ValidationReportJSON: validationJSON,
		IdempotencyKey:       idempotencyKey,
		ExpectedCurrentID:    current.ID,
		Notes:                notes,
		BalanceTag:           balanceTag,
		CreatedBy:            strings.TrimSpace(input.ActorAccountID),
		PublishedBy:          strings.TrimSpace(input.ActorAccountID),
		PublishedAt:          service.clock.Now().UTC(),
		RolledBackFrom:       targetID,
		AuditEntries:         auditEntries,
	})
	if err != nil {
		return content.PublishDraftResult{}, err
	}
	result.Published = true
	result.Version = content.VersionSummaryFromRecord(published.Record)
	result.Idempotent = published.Idempotent
	return result, nil
}

func (service *ContentService) AuditLog(ctx context.Context, input content.AuditLogInput) (content.AuditLog, error) {
	if service == nil || service.audit == nil {
		return content.AuditLog{}, ErrMissingContentAuditStore
	}
	input = content.NormalizeAuditLogInput(input)
	if input.VersionID != "" {
		if err := content.ValidateContentID("content audit version", input.VersionID); err != nil {
			return content.AuditLog{}, err
		}
	}
	if input.ContentType != "" && !content.IsKnownContentType(input.ContentType) {
		return content.AuditLog{}, fmt.Errorf("%s: %w", input.ContentType, content.ErrUnknownContentType)
	}
	if input.ContentID != "" {
		if err := content.ValidateContentID(string(input.ContentType), string(input.ContentID)); err != nil {
			return content.AuditLog{}, err
		}
	}
	log, err := service.audit.ListContentAudit(ctx, input)
	if err != nil {
		return content.AuditLog{}, err
	}
	log.Limit = input.Limit
	log.Offset = input.Offset
	log.GeneratedAt = service.clock.Now().UTC()
	return log, nil
}

func (service *ContentService) draftSnapshot(ctx context.Context, version string) (content.Snapshot, int, error) {
	if service == nil || service.drafts == nil {
		return content.Snapshot{}, 0, ErrMissingContentDraftStore
	}
	snapshot := content.Snapshot{Version: version}
	rowCount := 0
	for _, contentType := range content.AllContentTypes() {
		rows, err := service.drafts.LoadDraftRows(ctx, contentType)
		if err != nil {
			return content.Snapshot{}, 0, err
		}
		rowCount += len(rows)
		if err := snapshot.SetRows(contentType, content.SnapshotRowsFromDraftRows(rows)); err != nil {
			return content.Snapshot{}, 0, err
		}
	}
	return snapshot, rowCount, nil
}

func (service *ContentService) validateSnapshot(ctx context.Context, snapshot content.Snapshot) (content.DraftValidationReport, error) {
	if service == nil || service.validator == nil {
		return content.DraftValidationReport{}, ErrMissingContentDraftValidator
	}
	report := content.DraftValidationReport{
		Valid:     true,
		Version:   snapshot.Version,
		CheckedAt: service.clock.Now().UTC(),
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

func (service *ContentService) validatePublishSafety(ctx context.Context, current content.Snapshot, next content.Snapshot) error {
	if service == nil {
		return nil
	}
	changedRecipes := changedContentIDs(current.CraftRecipes, next.CraftRecipes)
	if len(changedRecipes) > 0 && service.activeCraft != nil {
		jobs, err := service.activeCraft.ActiveCraftJobs(ctx)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			if job.State != crafting.CraftJobStateRunning {
				continue
			}
			if changedRecipes[content.ContentID(string(job.RecipeSource.DefinitionID))] {
				return fmt.Errorf("recipe %q job %q version %q: %w",
					job.RecipeSource.DefinitionID, job.JobID, job.RecipeSource.Version, ErrContentPublishActiveCraftDefinition)
			}
		}
	}

	changedProduction := changedContentIDs(current.ProductionBuildings, next.ProductionBuildings)
	if len(changedProduction) > 0 && service.activeProduction != nil {
		buildings, err := service.activeProduction.ActiveProductionBuildings(ctx)
		if err != nil {
			return err
		}
		for _, building := range buildings {
			if building.State != production.BuildingStateActive {
				continue
			}
			if changedProduction[content.ContentID(string(building.Source.DefinitionID))] {
				return fmt.Errorf("production definition %q building %q planet %q version %q: %w",
					building.Source.DefinitionID, building.BuildingID, building.PlanetID, building.Source.Version,
					ErrContentPublishActiveProductionDefinition)
			}
		}
	}
	return nil
}

func changedContentIDs(current []content.SnapshotRow, next []content.SnapshotRow) map[content.ContentID]bool {
	currentRows := snapshotRowsByID(current)
	nextRows := snapshotRowsByID(next)
	changed := make(map[content.ContentID]bool)
	for contentID, currentRow := range currentRows {
		nextRow, ok := nextRows[contentID]
		if !ok || !snapshotRowsEquivalent(currentRow, nextRow) {
			changed[contentID] = true
		}
	}
	for contentID := range nextRows {
		if _, ok := currentRows[contentID]; !ok {
			changed[contentID] = true
		}
	}
	return changed
}

func snapshotRowsByID(rows []content.SnapshotRow) map[content.ContentID]content.SnapshotRow {
	out := make(map[content.ContentID]content.SnapshotRow, len(rows))
	for _, row := range rows {
		out[row.ContentID] = row
	}
	return out
}

func snapshotRowsEquivalent(left content.SnapshotRow, right content.SnapshotRow) bool {
	if left.Enabled != right.Enabled {
		return false
	}
	leftData, leftDataOK := compactJSON(left.DataJSON)
	rightData, rightDataOK := compactJSON(right.DataJSON)
	leftDisplay, leftDisplayOK := compactJSON(left.DisplayJSON)
	rightDisplay, rightDisplayOK := compactJSON(right.DisplayJSON)
	return leftDataOK && rightDataOK && leftDisplayOK && rightDisplayOK &&
		bytes.Equal(leftData, rightData) && bytes.Equal(leftDisplay, rightDisplay)
}

func compactJSON(raw json.RawMessage) ([]byte, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, true
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return nil, false
	}
	return buffer.Bytes(), true
}

func normalizePublishMetadata(notes string, balanceTag string) (string, string, error) {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return "", "", ErrMissingContentPublishNotes
	}
	balanceTag = strings.TrimSpace(balanceTag)
	if balanceTag == "" {
		return notes, "", nil
	}
	if len(balanceTag) > maxBalanceTagLength {
		return "", "", fmt.Errorf("balance_tag %q: %w", balanceTag, ErrInvalidContentBalanceTag)
	}
	for _, r := range balanceTag {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "", "", fmt.Errorf("balance_tag %q: %w", balanceTag, ErrInvalidContentBalanceTag)
	}
	return notes, balanceTag, nil
}

func publishIdempotencyKey(snapshot content.Snapshot, notes string, balanceTag string) (string, error) {
	payload := struct {
		Snapshot   content.Snapshot `json:"snapshot"`
		Notes      string           `json:"notes"`
		BalanceTag string           `json:"balance_tag"`
	}{
		Snapshot:   snapshot,
		Notes:      strings.TrimSpace(notes),
		BalanceTag: strings.TrimSpace(balanceTag),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "content_publish:" + hex.EncodeToString(sum[:]), nil
}

func deterministicContentUUID(namespace string, value string) string {
	sum := sha256.Sum256([]byte(namespace + "\x00" + value))
	id := append([]byte(nil), sum[:16]...)
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

func buildAuditEntries(versionID string, oldSnapshot content.Snapshot, newSnapshot content.Snapshot, actor string, note string, balanceTag string) ([]content.AuditLogEntryInput, error) {
	actor = strings.TrimSpace(actor)
	note = strings.TrimSpace(note)
	balanceTag = strings.TrimSpace(balanceTag)
	var entries []content.AuditLogEntryInput
	for _, contentType := range content.AllContentTypes() {
		oldRows := snapshotGroupRows(oldSnapshot, contentType)
		newRows := snapshotGroupRows(newSnapshot, contentType)
		contentIDs := sortedAuditContentIDs(oldRows, newRows)
		for _, contentID := range contentIDs {
			oldRow, oldOK := oldRows[contentID]
			newRow, newOK := newRows[contentID]
			oldJSON, err := auditRowJSON(oldRow, oldOK)
			if err != nil {
				return nil, err
			}
			newJSON, err := auditRowJSON(newRow, newOK)
			if err != nil {
				return nil, err
			}
			if string(oldJSON) == string(newJSON) {
				continue
			}
			entryID := deterministicContentUUID("content_audit", versionID+"|"+string(contentType)+"|"+string(contentID)+"|$")
			entries = append(entries, content.AuditLogEntryInput{
				ID:               entryID,
				ContentVersionID: versionID,
				ContentType:      contentType,
				ContentID:        contentID,
				FieldPath:        "$",
				OldValueJSON:     oldJSON,
				NewValueJSON:     newJSON,
				ActorAccountID:   actor,
				Note:             note,
				BalanceTag:       balanceTag,
			})
		}
	}
	return entries, nil
}

func snapshotGroupRows(snapshot content.Snapshot, contentType content.ContentType) map[content.ContentID]content.SnapshotRow {
	out := make(map[content.ContentID]content.SnapshotRow)
	for _, group := range snapshot.Groups() {
		if group.Type != contentType {
			continue
		}
		for _, row := range group.Rows {
			out[row.ContentID] = row
		}
		return out
	}
	return out
}

func sortedAuditContentIDs(left, right map[content.ContentID]content.SnapshotRow) []content.ContentID {
	seen := make(map[content.ContentID]struct{}, len(left)+len(right))
	for contentID := range left {
		seen[contentID] = struct{}{}
	}
	for contentID := range right {
		seen[contentID] = struct{}{}
	}
	out := make([]content.ContentID, 0, len(seen))
	for contentID := range seen {
		out = append(out, contentID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func auditRowJSON(row content.SnapshotRow, ok bool) (json.RawMessage, error) {
	if !ok {
		return nil, nil
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return nil, err
	}
	return scrubAuditJSON(encoded, row.ContentID)
}

func scrubAuditJSON(raw json.RawMessage, contentID content.ContentID) (json.RawMessage, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	scrubbed, err := json.Marshal(scrubAuditValue(decoded))
	if err != nil {
		return nil, err
	}
	if len(scrubbed) <= maxAuditRowJSONBytes {
		return scrubbed, nil
	}
	bounded, err := json.Marshal(map[string]any{
		"content_id": string(contentID),
		"redacted":   "audit_payload_too_large",
		"bytes":      len(scrubbed),
	})
	if err != nil {
		return nil, err
	}
	return bounded, nil
}

func scrubAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if isSensitiveAuditKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = scrubAuditValue(nested)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, nested := range typed {
			out[i] = scrubAuditValue(nested)
		}
		return out
	default:
		return value
	}
}

func isSensitiveAuditKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "private_key") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "procedural_seed") ||
		lower == "seed"
}

func snapshotRowCount(snapshot content.Snapshot) int {
	total := 0
	for _, group := range snapshot.Groups() {
		total += len(group.Rows)
	}
	return total
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

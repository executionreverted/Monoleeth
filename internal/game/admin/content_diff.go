package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gameproject/internal/game/content"
)

var ErrMissingContentDiffStore = errors.New("missing admin content diff store")

// DiffVersions computes the row-level content changes between two resolved
// snapshots. Base/Target accept a content version id or the reserved selectors
// content.DiffSelectorCurrent ("current") / content.DiffSelectorDraft ("draft").
// Returned payloads are secret-scrubbed so admin diff views never leak hidden
// loot/seed/spawn fields.
func (service *ContentService) DiffVersions(ctx context.Context, input content.DiffInput) (content.DiffResult, error) {
	if service == nil {
		return content.DiffResult{}, ErrMissingContentDiffStore
	}
	if service.snapshots == nil {
		return content.DiffResult{}, ErrMissingContentSnapshotReader
	}
	baseVersionID, baseSnapshot, err := service.resolveDiffSnapshot(ctx, input.BaseVersionID, content.DiffSelectorCurrent)
	if err != nil {
		return content.DiffResult{}, err
	}
	targetVersionID, targetSnapshot, err := service.resolveDiffSnapshot(ctx, input.TargetVersionID, content.DiffSelectorDraft)
	if err != nil {
		return content.DiffResult{}, err
	}
	rawEntries := content.DiffSnapshots(baseSnapshot, targetSnapshot)
	entries := make([]content.DiffEntry, 0, len(rawEntries))
	for _, entry := range rawEntries {
		entry.OldValueJSON = scrubDiffJSON(entry.OldValueJSON)
		entry.NewValueJSON = scrubDiffJSON(entry.NewValueJSON)
		entries = append(entries, entry)
	}
	return content.DiffResult{
		BaseVersionID:   baseVersionID,
		TargetVersionID: targetVersionID,
		Entries:         entries,
		Total:           len(entries),
	}, nil
}

func (service *ContentService) resolveDiffSnapshot(ctx context.Context, selector string, fallback string) (string, content.Snapshot, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = fallback
	}
	switch selector {
	case content.DiffSelectorCurrent:
		record, err := service.snapshots.LoadCurrentContentSnapshot(ctx)
		if err != nil {
			return "", content.Snapshot{}, err
		}
		if record.ID == "" {
			return content.DiffSelectorCurrent, record.Snapshot, nil
		}
		return record.ID, record.Snapshot, nil
	case content.DiffSelectorDraft:
		snapshot, _, err := service.draftSnapshot(ctx, "content_diff_draft")
		if err != nil {
			return "", content.Snapshot{}, err
		}
		return content.DiffSelectorDraft, snapshot, nil
	default:
		if err := content.ValidateContentID("content diff version", selector); err != nil {
			return "", content.Snapshot{}, err
		}
		record, err := service.snapshots.LoadContentSnapshotByID(ctx, selector)
		if err != nil {
			return "", content.Snapshot{}, err
		}
		return record.ID, record.Snapshot, nil
	}
}

func scrubDiffJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return raw
	}
	scrubbed, err := json.Marshal(scrubAuditValue(decoded))
	if err != nil {
		return raw
	}
	return scrubbed
}

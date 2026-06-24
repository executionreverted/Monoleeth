package content

import (
	"encoding/json"
	"time"
)

const (
	DefaultDraftListLimit = 50
	MaxDraftListLimit     = 200
)

type DraftListInput struct {
	ContentType ContentType
	Limit       int
	Offset      int
}

type DraftUpdateInput struct {
	ContentType  ContentType
	ContentID    ContentID
	DraftVersion string
	Enabled      bool
	DisplayJSON  json.RawMessage
	DataJSON     json.RawMessage
	UpdatedBy    string
}

type DraftValidationInput struct {
	Version string
}

type DraftValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DraftValidationReport struct {
	Valid     bool                   `json:"valid"`
	Version   string                 `json:"version"`
	CheckedAt time.Time              `json:"checked_at"`
	Issues    []DraftValidationIssue `json:"issues"`
}

type DraftRow struct {
	ContentID    ContentID
	DraftVersion string
	Enabled      bool
	DisplayJSON  json.RawMessage
	DataJSON     json.RawMessage
	UpdatedBy    string
}

type DraftList struct {
	ContentType ContentType
	Rows        []DraftRow
	Total       int
	Limit       int
	Offset      int
	GeneratedAt time.Time
}

func NormalizeDraftListInput(input DraftListInput) DraftListInput {
	if input.Limit <= 0 {
		input.Limit = DefaultDraftListLimit
	}
	if input.Limit > MaxDraftListLimit {
		input.Limit = MaxDraftListLimit
	}
	if input.Offset < 0 {
		input.Offset = 0
	}
	return input
}

func SnapshotRowsFromDraftRows(rows []DraftRow) []SnapshotRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]SnapshotRow, len(rows))
	for index, row := range rows {
		out[index] = SnapshotRow{
			ContentID:   row.ContentID,
			Enabled:     row.Enabled,
			DisplayJSON: append(json.RawMessage(nil), row.DisplayJSON...),
			DataJSON:    append(json.RawMessage(nil), row.DataJSON...),
		}
	}
	return out
}

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

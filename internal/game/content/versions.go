package content

import "time"

const (
	DefaultVersionListLimit = 25
	MaxVersionListLimit     = 100
)

type VersionListInput struct {
	Limit  int
	Offset int
}

type VersionSummary struct {
	ID             string
	Version        string
	Status         string
	Current        bool
	Notes          string
	BalanceTag     string
	CreatedBy      string
	CreatedAt      time.Time
	PublishedBy    string
	PublishedAt    time.Time
	RolledBackFrom string
}

type VersionList struct {
	Versions    []VersionSummary
	Total       int
	Limit       int
	Offset      int
	GeneratedAt time.Time
}

func NormalizeVersionListInput(input VersionListInput) VersionListInput {
	if input.Limit <= 0 {
		input.Limit = DefaultVersionListLimit
	}
	if input.Limit > MaxVersionListLimit {
		input.Limit = MaxVersionListLimit
	}
	if input.Offset < 0 {
		input.Offset = 0
	}
	return input
}

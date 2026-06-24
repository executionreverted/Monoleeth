package admin

import (
	"context"
	"errors"

	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

var ErrMissingContentVersionStore = errors.New("missing admin content version store")

type ContentVersionStore interface {
	ListContentVersions(context.Context, content.VersionListInput) (content.VersionList, error)
}

type ContentServiceConfig struct {
	Versions ContentVersionStore
	Clock    foundation.Clock
}

type ContentService struct {
	versions ContentVersionStore
	clock    foundation.Clock
}

func NewContentService(config ContentServiceConfig) *ContentService {
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &ContentService{versions: config.Versions, clock: clock}
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

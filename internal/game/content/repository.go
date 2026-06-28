package content

import (
	"context"
	"errors"
	"fmt"

	"gameproject/internal/game/world"
)

var ErrMissingContentRepository = errors.New("missing content repository")

// Repository is the DB/CMS seam for published gameplay content. Implementations
// must return already-published server-side content, never client-authored rows.
type Repository interface {
	LoadPublishedContent(ctx context.Context, worldID world.WorldID) (GameplayContent, error)
}

// StaticRepository is a seed/test adapter for typed default content. Runtime
// boot should use it only when tests inject it explicitly; live server startup
// loads published rows from contentdb.
type StaticRepository struct{}

func NewStaticRepository() StaticRepository {
	return StaticRepository{}
}

func (repository StaticRepository) LoadPublishedContent(ctx context.Context, worldID world.WorldID) (GameplayContent, error) {
	if err := ctx.Err(); err != nil {
		return GameplayContent{}, err
	}
	return DefaultGameplayContent(worldID)
}

func LoadPublishedContent(ctx context.Context, repository Repository, worldID world.WorldID) (GameplayContent, error) {
	if repository == nil {
		return GameplayContent{}, ErrMissingContentRepository
	}
	content, err := repository.LoadPublishedContent(ctx, worldID)
	if err != nil {
		return GameplayContent{}, fmt.Errorf("published content: %w", err)
	}
	if err := content.Validate(); err != nil {
		return GameplayContent{}, fmt.Errorf("published content: %w", err)
	}
	return content, nil
}

package server

import (
	"context"
	"fmt"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

// contentApplyOutcome reports whether a CMS publish reached the live runtime
// catalog projection. The runtime layer owns the projection swap; the handler
// layer combines this with the publish result to report runtime_applied,
// runtime_version, published_version, and pending_restart honestly.
type contentApplyOutcome struct {
	Applied        bool
	PendingRestart bool
	RuntimeVersion string
}

// applyPublishedContent reflects a freshly published content version into the
// live player catalog projection when the change is projection-safe, or
// honestly reports pending_restart when the change touches boot-wired
// authoritative services that cannot be hot-swapped.
func (runtime *Runtime) applyPublishedContent(ctx context.Context, plan gamecontent.RuntimeApplyPlan) (contentApplyOutcome, error) {
	outcome := contentApplyOutcome{RuntimeVersion: runtime.currentContentCatalogVersion()}
	if runtime == nil || runtime.contentReloader == nil {
		return outcome, nil
	}
	if plan.Class != gamecontent.ApplyClassSafeReload {
		outcome.PendingRestart = true
		return outcome, nil
	}
	bundle, err := runtime.contentReloader(ctx)
	if err != nil {
		return outcome, fmt.Errorf("reload published content: %w", err)
	}
	projection, err := gamecontent.ProjectGameplayContentForPlayers(bundle)
	if err != nil {
		return outcome, fmt.Errorf("reproject published content: %w", err)
	}
	runtime.mu.Lock()
	runtime.contentCatalogProjection = projection
	if projection.Version != "" {
		runtime.contentCatalogVersion = projection.Version
	}
	outcome.RuntimeVersion = runtime.contentCatalogVersion
	runtime.mu.Unlock()
	outcome.Applied = true
	return outcome, nil
}

func (runtime *Runtime) currentContentCatalogVersion() string {
	if runtime == nil {
		return ""
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.contentCatalogVersion
}

// reflectPublishedContent maps a publish/rollback result into an honest runtime
// apply outcome. When the publish did not commit (invalid draft), no reflection
// runs and the current runtime version is reported. When the publish committed
// but live reflection failed, the response still tells the admin the version is
// published but pending a restart.
func (runtime *Runtime) reflectPublishedContent(ctx context.Context, result gamecontent.PublishDraftResult) (contentApplyOutcome, error) {
	if !result.Published {
		return contentApplyOutcome{RuntimeVersion: runtime.currentContentCatalogVersion()}, nil
	}
	outcome, err := runtime.applyPublishedContent(ctx, result.RuntimeApplyPlan)
	if err != nil {
		return contentApplyOutcome{}, foundation.NewDomainError(foundation.CodeInternal, "Content published but not reflected in the live runtime; a restart is required.", foundation.WithCause(err))
	}
	return outcome, nil
}

// buildRuntimeContentReloader returns a closure that reloads the currently
// published gameplay content. It reuses the same loading path as boot so a
// live apply reads the same server-authoritative published version an admin
// just committed.
func buildRuntimeContentReloader(config RuntimeConfig) func(context.Context) (gamecontent.GameplayContent, error) {
	if config.ContentRepository != nil {
		repository := config.ContentRepository
		worldID := config.WorldID
		return func(ctx context.Context) (gamecontent.GameplayContent, error) {
			return gamecontent.LoadPublishedContent(ctx, repository, worldID)
		}
	}
	return func(ctx context.Context) (gamecontent.GameplayContent, error) {
		return loadRuntimeContent(ctx, config)
	}
}

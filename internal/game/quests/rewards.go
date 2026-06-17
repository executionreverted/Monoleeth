package quests

import (
	"fmt"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// Validate reports whether kind is one of the supported generated reward grant kinds.
func (kind RewardKind) Validate() error {
	switch kind {
	case RewardKindCredits, RewardKindItem, RewardKindMainXP, RewardKindRoleXP:
		return nil
	default:
		return fmt.Errorf("reward kind %q: %w", kind, ErrInvalidRewardKind)
	}
}

// Validate reports whether hook kind is a known policy placeholder.
func (kind RewardHookKind) Validate() error {
	switch kind {
	case RewardHookRareCap, RewardHookXCore, RewardHookPremium:
		return nil
	default:
		return fmt.Errorf("reward hook kind %q: %w", kind, ErrInvalidRewardHook)
	}
}

// Validate reports whether payload contains concrete grants and valid cap hooks.
func (payload RewardPayload) Validate() error {
	hooks := payload.rewardHooks()
	if len(payload.Grants) == 0 && len(hooks) == 0 {
		return ErrEmptyRewardPayload
	}
	for _, grant := range payload.Grants {
		if err := grant.Validate(); err != nil {
			return err
		}
	}
	if err := validateRewardHooks(hooks); err != nil {
		return err
	}
	if len(payload.Grants) == 0 {
		return ErrEmptyRewardPayload
	}
	return nil
}

// Validate reports whether grant has the fields required by its reward kind.
func (grant RewardGrant) Validate() error {
	if err := grant.Kind.Validate(); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(grant.Amount); err != nil {
		return fmt.Errorf("reward amount %d: %w", grant.Amount, ErrInvalidRewardAmount)
	}
	switch grant.Kind {
	case RewardKindCredits:
		if err := grant.Currency.Validate(); err != nil {
			return fmt.Errorf("reward currency %q: %w", grant.Currency, ErrInvalidRewardCurrency)
		}
		if grant.Currency != economy.CurrencyBucketCredits {
			return fmt.Errorf("reward currency %q: %w", grant.Currency, ErrInvalidRewardCurrency)
		}
	case RewardKindItem:
		if err := grant.ItemID.Validate(); err != nil {
			return fmt.Errorf("reward item %q: %w", grant.ItemID, ErrInvalidRewardItem)
		}
	case RewardKindMainXP:
		return nil
	case RewardKindRoleXP:
		if err := grant.Role.Validate(); err != nil {
			return fmt.Errorf("reward role %q: %w", grant.Role, ErrInvalidRewardRole)
		}
	}
	return nil
}

// Validate reports whether hook has a supported kind and positive limit.
func (hook RewardHook) Validate() error {
	if err := hook.Kind.Validate(); err != nil {
		return err
	}
	if hook.Limit <= 0 {
		return fmt.Errorf("reward hook limit %d: %w", hook.Limit, ErrInvalidRewardHook)
	}
	return nil
}

func (payload RewardPayload) rewardHooks() []RewardHook {
	hooks := make([]RewardHook, 0, len(payload.RareCapHooks)+len(payload.Hooks))
	hooks = append(hooks, payload.RareCapHooks...)
	hooks = append(hooks, payload.Hooks...)
	return hooks
}

func validateRewardHooks(hooks []RewardHook) error {
	seen := make(map[RewardHook]struct{}, len(hooks))
	for _, hook := range hooks {
		if err := hook.Validate(); err != nil {
			return err
		}
		if _, ok := seen[hook]; ok {
			return fmt.Errorf("reward hook %q:%q: %w", hook.Kind, hook.Key, ErrDuplicateRewardHook)
		}
		seen[hook] = struct{}{}
	}
	return nil
}

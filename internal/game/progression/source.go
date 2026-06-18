package progression

import (
	"fmt"
	"strings"
)

// XPSourceType identifies the authoritative gameplay event family that caused
// an XP grant.
type XPSourceType string

const (
	XPSourceTypeCombat          XPSourceType = "combat"
	XPSourceTypeQuest           XPSourceType = "quest"
	XPSourceTypeLoot            XPSourceType = "loot"
	XPSourceTypeScan            XPSourceType = "scan"
	XPSourceTypeCraft           XPSourceType = "craft"
	XPSourceTypeConstruction    XPSourceType = "construction"
	XPSourceTypeRoute           XPSourceType = "route"
	XPSourceTypeEvent           XPSourceType = "event"
	XPSourceTypeAdminAdjustment XPSourceType = "admin_adjustment"
)

// XPSourceID identifies the authoritative source event or domain object that
// produced one XP grant.
type XPSourceID string

// XPIdempotencyKey identifies one XP grant for retry safety.
type XPIdempotencyKey string

// XPGrantAuthority identifies the server-owned domain boundary that verified
// source completion before asking progression to mutate XP. This is not a
// client payload field; it is a guard against accidentally wiring generic
// client-submitted XP source data directly into GrantXP.
type XPGrantAuthority string

const (
	XPGrantAuthorityCombatService     XPGrantAuthority = "combat_service"
	XPGrantAuthorityQuestService      XPGrantAuthority = "quest_service"
	XPGrantAuthorityLootService       XPGrantAuthority = "loot_service"
	XPGrantAuthorityScannerService    XPGrantAuthority = "scanner_service"
	XPGrantAuthorityCraftingService   XPGrantAuthority = "crafting_service"
	XPGrantAuthorityProductionService XPGrantAuthority = "production_service"
	XPGrantAuthorityRouteService      XPGrantAuthority = "route_service"
	XPGrantAuthorityEventService      XPGrantAuthority = "event_service"
	XPGrantAuthorityAdminService      XPGrantAuthority = "admin_service"
)

// SupportedXPSourceTypes returns MVP XP source types in stable order.
func SupportedXPSourceTypes() []XPSourceType {
	return []XPSourceType{
		XPSourceTypeCombat,
		XPSourceTypeQuest,
		XPSourceTypeLoot,
		XPSourceTypeScan,
		XPSourceTypeCraft,
		XPSourceTypeConstruction,
		XPSourceTypeRoute,
		XPSourceTypeEvent,
		XPSourceTypeAdminAdjustment,
	}
}

// SupportedXPGrantAuthorities returns XP grant authorities in stable order.
func SupportedXPGrantAuthorities() []XPGrantAuthority {
	return []XPGrantAuthority{
		XPGrantAuthorityCombatService,
		XPGrantAuthorityQuestService,
		XPGrantAuthorityLootService,
		XPGrantAuthorityScannerService,
		XPGrantAuthorityCraftingService,
		XPGrantAuthorityProductionService,
		XPGrantAuthorityRouteService,
		XPGrantAuthorityEventService,
		XPGrantAuthorityAdminService,
	}
}

// String returns the stable source type representation.
func (sourceType XPSourceType) String() string {
	return string(sourceType)
}

// Validate reports whether sourceType is supported.
func (sourceType XPSourceType) Validate() error {
	switch sourceType {
	case XPSourceTypeCombat,
		XPSourceTypeQuest,
		XPSourceTypeLoot,
		XPSourceTypeScan,
		XPSourceTypeCraft,
		XPSourceTypeConstruction,
		XPSourceTypeRoute,
		XPSourceTypeEvent,
		XPSourceTypeAdminAdjustment:
		return nil
	default:
		return fmt.Errorf("xp source type %q: %w", sourceType, ErrInvalidXPSourceType)
	}
}

// String returns the stable authority representation.
func (authority XPGrantAuthority) String() string {
	return string(authority)
}

// Validate reports whether authority is a known server-owned XP grant boundary.
func (authority XPGrantAuthority) Validate() error {
	switch authority {
	case XPGrantAuthorityCombatService,
		XPGrantAuthorityQuestService,
		XPGrantAuthorityLootService,
		XPGrantAuthorityScannerService,
		XPGrantAuthorityCraftingService,
		XPGrantAuthorityProductionService,
		XPGrantAuthorityRouteService,
		XPGrantAuthorityEventService,
		XPGrantAuthorityAdminService:
		return nil
	default:
		return fmt.Errorf("xp grant authority %q: %w", authority, ErrInvalidXPGrantAuthority)
	}
}

// RequiredXPGrantAuthorityForSource returns the server domain that owns one XP
// source family.
func RequiredXPGrantAuthorityForSource(sourceType XPSourceType) (XPGrantAuthority, error) {
	if err := sourceType.Validate(); err != nil {
		return "", err
	}
	switch sourceType {
	case XPSourceTypeCombat:
		return XPGrantAuthorityCombatService, nil
	case XPSourceTypeQuest:
		return XPGrantAuthorityQuestService, nil
	case XPSourceTypeLoot:
		return XPGrantAuthorityLootService, nil
	case XPSourceTypeScan:
		return XPGrantAuthorityScannerService, nil
	case XPSourceTypeCraft:
		return XPGrantAuthorityCraftingService, nil
	case XPSourceTypeConstruction:
		return XPGrantAuthorityProductionService, nil
	case XPSourceTypeRoute:
		return XPGrantAuthorityRouteService, nil
	case XPSourceTypeEvent:
		return XPGrantAuthorityEventService, nil
	case XPSourceTypeAdminAdjustment:
		return XPGrantAuthorityAdminService, nil
	default:
		return "", fmt.Errorf("xp source type %q: %w", sourceType, ErrInvalidXPSourceType)
	}
}

// ValidateForSource reports whether authority may grant XP for sourceType.
func (authority XPGrantAuthority) ValidateForSource(sourceType XPSourceType) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	required, err := RequiredXPGrantAuthorityForSource(sourceType)
	if err != nil {
		return err
	}
	if authority != required {
		return fmt.Errorf("authority %q for source %q requires %q: %w", authority, sourceType, required, ErrUnauthorizedXPSource)
	}
	return nil
}

// String returns the stable source id representation.
func (sourceID XPSourceID) String() string {
	return string(sourceID)
}

// Validate reports whether sourceID is non-blank.
func (sourceID XPSourceID) Validate() error {
	if strings.TrimSpace(string(sourceID)) == "" {
		return ErrEmptyXPSourceID
	}
	return nil
}

// String returns the stable idempotency key representation.
func (key XPIdempotencyKey) String() string {
	return string(key)
}

// IsZero reports whether key is the zero value.
func (key XPIdempotencyKey) IsZero() bool {
	return key == ""
}

// Validate reports whether key is non-blank.
func (key XPIdempotencyKey) Validate() error {
	if strings.TrimSpace(string(key)) == "" {
		return ErrEmptyXPIdempotencyKey
	}
	return nil
}

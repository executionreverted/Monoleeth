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

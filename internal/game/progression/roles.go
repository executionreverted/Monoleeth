package progression

import (
	"fmt"
	"strings"
)

// RoleType identifies a role-specific XP track.
type RoleType string

const (
	RoleTypeCombat       RoleType = "combat"
	RoleTypeScout        RoleType = "scout"
	RoleTypeCrafting     RoleType = "crafting"
	RoleTypeConstruction RoleType = "construction"
)

// SkillNodeID identifies a pilot passive skill node definition.
type SkillNodeID string

// SupportedRoleTypes returns the MVP role tracks in stable order.
func SupportedRoleTypes() []RoleType {
	return []RoleType{
		RoleTypeCombat,
		RoleTypeScout,
		RoleTypeCrafting,
		RoleTypeConstruction,
	}
}

// String returns the stable role representation.
func (role RoleType) String() string {
	return string(role)
}

// Validate reports whether role is supported.
func (role RoleType) Validate() error {
	switch role {
	case RoleTypeCombat, RoleTypeScout, RoleTypeCrafting, RoleTypeConstruction:
		return nil
	default:
		return fmt.Errorf("role type %q: %w", role, ErrInvalidRoleType)
	}
}

// IsZero reports whether role is the zero value.
func (role RoleType) IsZero() bool {
	return role == ""
}

// String returns the stable skill node id representation.
func (id SkillNodeID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id SkillNodeID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptySkillNodeID
	}
	return nil
}

// IsZero reports whether id is the zero value.
func (id SkillNodeID) IsZero() bool {
	return id == ""
}

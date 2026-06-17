package progression

import (
	"fmt"
	"strings"
)

// PilotSkillBranch identifies one passive skill tree branch.
type PilotSkillBranch string

const (
	PilotSkillBranchCombat   PilotSkillBranch = "combat"
	PilotSkillBranchScout    PilotSkillBranch = "scout"
	PilotSkillBranchIndustry PilotSkillBranch = "industry"
)

// PilotSkillEffectOperation identifies how a passive effect changes a stat.
type PilotSkillEffectOperation string

const (
	PilotSkillEffectAdd PilotSkillEffectOperation = "add"
	PilotSkillEffectMul PilotSkillEffectOperation = "mul"
)

// PilotSkillEffect describes passive stat metadata consumed later by stats.
type PilotSkillEffect struct {
	Stat      string                    `json:"stat"`
	Operation PilotSkillEffectOperation `json:"op"`
	Value     float64                   `json:"value"`
}

// PilotSkillDefinition is a server-owned passive pilot skill node definition.
type PilotSkillDefinition struct {
	NodeID               SkillNodeID        `json:"node_id"`
	Branch               PilotSkillBranch   `json:"branch"`
	RankRequirement      int                `json:"rank_requirement"`
	RoleRequirement      RoleType           `json:"role_requirement,omitempty"`
	RoleLevelRequirement int                `json:"role_level_requirement,omitempty"`
	PrerequisiteNodes    []SkillNodeID      `json:"prerequisite_nodes,omitempty"`
	CostPoints           int                `json:"cost_points"`
	Effects              []PilotSkillEffect `json:"effects,omitempty"`
}

var defaultPilotSkillDefinitions = []PilotSkillDefinition{
	{
		NodeID:          "combat_weapon_calibration",
		Branch:          PilotSkillBranchCombat,
		RankRequirement: MinRank + 1,
		CostPoints:      1,
		Effects: []PilotSkillEffect{
			{Stat: "laser_damage_flat", Operation: PilotSkillEffectAdd, Value: 2},
		},
	},
	{
		NodeID:               "combat_heat_control",
		Branch:               PilotSkillBranchCombat,
		RankRequirement:      MinRank + 1,
		RoleRequirement:      RoleTypeCombat,
		RoleLevelRequirement: 2,
		PrerequisiteNodes:    []SkillNodeID{"combat_weapon_calibration"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "laser_energy_cost_mult", Operation: PilotSkillEffectMul, Value: 0.98},
		},
	},
	{
		NodeID:               "combat_shield_cycles",
		Branch:               PilotSkillBranchCombat,
		RankRequirement:      3,
		RoleRequirement:      RoleTypeCombat,
		RoleLevelRequirement: 3,
		PrerequisiteNodes:    []SkillNodeID{"combat_heat_control"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "shield_regen_flat", Operation: PilotSkillEffectAdd, Value: 1},
		},
	},
	{
		NodeID:               "combat_elite_tracking",
		Branch:               PilotSkillBranchCombat,
		RankRequirement:      4,
		RoleRequirement:      RoleTypeCombat,
		RoleLevelRequirement: 4,
		PrerequisiteNodes:    []SkillNodeID{"combat_shield_cycles"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "weapon_accuracy_bonus", Operation: PilotSkillEffectAdd, Value: 0.02},
		},
	},
	{
		NodeID:               "combat_ace_protocols",
		Branch:               PilotSkillBranchCombat,
		RankRequirement:      MaxMVPRank,
		RoleRequirement:      RoleTypeCombat,
		RoleLevelRequirement: 5,
		PrerequisiteNodes:    []SkillNodeID{"combat_elite_tracking"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "weapon_damage_mult", Operation: PilotSkillEffectMul, Value: 1.02},
		},
	},
	{
		NodeID:          "scout_signal_tuning",
		Branch:          PilotSkillBranchScout,
		RankRequirement: MinRank + 1,
		CostPoints:      1,
		Effects: []PilotSkillEffect{
			{Stat: "scan_strength_flat", Operation: PilotSkillEffectAdd, Value: 1},
		},
	},
	{
		NodeID:               "scout_long_range_scan",
		Branch:               PilotSkillBranchScout,
		RankRequirement:      MinRank + 1,
		RoleRequirement:      RoleTypeScout,
		RoleLevelRequirement: 2,
		PrerequisiteNodes:    []SkillNodeID{"scout_signal_tuning"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "radar_range_flat", Operation: PilotSkillEffectAdd, Value: 25},
		},
	},
	{
		NodeID:               "scout_fog_reading",
		Branch:               PilotSkillBranchScout,
		RankRequirement:      3,
		RoleRequirement:      RoleTypeScout,
		RoleLevelRequirement: 3,
		PrerequisiteNodes:    []SkillNodeID{"scout_long_range_scan"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "fog_reveal_radius_flat", Operation: PilotSkillEffectAdd, Value: 12},
		},
	},
	{
		NodeID:               "scout_signal_analysis",
		Branch:               PilotSkillBranchScout,
		RankRequirement:      4,
		RoleRequirement:      RoleTypeScout,
		RoleLevelRequirement: 4,
		PrerequisiteNodes:    []SkillNodeID{"scout_fog_reading"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "scan_success_bonus", Operation: PilotSkillEffectAdd, Value: 0.02},
		},
	},
	{
		NodeID:               "scout_deep_space_nav",
		Branch:               PilotSkillBranchScout,
		RankRequirement:      MaxMVPRank,
		RoleRequirement:      RoleTypeScout,
		RoleLevelRequirement: 5,
		PrerequisiteNodes:    []SkillNodeID{"scout_signal_analysis"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "ship_speed_mult", Operation: PilotSkillEffectMul, Value: 1.02},
		},
	},
	{
		NodeID:          "industry_cargo_protocols",
		Branch:          PilotSkillBranchIndustry,
		RankRequirement: MinRank + 1,
		CostPoints:      1,
		Effects: []PilotSkillEffect{
			{Stat: "cargo_capacity_flat", Operation: PilotSkillEffectAdd, Value: 10},
		},
	},
	{
		NodeID:               "industry_craft_efficiency",
		Branch:               PilotSkillBranchIndustry,
		RankRequirement:      MinRank + 1,
		RoleRequirement:      RoleTypeCrafting,
		RoleLevelRequirement: 2,
		PrerequisiteNodes:    []SkillNodeID{"industry_cargo_protocols"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "craft_time_mult", Operation: PilotSkillEffectMul, Value: 0.98},
		},
	},
	{
		NodeID:               "industry_build_planning",
		Branch:               PilotSkillBranchIndustry,
		RankRequirement:      3,
		RoleRequirement:      RoleTypeConstruction,
		RoleLevelRequirement: 2,
		PrerequisiteNodes:    []SkillNodeID{"industry_craft_efficiency"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "construction_time_mult", Operation: PilotSkillEffectMul, Value: 0.98},
		},
	},
	{
		NodeID:               "industry_material_handling",
		Branch:               PilotSkillBranchIndustry,
		RankRequirement:      4,
		RoleRequirement:      RoleTypeCrafting,
		RoleLevelRequirement: 4,
		PrerequisiteNodes:    []SkillNodeID{"industry_build_planning"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "craft_material_refund_bonus", Operation: PilotSkillEffectAdd, Value: 0.01},
		},
	},
	{
		NodeID:               "industry_frontier_logistics",
		Branch:               PilotSkillBranchIndustry,
		RankRequirement:      MaxMVPRank,
		RoleRequirement:      RoleTypeConstruction,
		RoleLevelRequirement: 4,
		PrerequisiteNodes:    []SkillNodeID{"industry_material_handling"},
		CostPoints:           1,
		Effects: []PilotSkillEffect{
			{Stat: "route_cargo_capacity_mult", Operation: PilotSkillEffectMul, Value: 1.02},
		},
	},
}

// PilotSkillDefinitions returns the deterministic MVP passive skill tree.
func PilotSkillDefinitions() []PilotSkillDefinition {
	definitions := make([]PilotSkillDefinition, len(defaultPilotSkillDefinitions))
	for i, definition := range defaultPilotSkillDefinitions {
		definitions[i] = clonePilotSkillDefinition(definition)
	}
	return definitions
}

// PilotSkillDefinitionFor returns one server-owned skill node definition.
func PilotSkillDefinitionFor(nodeID SkillNodeID) (PilotSkillDefinition, error) {
	if err := nodeID.Validate(); err != nil {
		return PilotSkillDefinition{}, err
	}
	for _, definition := range defaultPilotSkillDefinitions {
		if definition.NodeID == nodeID {
			return clonePilotSkillDefinition(definition), nil
		}
	}
	return PilotSkillDefinition{}, fmt.Errorf("node %q: %w", nodeID, ErrUnknownSkillNode)
}

// Validate reports whether branch is supported by the MVP tree.
func (branch PilotSkillBranch) Validate() error {
	switch branch {
	case PilotSkillBranchCombat, PilotSkillBranchScout, PilotSkillBranchIndustry:
		return nil
	default:
		return fmt.Errorf("skill branch %q: %w", branch, ErrInvalidSkillBranch)
	}
}

// Validate reports whether definition is internally usable.
func (definition PilotSkillDefinition) Validate() error {
	if err := definition.NodeID.Validate(); err != nil {
		return err
	}
	if err := definition.Branch.Validate(); err != nil {
		return err
	}
	if err := ValidateRank(definition.RankRequirement); err != nil {
		return err
	}
	if definition.CostPoints <= 0 {
		return fmt.Errorf("node %q cost %d: %w", definition.NodeID, definition.CostPoints, ErrInvalidSkillPointCost)
	}
	if definition.RoleRequirement.IsZero() != (definition.RoleLevelRequirement == 0) {
		return fmt.Errorf("node %q role requirement %q level %d: %w", definition.NodeID, definition.RoleRequirement, definition.RoleLevelRequirement, ErrInvalidSkillNodeDefinition)
	}
	if !definition.RoleRequirement.IsZero() {
		if err := definition.RoleRequirement.Validate(); err != nil {
			return err
		}
		if err := ValidateRoleLevel(definition.RoleLevelRequirement); err != nil {
			return err
		}
	}
	for _, prerequisite := range definition.PrerequisiteNodes {
		if err := prerequisite.Validate(); err != nil {
			return err
		}
		if prerequisite == definition.NodeID {
			return fmt.Errorf("node %q self prerequisite: %w", definition.NodeID, ErrInvalidSkillNodeDefinition)
		}
	}
	for _, effect := range definition.Effects {
		if err := effect.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate reports whether effect has a named stat and supported operation.
func (effect PilotSkillEffect) Validate() error {
	if strings.TrimSpace(effect.Stat) == "" {
		return fmt.Errorf("empty skill effect stat: %w", ErrInvalidSkillNodeDefinition)
	}
	switch effect.Operation {
	case PilotSkillEffectAdd, PilotSkillEffectMul:
		return nil
	default:
		return fmt.Errorf("effect operation %q: %w", effect.Operation, ErrInvalidSkillNodeDefinition)
	}
}

func clonePilotSkillDefinition(definition PilotSkillDefinition) PilotSkillDefinition {
	definition.PrerequisiteNodes = append([]SkillNodeID(nil), definition.PrerequisiteNodes...)
	definition.Effects = append([]PilotSkillEffect(nil), definition.Effects...)
	return definition
}

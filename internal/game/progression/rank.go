package progression

import "fmt"

const defaultRankUpReason = "rank_up"

// RankRequirement defines the server-owned requirements to advance to Rank.
type RankRequirement struct {
	Rank                int        `json:"rank"`
	MainLevel           int        `json:"main_level"`
	AnyRoleTypes        []RoleType `json:"any_role_types,omitempty"`
	AnyRoleMinLevel     int        `json:"any_role_min_level,omitempty"`
	CompletedQuestCount int        `json:"completed_quest_count,omitempty"`
	RequirementCodes    []string   `json:"requirement_codes,omitempty"`
}

// RankMilestoneState is the server-owned progression evidence used by rank
// requirements that depend on completed gameplay milestones.
type RankMilestoneState struct {
	CompletedQuestCount int `json:"completed_quest_count,omitempty"`
}

var defaultRankRequirements = []RankRequirement{
	{Rank: 1, MainLevel: 1},
	{Rank: 2, MainLevel: 2, CompletedQuestCount: 1, RequirementCodes: []string{"main_level:2", "completed_quests:1"}},
	{Rank: 3, MainLevel: 3, RequirementCodes: []string{"main_level:3"}},
	{
		Rank:             4,
		MainLevel:        4,
		AnyRoleTypes:     []RoleType{RoleTypeCombat, RoleTypeScout},
		AnyRoleMinLevel:  3,
		RequirementCodes: []string{"main_level:4", "any_role_level:combat_or_scout:3"},
	},
	{
		Rank:             5,
		MainLevel:        5,
		AnyRoleTypes:     SupportedRoleTypes(),
		AnyRoleMinLevel:  4,
		RequirementCodes: []string{"main_level:5", "any_role_level:any:4"},
	},
}

// RankRequirements returns the deterministic MVP rank requirement table.
func RankRequirements() []RankRequirement {
	requirements := append([]RankRequirement(nil), defaultRankRequirements...)
	for index := range requirements {
		requirements[index].AnyRoleTypes = append([]RoleType(nil), requirements[index].AnyRoleTypes...)
		requirements[index].RequirementCodes = append([]string(nil), requirements[index].RequirementCodes...)
	}
	return requirements
}

// RankRequirementFor returns the server-owned requirement for rank.
func RankRequirementFor(rank int) (RankRequirement, error) {
	if err := ValidateRank(rank); err != nil {
		return RankRequirement{}, err
	}
	for _, requirement := range defaultRankRequirements {
		if requirement.Rank == rank {
			requirement.AnyRoleTypes = append([]RoleType(nil), requirement.AnyRoleTypes...)
			requirement.RequirementCodes = append([]string(nil), requirement.RequirementCodes...)
			return requirement, nil
		}
	}
	return RankRequirement{}, fmt.Errorf("rank %d: %w", rank, ErrInvalidRank)
}

func (requirement RankRequirement) missingFor(snapshot ProgressionSnapshot, milestones RankMilestoneState) []string {
	var missing []string
	if snapshot.Player.MainLevel < requirement.MainLevel {
		missing = append(missing, fmt.Sprintf("main_level:%d", requirement.MainLevel))
	}
	if requirement.AnyRoleMinLevel > 0 && !requirement.anyRoleRequirementMet(snapshot) {
		switch requirement.Rank {
		case 4:
			missing = append(missing, fmt.Sprintf("any_role_level:combat_or_scout:%d", requirement.AnyRoleMinLevel))
		default:
			missing = append(missing, fmt.Sprintf("any_role_level:any:%d", requirement.AnyRoleMinLevel))
		}
	}
	if requirement.CompletedQuestCount > 0 && milestones.CompletedQuestCount < requirement.CompletedQuestCount {
		missing = append(missing, fmt.Sprintf("completed_quests:%d", requirement.CompletedQuestCount))
	}
	return missing
}

func (requirement RankRequirement) anyRoleRequirementMet(snapshot ProgressionSnapshot) bool {
	for _, role := range requirement.AnyRoleTypes {
		roleLevel, ok := snapshot.RoleLevel(role)
		if !ok {
			continue
		}
		if roleLevel.Level >= requirement.AnyRoleMinLevel {
			return true
		}
	}
	return false
}

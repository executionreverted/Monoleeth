package progression

import "fmt"

const (
	MinProgressionLevel = 1
	MaxMVPLevel         = 5
	MinRank             = 1
	MaxMVPRank          = 5
)

var (
	defaultMainXPTable = XPTable{
		{Level: 1, RequiredXP: 0},
		{Level: 2, RequiredXP: 100},
		{Level: 3, RequiredXP: 300},
		{Level: 4, RequiredXP: 700},
		{Level: 5, RequiredXP: 1500},
	}
	defaultRoleXPTable = XPTable{
		{Level: 1, RequiredXP: 0},
		{Level: 2, RequiredXP: 75},
		{Level: 3, RequiredXP: 225},
		{Level: 4, RequiredXP: 500},
		{Level: 5, RequiredXP: 1000},
	}
)

// XPTableRow defines the total XP required to reach one level.
type XPTableRow struct {
	Level      int   `json:"level"`
	RequiredXP int64 `json:"required_xp"`
}

// XPTable is a deterministic total-XP threshold table.
type XPTable []XPTableRow

// MainXPTable returns a defensive copy of the deterministic MVP main XP table.
func MainXPTable() XPTable {
	return defaultMainXPTable.Clone()
}

// RoleXPTable returns a defensive copy of the deterministic MVP role XP table.
func RoleXPTable() XPTable {
	return defaultRoleXPTable.Clone()
}

// MainLevelForXP returns the MVP main level for totalXP.
func MainLevelForXP(totalXP int64) (int, error) {
	return LevelForXP(totalXP, defaultMainXPTable)
}

// RoleLevelForXP returns the MVP role level for totalXP.
func RoleLevelForXP(totalXP int64) (int, error) {
	return LevelForXP(totalXP, defaultRoleXPTable)
}

// LevelForXP returns the highest table level whose required XP is met.
func LevelForXP(totalXP int64, table XPTable) (int, error) {
	if totalXP < 0 {
		return 0, fmt.Errorf("xp %d: %w", totalXP, ErrNegativeXP)
	}
	if err := table.Validate(); err != nil {
		return 0, err
	}

	level := MinProgressionLevel
	for _, row := range table {
		if totalXP < row.RequiredXP {
			break
		}
		level = row.Level
	}
	return level, nil
}

// ValidateMainLevel reports whether level is covered by the MVP main XP table.
func ValidateMainLevel(level int) error {
	return validateLevel("main", level, defaultMainXPTable)
}

// ValidateRoleLevel reports whether level is covered by the MVP role XP table.
func ValidateRoleLevel(level int) error {
	return validateLevel("role", level, defaultRoleXPTable)
}

// ValidateRank reports whether rank is covered by the MVP rank range.
func ValidateRank(rank int) error {
	if rank < MinRank || rank > MaxMVPRank {
		return fmt.Errorf("rank %d outside %d-%d: %w", rank, MinRank, MaxMVPRank, ErrInvalidRank)
	}
	return nil
}

// RequiredXPForLevel returns the exact threshold for level.
func (table XPTable) RequiredXPForLevel(level int) (int64, error) {
	if err := table.Validate(); err != nil {
		return 0, err
	}
	for _, row := range table {
		if row.Level == level {
			return row.RequiredXP, nil
		}
	}
	return 0, fmt.Errorf("level %d: %w", level, ErrInvalidLevel)
}

// MaxLevel returns the highest level covered by table.
func (table XPTable) MaxLevel() (int, error) {
	if err := table.Validate(); err != nil {
		return 0, err
	}
	return table[len(table)-1].Level, nil
}

// Clone returns a defensive copy of table.
func (table XPTable) Clone() XPTable {
	return append(XPTable(nil), table...)
}

// Validate reports whether table has contiguous levels and increasing XP.
func (table XPTable) Validate() error {
	if len(table) == 0 {
		return ErrInvalidXPTable
	}
	if table[0].Level != MinProgressionLevel {
		return fmt.Errorf("first level %d: %w", table[0].Level, ErrInvalidXPTable)
	}
	if table[0].RequiredXP != 0 {
		return fmt.Errorf("first required xp %d: %w", table[0].RequiredXP, ErrInvalidXPTable)
	}

	seen := make(map[int]struct{}, len(table))
	previous := table[0]
	if previous.RequiredXP < 0 {
		return fmt.Errorf("level %d xp %d: %w", previous.Level, previous.RequiredXP, ErrNegativeXP)
	}
	seen[previous.Level] = struct{}{}

	for i := 1; i < len(table); i++ {
		row := table[i]
		if row.Level <= 0 {
			return fmt.Errorf("level %d: %w", row.Level, ErrInvalidLevel)
		}
		if _, ok := seen[row.Level]; ok {
			return fmt.Errorf("level %d: %w", row.Level, ErrDuplicateXPTableLevel)
		}
		if row.Level != previous.Level+1 {
			return fmt.Errorf("level %d after %d: %w", row.Level, previous.Level, ErrInvalidXPTable)
		}
		if row.RequiredXP < 0 {
			return fmt.Errorf("level %d xp %d: %w", row.Level, row.RequiredXP, ErrNegativeXP)
		}
		if row.RequiredXP <= previous.RequiredXP {
			return fmt.Errorf("level %d xp %d after %d: %w", row.Level, row.RequiredXP, previous.RequiredXP, ErrUnsortedXPTable)
		}
		seen[row.Level] = struct{}{}
		previous = row
	}
	return nil
}

func validateLevel(kind string, level int, table XPTable) error {
	maxLevel, err := table.MaxLevel()
	if err != nil {
		return err
	}
	if level < MinProgressionLevel || level > maxLevel {
		return fmt.Errorf("%s level %d outside %d-%d: %w", kind, level, MinProgressionLevel, maxLevel, ErrInvalidLevel)
	}
	return nil
}

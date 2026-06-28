package progression

import (
	"errors"
	"testing"
)

func TestMainXPTableThresholdsUseOldDarkOrbitShipXP(t *testing.T) {
	if err := MainXPTable().Validate(); err != nil {
		t.Fatalf("MainXPTable Validate() = %v, want nil", err)
	}
	assertTableThreshold(t, MainXPTable(), 0, 1)
	assertTableThreshold(t, MainXPTable(), 9_999, 1)
	assertTableThreshold(t, MainXPTable(), 10_000, 2)
	assertTableThreshold(t, MainXPTable(), 19_999, 2)
	assertTableThreshold(t, MainXPTable(), 20_000, 3)
	assertTableThreshold(t, MainXPTable(), 79_999, 4)
	assertTableThreshold(t, MainXPTable(), 80_000, 5)
	assertTableThreshold(t, MainXPTable(), 2_147_483_646, 19)
	assertTableThreshold(t, MainXPTable(), 2_147_483_647, 20)
	assertTableThreshold(t, MainXPTable(), 10_737_418_239_999, 31)
	assertTableThreshold(t, MainXPTable(), 10_737_418_240_000, 32)

	for rank := MinRank; rank <= MaxMVPRank; rank++ {
		if err := ValidateRank(rank); err != nil {
			t.Fatalf("ValidateRank(%d) = %v, want nil", rank, err)
		}
	}
}

func TestRoleXPTableThresholdsCoverMVPRanksOneThroughFive(t *testing.T) {
	if err := RoleXPTable().Validate(); err != nil {
		t.Fatalf("RoleXPTable Validate() = %v, want nil", err)
	}
	assertTableThreshold(t, RoleXPTable(), 0, 1)
	assertTableThreshold(t, RoleXPTable(), 74, 1)
	assertTableThreshold(t, RoleXPTable(), 75, 2)
	assertTableThreshold(t, RoleXPTable(), 224, 2)
	assertTableThreshold(t, RoleXPTable(), 225, 3)
	assertTableThreshold(t, RoleXPTable(), 499, 3)
	assertTableThreshold(t, RoleXPTable(), 500, 4)
	assertTableThreshold(t, RoleXPTable(), 999, 4)
	assertTableThreshold(t, RoleXPTable(), 1000, 5)
	assertTableThreshold(t, RoleXPTable(), 20_000, 5)
}

func TestXPTableCopiesAreDefensive(t *testing.T) {
	table := MainXPTable()
	table[1].RequiredXP = 1

	level, err := MainLevelForXP(99)
	if err != nil {
		t.Fatalf("MainLevelForXP() = %v, want nil", err)
	}
	if level != 1 {
		t.Fatalf("MainLevelForXP(99) after table mutation = %d, want 1", level)
	}
}

func TestXPTableValidationRejectsBrokenTables(t *testing.T) {
	tests := []struct {
		name    string
		table   XPTable
		wantErr error
	}{
		{
			name:    "empty",
			table:   nil,
			wantErr: ErrInvalidXPTable,
		},
		{
			name: "first level not one",
			table: XPTable{
				{Level: 2, RequiredXP: 0},
			},
			wantErr: ErrInvalidXPTable,
		},
		{
			name: "first xp not zero",
			table: XPTable{
				{Level: 1, RequiredXP: 1},
			},
			wantErr: ErrInvalidXPTable,
		},
		{
			name: "duplicate level",
			table: XPTable{
				{Level: 1, RequiredXP: 0},
				{Level: 2, RequiredXP: 100},
				{Level: 2, RequiredXP: 200},
			},
			wantErr: ErrDuplicateXPTableLevel,
		},
		{
			name: "gap",
			table: XPTable{
				{Level: 1, RequiredXP: 0},
				{Level: 3, RequiredXP: 100},
			},
			wantErr: ErrInvalidXPTable,
		},
		{
			name: "negative xp",
			table: XPTable{
				{Level: 1, RequiredXP: 0},
				{Level: 2, RequiredXP: -1},
			},
			wantErr: ErrNegativeXP,
		},
		{
			name: "unsorted xp",
			table: XPTable{
				{Level: 1, RequiredXP: 0},
				{Level: 2, RequiredXP: 0},
			},
			wantErr: ErrUnsortedXPTable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.table.Validate(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestLevelLookupRejectsNegativeXPAndInvalidLevels(t *testing.T) {
	if _, err := MainLevelForXP(-1); !errors.Is(err, ErrNegativeXP) {
		t.Fatalf("MainLevelForXP(-1) error = %v, want ErrNegativeXP", err)
	}
	if err := ValidateMainLevel(0); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("ValidateMainLevel(0) error = %v, want ErrInvalidLevel", err)
	}
	if err := ValidateRoleLevel(MaxMVPLevel + 1); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("ValidateRoleLevel(%d) error = %v, want ErrInvalidLevel", MaxMVPLevel+1, err)
	}
	if err := ValidateRank(MaxMVPRank + 1); !errors.Is(err, ErrInvalidRank) {
		t.Fatalf("ValidateRank(%d) error = %v, want ErrInvalidRank", MaxMVPRank+1, err)
	}
	if _, err := MainXPTable().RequiredXPForLevel(MaxMainLevel + 1); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("RequiredXPForLevel(%d) error = %v, want ErrInvalidLevel", MaxMainLevel+1, err)
	}
}

func assertTableThreshold(t *testing.T, table XPTable, xp int64, wantLevel int) {
	t.Helper()

	got, err := LevelForXP(xp, table)
	if err != nil {
		t.Fatalf("LevelForXP(%d) = %v, want nil", xp, err)
	}
	if got != wantLevel {
		t.Fatalf("LevelForXP(%d) = %d, want %d", xp, got, wantLevel)
	}
}

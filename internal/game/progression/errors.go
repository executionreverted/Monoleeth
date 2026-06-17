package progression

import "errors"

var (
	ErrInvalidRoleType             = errors.New("invalid role type")
	ErrEmptySkillNodeID            = errors.New("empty skill node id")
	ErrInvalidLevel                = errors.New("invalid progression level")
	ErrInvalidRank                 = errors.New("invalid progression rank")
	ErrNegativeXP                  = errors.New("negative xp")
	ErrNegativeSkillPoints         = errors.New("negative skill points")
	ErrSpentSkillPointsExceedTotal = errors.New("spent skill points exceed total")
	ErrInvalidXPTable              = errors.New("invalid xp table")
	ErrDuplicateXPTableLevel       = errors.New("duplicate xp table level")
	ErrUnsortedXPTable             = errors.New("unsorted xp table")
	ErrLevelXPMismatch             = errors.New("level does not match xp table")
	ErrSnapshotPlayerMismatch      = errors.New("progression snapshot player mismatch")
	ErrDuplicateRoleLevel          = errors.New("duplicate role level")
	ErrDuplicateSkillNodeUnlock    = errors.New("duplicate skill node unlock")
)

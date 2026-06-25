package contentdb

import (
	"context"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

type ProgressionStore struct {
	store *Store
}

var _ progression.Repository = (*ProgressionStore)(nil)

func NewProgressionStore(store *Store) (*ProgressionStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &ProgressionStore{store: store}, nil
}

func (store *ProgressionStore) LoadProgressionSnapshots(ctx context.Context) ([]progression.ProgressionSnapshot, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	players, err := store.loadProgressionPlayers(ctx)
	if err != nil {
		return nil, err
	}
	if len(players) == 0 {
		return nil, nil
	}
	roleLevels, err := store.loadRoleLevels(ctx)
	if err != nil {
		return nil, err
	}
	skillPoints, err := store.loadSkillPoints(ctx)
	if err != nil {
		return nil, err
	}
	unlockedNodes, err := store.loadUnlockedSkillNodes(ctx)
	if err != nil {
		return nil, err
	}

	playerIDs := make([]foundation.PlayerID, 0, len(players))
	for playerID := range players {
		playerIDs = append(playerIDs, playerID)
	}
	sort.Slice(playerIDs, func(i, j int) bool { return playerIDs[i] < playerIDs[j] })

	snapshots := make([]progression.ProgressionSnapshot, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		points, ok := skillPoints[playerID]
		if !ok {
			points = progression.SkillPointState{
				PlayerID:  playerID,
				UpdatedAt: players[playerID].UpdatedAt,
			}
		}
		snapshot, err := progression.NewProgressionSnapshot(players[playerID], roleLevels[playerID], points, unlockedNodes[playerID])
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func (store *ProgressionStore) SaveProgressionSnapshot(ctx context.Context, snapshot progression.ProgressionSnapshot) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)

	player := snapshot.Player
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO player_progression(player_id, main_xp, main_level, rank_level, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (player_id) DO UPDATE
		SET main_xp = EXCLUDED.main_xp,
			main_level = EXCLUDED.main_level,
			rank_level = EXCLUDED.rank_level,
			updated_at = EXCLUDED.updated_at
	`, player.PlayerID.String(), player.MainXP, player.MainLevel, player.Rank, player.CreatedAt.UTC(), player.UpdatedAt.UTC()); err != nil {
		return err
	}

	points := snapshot.SkillPoints
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO player_skill_points(player_id, total_points, spent_points, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (player_id) DO UPDATE
		SET total_points = EXCLUDED.total_points,
			spent_points = EXCLUDED.spent_points,
			updated_at = EXCLUDED.updated_at
	`, points.PlayerID.String(), points.TotalPoints, points.SpentPoints, points.UpdatedAt.UTC()); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM player_role_levels WHERE player_id = $1`, player.PlayerID.String()); err != nil {
		return err
	}
	for _, roleLevel := range snapshot.RoleLevels() {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO player_role_levels(player_id, role_type, level, xp, updated_at)
			VALUES ($1, $2, $3, $4, $5)
		`, roleLevel.PlayerID.String(), roleLevel.Role.String(), roleLevel.Level, roleLevel.XP, roleLevel.UpdatedAt.UTC()); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM player_unlocked_skill_nodes WHERE player_id = $1`, player.PlayerID.String()); err != nil {
		return err
	}
	for _, node := range snapshot.UnlockedSkillNodes() {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO player_unlocked_skill_nodes(player_id, node_id, unlocked_at)
			VALUES ($1, $2, $3)
		`, node.PlayerID.String(), node.NodeID.String(), node.UnlockedAt.UTC()); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

func (store *ProgressionStore) loadProgressionPlayers(ctx context.Context) (map[foundation.PlayerID]progression.PlayerProgressionState, error) {
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, main_xp, main_level, rank_level, created_at, updated_at
		FROM player_progression
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[foundation.PlayerID]progression.PlayerProgressionState)
	for rows.Next() {
		var playerID string
		var mainXP int64
		var mainLevel int
		var rank int
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(&playerID, &mainXP, &mainLevel, &rank, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		state := progression.PlayerProgressionState{
			PlayerID:  foundation.PlayerID(playerID),
			MainXP:    mainXP,
			MainLevel: mainLevel,
			Rank:      rank,
			CreatedAt: createdAt.UTC(),
			UpdatedAt: updatedAt.UTC(),
		}
		if err := state.Validate(); err != nil {
			return nil, err
		}
		out[state.PlayerID] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (store *ProgressionStore) loadRoleLevels(ctx context.Context) (map[foundation.PlayerID][]progression.RoleLevelState, error) {
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, role_type, level, xp, updated_at
		FROM player_role_levels
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[foundation.PlayerID][]progression.RoleLevelState)
	for rows.Next() {
		var playerID string
		var roleType string
		var level int
		var xp int64
		var updatedAt time.Time
		if err := rows.Scan(&playerID, &roleType, &level, &xp, &updatedAt); err != nil {
			return nil, err
		}
		state := progression.RoleLevelState{
			PlayerID:  foundation.PlayerID(playerID),
			Role:      progression.RoleType(roleType),
			Level:     level,
			XP:        xp,
			UpdatedAt: updatedAt.UTC(),
		}
		if err := state.Validate(); err != nil {
			return nil, err
		}
		out[state.PlayerID] = append(out[state.PlayerID], state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (store *ProgressionStore) loadSkillPoints(ctx context.Context) (map[foundation.PlayerID]progression.SkillPointState, error) {
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, total_points, spent_points, updated_at
		FROM player_skill_points
	`)
	if err != nil {
		if isUndefinedTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make(map[foundation.PlayerID]progression.SkillPointState)
	for rows.Next() {
		var playerID string
		var totalPoints int
		var spentPoints int
		var updatedAt time.Time
		if err := rows.Scan(&playerID, &totalPoints, &spentPoints, &updatedAt); err != nil {
			return nil, err
		}
		state := progression.SkillPointState{
			PlayerID:    foundation.PlayerID(playerID),
			TotalPoints: totalPoints,
			SpentPoints: spentPoints,
			UpdatedAt:   updatedAt.UTC(),
		}
		if err := state.Validate(); err != nil {
			return nil, err
		}
		out[state.PlayerID] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (store *ProgressionStore) loadUnlockedSkillNodes(ctx context.Context) (map[foundation.PlayerID][]progression.UnlockedSkillNodeState, error) {
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, node_id, unlocked_at
		FROM player_unlocked_skill_nodes
	`)
	if err != nil {
		if isUndefinedTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make(map[foundation.PlayerID][]progression.UnlockedSkillNodeState)
	for rows.Next() {
		var playerID string
		var nodeID string
		var unlockedAt time.Time
		if err := rows.Scan(&playerID, &nodeID, &unlockedAt); err != nil {
			return nil, err
		}
		state := progression.UnlockedSkillNodeState{
			PlayerID:   foundation.PlayerID(playerID),
			NodeID:     progression.SkillNodeID(nodeID),
			UnlockedAt: unlockedAt.UTC(),
		}
		if err := state.Validate(); err != nil {
			return nil, err
		}
		out[state.PlayerID] = append(out[state.PlayerID], state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isUndefinedTable(err error) bool {
	if err == nil {
		return false
	}
	if sqlErr, ok := err.(interface{ SQLState() string }); ok {
		return sqlErr.SQLState() == "42P01"
	}
	return false
}

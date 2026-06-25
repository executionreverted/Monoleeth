package contentdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

var (
	ErrNilHangarUpdate          = errors.New("nil hangar update")
	ErrNilHangarView            = errors.New("nil hangar view")
	ErrHangarPlayerMismatch     = errors.New("hangar player mismatch")
	ErrActiveShipMissingRow     = errors.New("active ship missing player ship row")
	ErrDuplicateHangarActiveRow = errors.New("duplicate active ship row")
)

type HangarStore struct {
	store *Store
}

var _ ships.HangarStore = (*HangarStore)(nil)

func NewHangarStore(store *Store) (*HangarStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &HangarStore{store: store}, nil
}

func (store *HangarStore) UpdatePlayerHangar(playerID foundation.PlayerID, update func(*ships.HangarRecord) error) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return err
	}
	if update == nil {
		return ErrNilHangarUpdate
	}

	ctx := context.Background()
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)

	if err = lockPlayerHangar(ctx, tx, playerID); err != nil {
		return err
	}
	record, err := loadPlayerHangarRecord(ctx, tx, playerID)
	if err != nil {
		return err
	}
	if err = update(&record); err != nil {
		return err
	}
	if err = replacePlayerHangarRecord(ctx, tx, playerID, record); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (store *HangarStore) ViewPlayerHangar(playerID foundation.PlayerID, view func(ships.HangarRecord) error) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return err
	}
	if view == nil {
		return ErrNilHangarView
	}

	record, err := loadPlayerHangarRecord(context.Background(), store.store.db, playerID)
	if err != nil {
		return err
	}
	return view(record)
}

type hangarReader interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func lockPlayerHangar(ctx context.Context, tx *sql.Tx, playerID foundation.PlayerID) error {
	var lockedPlayerID string
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM players
		WHERE id = $1
		FOR UPDATE
	`, playerID.String()).Scan(&lockedPlayerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func loadPlayerHangarRecord(ctx context.Context, reader hangarReader, playerID foundation.PlayerID) (ships.HangarRecord, error) {
	var record ships.HangarRecord
	rows, err := reader.QueryContext(ctx, `
		SELECT player_id, ship_id, unlocked_at, state, disabled_reason, disabled_at, last_repaired_at, metadata_json
		FROM player_ships
		WHERE player_id = $1
		ORDER BY ship_id
	`, playerID.String())
	if err != nil {
		return ships.HangarRecord{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var storedPlayerID string
		var shipID string
		var unlockedAt time.Time
		var state string
		var disabledReason string
		var disabledAt sql.NullTime
		var lastRepairedAt sql.NullTime
		var metadataJSON []byte
		if err := rows.Scan(&storedPlayerID, &shipID, &unlockedAt, &state, &disabledReason, &disabledAt, &lastRepairedAt, &metadataJSON); err != nil {
			return ships.HangarRecord{}, err
		}
		playerShip := ships.PlayerShipState{
			PlayerID:       foundation.PlayerID(storedPlayerID),
			ShipID:         foundation.ShipID(shipID),
			UnlockedAt:     unlockedAt.UTC(),
			State:          ships.ShipState(state),
			DisabledReason: disabledReason,
		}
		if disabledAt.Valid {
			disabled := disabledAt.Time.UTC()
			playerShip.DisabledAt = &disabled
		}
		if lastRepairedAt.Valid {
			repaired := lastRepairedAt.Time.UTC()
			playerShip.LastRepairedAt = &repaired
		}
		if len(metadataJSON) > 0 {
			playerShip.MetadataJSON = append([]byte(nil), metadataJSON...)
		}
		if err := playerShip.Validate(); err != nil {
			return ships.HangarRecord{}, err
		}
		record.PutPlayerShip(playerShip)
	}
	if err := rows.Err(); err != nil {
		return ships.HangarRecord{}, err
	}

	activeRows, err := reader.QueryContext(ctx, `
		SELECT player_id, ship_id, activated_at, updated_at
		FROM player_active_ships
		WHERE player_id = $1
	`, playerID.String())
	if err != nil {
		return ships.HangarRecord{}, err
	}
	defer activeRows.Close()

	activeCount := 0
	for activeRows.Next() {
		activeCount++
		if activeCount > 1 {
			return ships.HangarRecord{}, ErrDuplicateHangarActiveRow
		}
		var storedPlayerID string
		var shipID string
		var activatedAt time.Time
		var updatedAt time.Time
		if err := activeRows.Scan(&storedPlayerID, &shipID, &activatedAt, &updatedAt); err != nil {
			return ships.HangarRecord{}, err
		}
		activeShip := ships.ActiveShipState{
			PlayerID:    foundation.PlayerID(storedPlayerID),
			ShipID:      foundation.ShipID(shipID),
			ActivatedAt: activatedAt.UTC(),
			UpdatedAt:   updatedAt.UTC(),
		}
		if err := activeShip.Validate(); err != nil {
			return ships.HangarRecord{}, err
		}
		record.PutActiveShip(activeShip)
	}
	if err := activeRows.Err(); err != nil {
		return ships.HangarRecord{}, err
	}
	if err := validateHangarRecord(playerID, record); err != nil {
		return ships.HangarRecord{}, err
	}
	return record, nil
}

func replacePlayerHangarRecord(ctx context.Context, tx *sql.Tx, playerID foundation.PlayerID, record ships.HangarRecord) error {
	if err := validateHangarRecord(playerID, record); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM player_active_ships WHERE player_id = $1`, playerID.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM player_ships WHERE player_id = $1`, playerID.String()); err != nil {
		return err
	}
	for _, playerShip := range record.PlayerShips() {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO player_ships(player_id, ship_id, unlocked_at, state, disabled_reason, disabled_at, last_repaired_at, metadata_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, playerShip.PlayerID.String(), playerShip.ShipID.String(), playerShip.UnlockedAt.UTC(), playerShip.State.String(), playerShip.DisabledReason, nullableTime(playerShip.DisabledAt), nullableTime(playerShip.LastRepairedAt), nullableRawJSON(playerShip.MetadataJSON)); err != nil {
			return err
		}
	}
	if activeShip, ok := record.ActiveShip(); ok {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO player_active_ships(player_id, ship_id, activated_at, updated_at)
			VALUES ($1, $2, $3, $4)
		`, activeShip.PlayerID.String(), activeShip.ShipID.String(), activeShip.ActivatedAt.UTC(), activeShip.UpdatedAt.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func validateHangarRecord(playerID foundation.PlayerID, record ships.HangarRecord) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	shipIDs := make(map[foundation.ShipID]struct{})
	for _, playerShip := range record.PlayerShips() {
		if playerShip.PlayerID != playerID {
			return fmt.Errorf("%w: ship row player %q != %q", ErrHangarPlayerMismatch, playerShip.PlayerID, playerID)
		}
		if err := playerShip.Validate(); err != nil {
			return err
		}
		shipIDs[playerShip.ShipID] = struct{}{}
	}
	if activeShip, ok := record.ActiveShip(); ok {
		if activeShip.PlayerID != playerID {
			return fmt.Errorf("%w: active row player %q != %q", ErrHangarPlayerMismatch, activeShip.PlayerID, playerID)
		}
		if err := activeShip.Validate(); err != nil {
			return err
		}
		if _, ok := shipIDs[activeShip.ShipID]; !ok {
			return fmt.Errorf("%w: %s", ErrActiveShipMissingRow, activeShip.ShipID)
		}
	}
	return nil
}

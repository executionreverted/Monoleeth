package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

// AutomationRouteDurableStore is the Postgres-backed durable adapter for the
// automation route lifecycle (create/update/enable/disable). It enforces
// reference-key dedup and revision CAS atomically via two tables.
type AutomationRouteDurableStore struct {
	store *Store
}

var (
	_ production.AutomationRouteDurableCommitStore = (*AutomationRouteDurableStore)(nil)
	_ production.AutomationRouteDurableReader      = (*AutomationRouteDurableStore)(nil)
)

func NewAutomationRouteDurableStore(store *Store) (*AutomationRouteDurableStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &AutomationRouteDurableStore{store: store}, nil
}

func (s *AutomationRouteDurableStore) ApplyAutomationRouteDurableCommitPlan(
	plan production.AutomationRouteDurableCommitPlan,
) (result production.AutomationRouteDurableCommitResult, err error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.AutomationRouteDurableCommitResult{}, ErrNilDatabase
	}
	if plan.Route.RouteID == "" || plan.ReferenceKey == "" {
		return production.AutomationRouteDurableCommitResult{}, nil
	}
	if vErr := plan.Validate(); vErr != nil {
		return production.AutomationRouteDurableCommitResult{}, vErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	tx, txErr := s.store.db.BeginTx(ctx, nil)
	if txErr != nil {
		return production.AutomationRouteDurableCommitResult{}, txErr
	}
	defer rollbackUnlessCommitted(tx, &err)

	// Dedup: check if reference_key already committed.
	existingRef, refOk, refErr := loadRouteReferenceRecordForUpdate(ctx, tx, plan.ReferenceKey)
	if refErr != nil {
		return production.AutomationRouteDurableCommitResult{}, refErr
	}
	if refOk {
		if !routeDurableRecordsEqualByJSON(existingRef, plan, existingRef.Revision) {
			return production.AutomationRouteDurableCommitResult{}, fmt.Errorf("reference_conflict: %w", production.ErrInvalidAutomationRouteDurableCommit)
		}
		if err = tx.Commit(); err != nil {
			return production.AutomationRouteDurableCommitResult{}, err
		}
		return production.AutomationRouteDurableCommitResult{
			Record:    existingRef,
			Duplicate: true,
		}, nil
	}

	// CAS: read current route record under row lock.
	existingRoute, routeExists, routeErr := loadRouteRecordForUpdate(ctx, tx, plan.Route.RouteID)
	if routeErr != nil {
		return production.AutomationRouteDurableCommitResult{}, routeErr
	}
	if routeExists {
		if plan.ExpectedRevision != existingRoute.Revision {
			return production.AutomationRouteDurableCommitResult{}, fmt.Errorf("route %q expected revision %d current %d: %w",
				plan.Route.RouteID, plan.ExpectedRevision, existingRoute.Revision, production.ErrStaleAutomationRouteDurableCommit)
		}
	} else if plan.ExpectedRevision != 0 {
		return production.AutomationRouteDurableCommitResult{}, fmt.Errorf("route %q expected revision %d: %w",
			plan.Route.RouteID, plan.ExpectedRevision, production.ErrStaleAutomationRouteDurableCommit)
	}

	newRevision := plan.ExpectedRevision + 1
	record := production.AutomationRouteDurableRecord{
		Route:            plan.Route,
		SourceProductionState: plan.SourceProductionState,
		ReferenceKey:     plan.ReferenceKey,
		Revision:         newRevision,
		RecordedAt:       plan.RecordedAt,
	}
	recordJSON, mErr := json.Marshal(record)
	if mErr != nil {
		return production.AutomationRouteDurableCommitResult{}, fmt.Errorf("marshal route durable record: %w", mErr)
	}

	// Upsert route record.
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO automation_route_durable_records(route_id, reference_key, owner_player_id, revision, record_json)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (route_id) DO UPDATE
		SET reference_key = EXCLUDED.reference_key,
			owner_player_id = EXCLUDED.owner_player_id,
			revision = EXCLUDED.revision,
			record_json = EXCLUDED.record_json,
			committed_at = now()
	`,
		string(record.Route.RouteID),
		string(record.ReferenceKey),
		string(record.Route.OwnerPlayerID),
		record.Revision,
		string(recordJSON),
	); err != nil {
		return production.AutomationRouteDurableCommitResult{}, err
	}

	// Insert reference dedup row.
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO automation_route_durable_references(reference_key, route_id, revision, record_json)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (reference_key) DO NOTHING
	`,
		string(record.ReferenceKey),
		string(record.Route.RouteID),
		record.Revision,
		string(recordJSON),
	); err != nil {
		return production.AutomationRouteDurableCommitResult{}, err
	}

	if err = tx.Commit(); err != nil {
		return production.AutomationRouteDurableCommitResult{}, err
	}
	return production.AutomationRouteDurableCommitResult{Record: record}, nil
}

func (s *AutomationRouteDurableStore) CommittedAutomationRouteDurableRecord(
	routeID foundation.RouteID,
) (production.AutomationRouteDurableRecord, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.AutomationRouteDurableRecord{}, false, ErrNilDatabase
	}
	if err := routeID.Validate(); err != nil {
		return production.AutomationRouteDurableRecord{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	return loadRouteRecord(ctx, s.store.db, routeID)
}

func (s *AutomationRouteDurableStore) CommittedAutomationRouteDurableRecordByReference(
	referenceKey foundation.IdempotencyKey,
) (production.AutomationRouteDurableRecord, bool, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return production.AutomationRouteDurableRecord{}, false, ErrNilDatabase
	}
	if err := referenceKey.Validate(); err != nil {
		return production.AutomationRouteDurableRecord{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	row := s.store.db.QueryRowContext(ctx, `
		SELECT record_json FROM automation_route_durable_references WHERE reference_key = $1
	`, string(referenceKey))
	return scanRouteRecordRow(row)
}

func (s *AutomationRouteDurableStore) CommittedAutomationRouteDurableRecordsForOwner(
	playerID foundation.PlayerID,
) ([]production.AutomationRouteDurableRecord, error) {
	if s == nil || s.store == nil || s.store.db == nil {
		return nil, ErrNilDatabase
	}
	if err := playerID.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimLifecycleStoreTimeout)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT record_json FROM automation_route_durable_records
		WHERE owner_player_id = $1
		ORDER BY route_id
	`, string(playerID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]production.AutomationRouteDurableRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record production.AutomationRouteDurableRecord
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Route.RouteID < records[j].Route.RouteID
	})
	return records, rows.Err()
}

type routeRecordScanner interface {
	Scan(dest ...any) error
}

func scanRouteRecordRow(row routeRecordScanner) (production.AutomationRouteDurableRecord, bool, error) {
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return production.AutomationRouteDurableRecord{}, false, nil
		}
		return production.AutomationRouteDurableRecord{}, false, err
	}
	var record production.AutomationRouteDurableRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return production.AutomationRouteDurableRecord{}, false, err
	}
	return record, true, nil
}

func loadRouteRecord(
	ctx context.Context,
	runner queryContextRunner,
	routeID foundation.RouteID,
) (production.AutomationRouteDurableRecord, bool, error) {
	row := runner.QueryRowContext(ctx, `
		SELECT record_json FROM automation_route_durable_records WHERE route_id = $1
	`, string(routeID))
	return scanRouteRecordRow(row)
}

func loadRouteRecordForUpdate(
	ctx context.Context,
	tx *sql.Tx,
	routeID foundation.RouteID,
) (production.AutomationRouteDurableRecord, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT record_json FROM automation_route_durable_records WHERE route_id = $1 FOR UPDATE
	`, string(routeID))
	return scanRouteRecordRow(row)
}

func loadRouteReferenceRecordForUpdate(
	ctx context.Context,
	tx *sql.Tx,
	referenceKey foundation.IdempotencyKey,
) (production.AutomationRouteDurableRecord, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT record_json FROM automation_route_durable_references WHERE reference_key = $1
	`, string(referenceKey))
	return scanRouteRecordRow(row)
}

// routeDurableRecordsEqualByJSON compares the committed dedup record against
// the incoming plan by serializing the plan's route into a record with the
// same revision and comparing canonical JSON.
func routeDurableRecordsEqualByJSON(
	existing production.AutomationRouteDurableRecord,
	plan production.AutomationRouteDurableCommitPlan,
	revision uint64,
) bool {
	candidate := production.AutomationRouteDurableRecord{
		Route:                 plan.Route,
		SourceProductionState: plan.SourceProductionState,
		ReferenceKey:          plan.ReferenceKey,
		Revision:              revision,
		RecordedAt:            plan.RecordedAt,
	}
	existingJSON, err1 := json.Marshal(existing)
	candidateJSON, err2 := json.Marshal(candidate)
	if err1 != nil || err2 != nil {
		return false
	}
	var ev, cv any
	if json.Unmarshal(existingJSON, &ev) != nil || json.Unmarshal(candidateJSON, &cv) != nil {
		return false
	}
	evCanonical, _ := json.Marshal(ev)
	cvCanonical, _ := json.Marshal(cv)
	return string(evCanonical) == string(cvCanonical)
}

package discovery

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

// ClaimDurableLifecycleStore is the durable adapter contract for committing one
// completed planet-claim lifecycle: X Core storage mutation evidence, owner-CAS
// begin evidence, optional production initialization, and completion/outbox
// evidence.
type ClaimDurableLifecycleStore interface {
	ApplyClaimDurableLifecyclePlan(ClaimDurableLifecyclePlan) (ClaimDurableLifecycleResult, error)
}

// ClaimDurableLifecycleReader is the recovery/readback side of the durable
// claim lifecycle adapter.
type ClaimDurableLifecycleReader interface {
	CommittedClaimDurableLifecyclePlan(PlanetClaimReference) (ClaimDurableLifecyclePlan, bool, error)
	CommittedClaimOutboxDispatchPlan(PlanetClaimReference) (ClaimOutboxDispatchPlan, bool, error)
}

// ClaimDurableLifecycleResult reports the rows accepted by the durable claim
// lifecycle boundary. Exact replays return the original rows with Duplicate set.
type ClaimDurableLifecycleResult struct {
	Plan      ClaimDurableLifecyclePlan
	Duplicate bool
}

// ApplyDurableLifecycle validates and records this lifecycle plan through a
// durable claim lifecycle adapter.
func (plan ClaimDurableLifecyclePlan) ApplyDurableLifecycle(
	store ClaimDurableLifecycleStore,
) (ClaimDurableLifecycleResult, error) {
	if store == nil {
		return ClaimDurableLifecycleResult{}, ErrInvalidClaimDurableCommit
	}
	return store.ApplyClaimDurableLifecyclePlan(plan)
}

// InMemoryClaimDurableLifecycleStore is a process-local durable-table contract
// used by tests and future DB adapters.
type InMemoryClaimDurableLifecycleStore struct {
	mu         sync.RWMutex
	plans      map[PlanetClaimReference]ClaimDurableLifecyclePlan
	references []PlanetClaimReference
}

// NewInMemoryClaimDurableLifecycleStore returns an empty claim lifecycle
// durable commit adapter contract.
func NewInMemoryClaimDurableLifecycleStore() *InMemoryClaimDurableLifecycleStore {
	return &InMemoryClaimDurableLifecycleStore{
		plans: make(map[PlanetClaimReference]ClaimDurableLifecyclePlan),
	}
}

// ApplyClaimDurableLifecyclePlan atomically records one non-empty claim
// lifecycle. Empty plans are no-ops; exact reference replays are idempotent;
// conflicting reference reuse is rejected before mutation.
func (store *InMemoryClaimDurableLifecycleStore) ApplyClaimDurableLifecyclePlan(
	plan ClaimDurableLifecyclePlan,
) (ClaimDurableLifecycleResult, error) {
	if store == nil {
		return ClaimDurableLifecycleResult{}, ErrInvalidClaimDurableCommit
	}
	if claimDurableLifecyclePlanIsNoOp(plan) {
		return ClaimDurableLifecycleResult{}, nil
	}
	normalized, err := normalizeClaimDurableLifecyclePlan(plan)
	if err != nil {
		return ClaimDurableLifecycleResult{}, err
	}
	reference := normalized.Commit.Boundary.ClaimReference

	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if existing, ok := store.plans[reference]; ok {
		existingReplay, err := claimDurableLifecyclePlanForReplay(existing)
		if err != nil {
			return ClaimDurableLifecycleResult{}, err
		}
		if !claimDurableLifecyclePlansEqual(existingReplay, normalized) {
			return ClaimDurableLifecycleResult{}, fmt.Errorf("claim_reference_conflict: %w", ErrInvalidClaimDurableCommit)
		}
		return ClaimDurableLifecycleResult{Plan: cloneClaimDurableLifecyclePlan(existing), Duplicate: true}, nil
	}
	store.plans[reference] = cloneClaimDurableLifecyclePlan(normalized)
	store.references = append(store.references, reference)
	return ClaimDurableLifecycleResult{Plan: cloneClaimDurableLifecyclePlan(normalized)}, nil
}

// ClaimReferences returns committed claim references in commit order.
func (store *InMemoryClaimDurableLifecycleStore) ClaimReferences() []PlanetClaimReference {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	return append([]PlanetClaimReference(nil), store.references...)
}

// OutboxRecords returns committed claim lifecycle outbox rows in commit order.
func (store *InMemoryClaimDurableLifecycleStore) OutboxRecords() []ClaimOutboxRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	records := make([]ClaimOutboxRecord, 0, len(store.references))
	for _, reference := range store.references {
		records = append(records, store.plans[reference].Commit.Outbox)
	}
	return cloneClaimOutboxRecords(records)
}

// ClaimPendingClaimOutboxRecordsForPublish moves committed pending claim
// outbox rows to in-flight in commit order.
func (store *InMemoryClaimDurableLifecycleStore) ClaimPendingClaimOutboxRecordsForPublish(
	limit int,
	claimedAt time.Time,
) ([]ClaimOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	if limit <= 0 {
		return nil, nil
	}
	claimedAt = claimedAt.UTC()
	if claimedAt.IsZero() {
		claimedAt = time.Unix(0, 0).UTC()
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateClaimDurableLifecycleReadbacksLocked(); err != nil {
		return nil, err
	}

	records := make([]ClaimOutboxRecord, 0, limit)
	for _, reference := range store.references {
		if len(records) >= limit {
			break
		}
		plan := store.plans[reference]
		if plan.Commit.Outbox.Status != ClaimOutboxStatusPending {
			continue
		}
		plan.Commit.Outbox.Status = ClaimOutboxStatusInFlight
		plan.Commit.Outbox.ClaimedAt = claimedAt
		plan.Commit.Outbox.Attempts++
		plan.Commit.Outbox.ClaimToken = claimOutboxClaimToken(
			plan.Commit.Outbox.OutboxID,
			plan.Commit.Outbox.Attempts,
		)
		store.plans[reference] = cloneClaimDurableLifecyclePlan(plan)
		records = append(records, cloneClaimOutboxRecord(plan.Commit.Outbox))
	}
	return records, nil
}

// MarkClaimOutboxPublished records successful delivery for the current claim
// token on a committed claim lifecycle outbox row.
func (store *InMemoryClaimDurableLifecycleStore) MarkClaimOutboxPublished(
	outboxID string,
	claimToken string,
	publishedAt time.Time,
) (ClaimOutboxRecord, bool, error) {
	if store == nil {
		return ClaimOutboxRecord{}, false, ErrInvalidClaimOutboxPublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateClaimDurableLifecycleReadbacksLocked(); err != nil {
		return ClaimOutboxRecord{}, false, err
	}

	reference, ok := store.outboxReferenceLocked(outboxID)
	if !ok {
		return ClaimOutboxRecord{}, false, nil
	}
	plan := store.plans[reference]
	if !claimDurableLifecycleOutboxClaimMatches(plan.Commit.Outbox, claimToken) {
		return ClaimOutboxRecord{}, false, nil
	}
	plan.Commit.Outbox.Status = ClaimOutboxStatusPublished
	plan.Commit.Outbox.PublishedAt = publishedAt.UTC()
	plan.Commit.Outbox.FailedAt = time.Time{}
	plan.Commit.Outbox.LastError = ""
	store.plans[reference] = cloneClaimDurableLifecyclePlan(plan)
	return cloneClaimOutboxRecord(plan.Commit.Outbox), true, nil
}

// MarkClaimOutboxFailed records failed delivery for the current claim token on
// a committed claim lifecycle outbox row.
func (store *InMemoryClaimDurableLifecycleStore) MarkClaimOutboxFailed(
	outboxID string,
	claimToken string,
	reason string,
	failedAt time.Time,
) (ClaimOutboxRecord, bool, error) {
	if store == nil {
		return ClaimOutboxRecord{}, false, ErrInvalidClaimOutboxPublisher
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateClaimDurableLifecycleReadbacksLocked(); err != nil {
		return ClaimOutboxRecord{}, false, err
	}

	reference, ok := store.outboxReferenceLocked(outboxID)
	if !ok {
		return ClaimOutboxRecord{}, false, nil
	}
	plan := store.plans[reference]
	if !claimDurableLifecycleOutboxClaimMatches(plan.Commit.Outbox, claimToken) {
		return ClaimOutboxRecord{}, false, nil
	}
	plan.Commit.Outbox.Status = ClaimOutboxStatusFailed
	plan.Commit.Outbox.FailedAt = failedAt.UTC()
	plan.Commit.Outbox.LastError = reason
	store.plans[reference] = cloneClaimDurableLifecyclePlan(plan)
	return cloneClaimOutboxRecord(plan.Commit.Outbox), true, nil
}

// ReleaseExpiredClaimOutboxRecordsForPublish returns stale committed claim
// outbox leases to pending.
func (store *InMemoryClaimDurableLifecycleStore) ReleaseExpiredClaimOutboxRecordsForPublish(
	limit int,
	claimedBefore time.Time,
	releasedAt time.Time,
) ([]ClaimOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	if limit <= 0 || claimedBefore.IsZero() {
		return nil, nil
	}
	claimedBefore = claimedBefore.UTC()
	releasedAt = releasedAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateClaimDurableLifecycleReadbacksLocked(); err != nil {
		return nil, err
	}

	records := make([]ClaimOutboxRecord, 0, limit)
	for _, reference := range store.references {
		if len(records) >= limit {
			break
		}
		plan := store.plans[reference]
		record := plan.Commit.Outbox
		if record.Status != ClaimOutboxStatusInFlight ||
			record.ClaimedAt.IsZero() ||
			!record.ClaimedAt.Before(claimedBefore) {
			continue
		}
		plan.Commit.Outbox.Status = ClaimOutboxStatusPending
		plan.Commit.Outbox.ClaimedAt = time.Time{}
		plan.Commit.Outbox.ClaimToken = ""
		plan.Commit.Outbox.RetriedAt = releasedAt
		store.plans[reference] = cloneClaimDurableLifecyclePlan(plan)
		records = append(records, cloneClaimOutboxRecord(plan.Commit.Outbox))
	}
	return records, nil
}

// RetryFailedClaimOutboxRecordsForPublish returns failed committed claim
// outbox rows to pending in commit order while preserving failure evidence.
func (store *InMemoryClaimDurableLifecycleStore) RetryFailedClaimOutboxRecordsForPublish(
	limit int,
	retriedAt time.Time,
) ([]ClaimOutboxRecord, error) {
	if store == nil {
		return nil, ErrInvalidClaimOutboxPublisher
	}
	if limit <= 0 {
		return nil, nil
	}
	retriedAt = retriedAt.UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateClaimDurableLifecycleReadbacksLocked(); err != nil {
		return nil, err
	}

	records := make([]ClaimOutboxRecord, 0, limit)
	for _, reference := range store.references {
		if len(records) >= limit {
			break
		}
		plan := store.plans[reference]
		if plan.Commit.Outbox.Status != ClaimOutboxStatusFailed {
			continue
		}
		plan.Commit.Outbox.Status = ClaimOutboxStatusPending
		plan.Commit.Outbox.ClaimedAt = time.Time{}
		plan.Commit.Outbox.ClaimToken = ""
		plan.Commit.Outbox.RetriedAt = retriedAt
		store.plans[reference] = cloneClaimDurableLifecyclePlan(plan)
		records = append(records, cloneClaimOutboxRecord(plan.Commit.Outbox))
	}
	return records, nil
}

// CommittedClaimDurableLifecyclePlan returns the validated committed lifecycle
// plan for one claim reference.
func (store *InMemoryClaimDurableLifecycleStore) CommittedClaimDurableLifecyclePlan(
	reference PlanetClaimReference,
) (ClaimDurableLifecyclePlan, bool, error) {
	if store == nil {
		return ClaimDurableLifecyclePlan{}, false, ErrInvalidClaimDurableCommit
	}
	if err := reference.Validate(); err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	plan, ok := store.plans[reference]
	if !ok {
		return ClaimDurableLifecyclePlan{}, false, nil
	}
	cloned := cloneClaimDurableLifecyclePlan(plan)
	if err := validateClaimDurableLifecycleReadbackPlan(cloned); err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	return cloned, true, nil
}

// CommittedClaimOutboxDispatchPlan returns the validated publisher dispatch
// handoff for one committed claim reference.
func (store *InMemoryClaimDurableLifecycleStore) CommittedClaimOutboxDispatchPlan(
	reference PlanetClaimReference,
) (ClaimOutboxDispatchPlan, bool, error) {
	plan, ok, err := store.CommittedClaimDurableLifecyclePlan(reference)
	if err != nil || !ok {
		return ClaimOutboxDispatchPlan{}, ok, err
	}
	dispatch, err := NewClaimOutboxDispatchPlan(&plan.Commit.Reference, &plan.Commit.Outbox)
	if err != nil {
		return ClaimOutboxDispatchPlan{}, false, err
	}
	return dispatch, true, nil
}

func (store *InMemoryClaimDurableLifecycleStore) ensureMapsLocked() {
	if store.plans == nil {
		store.plans = make(map[PlanetClaimReference]ClaimDurableLifecyclePlan)
	}
}

func (store *InMemoryClaimDurableLifecycleStore) outboxReferenceLocked(outboxID string) (PlanetClaimReference, bool) {
	for _, reference := range store.references {
		if store.plans[reference].Commit.Outbox.OutboxID == outboxID {
			return reference, true
		}
	}
	return "", false
}

func claimDurableLifecycleOutboxClaimMatches(record ClaimOutboxRecord, claimToken string) bool {
	return record.Status == ClaimOutboxStatusInFlight && record.ClaimToken != "" && record.ClaimToken == claimToken
}

func (store *InMemoryClaimDurableLifecycleStore) validateClaimDurableLifecycleReadbacksLocked() error {
	for _, reference := range store.references {
		plan, ok := store.plans[reference]
		if !ok {
			return ErrInvalidClaimDurableCommit
		}
		if err := validateClaimDurableLifecycleReadbackPlan(plan); err != nil {
			return err
		}
	}
	return nil
}

func validateClaimDurableLifecycleReadbackPlan(plan ClaimDurableLifecyclePlan) error {
	cloned := cloneClaimDurableLifecyclePlan(plan)
	if err := validateClaimOutboxReadbackState(cloned.Commit.Outbox); err != nil {
		return err
	}
	cloned.Commit.Outbox = pendingClaimOutboxRecordForCommitValidation(cloned.Commit.Outbox)
	if _, err := normalizeClaimDurableLifecyclePlan(cloned); err != nil {
		return err
	}
	return nil
}

func normalizeClaimDurableLifecyclePlan(plan ClaimDurableLifecyclePlan) (ClaimDurableLifecyclePlan, error) {
	if !plan.HasProductionInit && !reflect.DeepEqual(plan.ProductionInitialized, ClaimProductionInitializationDurablePlan{}) {
		return ClaimDurableLifecyclePlan{}, fmt.Errorf("production_initialization: %w", ErrInvalidClaimDurableCommit)
	}
	begin, err := normalizeClaimDurableLifecycleBeginPlan(plan.Begin)
	if err != nil {
		return ClaimDurableLifecyclePlan{}, fmt.Errorf("begin: %w", err)
	}
	commit, err := normalizeClaimDurableLifecycleCommitPlan(plan.Commit)
	if err != nil {
		return ClaimDurableLifecyclePlan{}, fmt.Errorf("commit: %w", err)
	}
	var productionInit *ClaimProductionInitializationDurablePlan
	if plan.HasProductionInit {
		normalizedInit, err := normalizeClaimDurableLifecycleProductionInitPlan(plan.ProductionInitialized)
		if err != nil {
			return ClaimDurableLifecyclePlan{}, fmt.Errorf("production_initialization: %w", err)
		}
		productionInit = &normalizedInit
	}
	return NewClaimDurableLifecyclePlan(&begin, productionInit, &commit)
}

func claimDurableLifecyclePlanForReplay(plan ClaimDurableLifecyclePlan) (ClaimDurableLifecyclePlan, error) {
	cloned := cloneClaimDurableLifecyclePlan(plan)
	if err := validateClaimDurableLifecycleReadbackPlan(cloned); err != nil {
		return ClaimDurableLifecyclePlan{}, err
	}
	cloned.Commit.Outbox = pendingClaimOutboxRecordForCommitValidation(cloned.Commit.Outbox)
	return cloned, nil
}

func normalizeClaimDurableLifecycleBeginPlan(plan ClaimDurableBeginPlan) (ClaimDurableBeginPlan, error) {
	if err := validateClaimDurableBeginPlan(plan); err != nil {
		return ClaimDurableBeginPlan{}, err
	}
	if !plan.HasXCoreStorageMutation {
		return ClaimDurableBeginPlan{}, ErrInvalidClaimDurableCommit
	}
	return NewClaimDurableBeginPlan(
		&plan.XCoreStorageMutation,
		&plan.Planet,
		&plan.Boundary,
		plan.StaleIntel,
	)
}

func normalizeClaimDurableLifecycleCommitPlan(plan ClaimDurableCommitPlan) (ClaimDurableCommitPlan, error) {
	var xcore *ClaimXCoreConsumptionRecord
	if claimLifecycleXCorePresent(plan.XCoreConsumption) {
		xcore = &plan.XCoreConsumption
	}
	return NewClaimDurableCommitPlan(
		&plan.Boundary,
		&plan.Reference,
		&plan.Event,
		&plan.Outbox,
		xcore,
	)
}

func normalizeClaimDurableLifecycleProductionInitPlan(
	plan ClaimProductionInitializationDurablePlan,
) (ClaimProductionInitializationDurablePlan, error) {
	return normalizeClaimProductionInitializationDurablePlan(plan)
}

func claimDurableLifecyclePlanIsNoOp(plan ClaimDurableLifecyclePlan) bool {
	return reflect.DeepEqual(plan, ClaimDurableLifecyclePlan{})
}

func cloneClaimDurableLifecyclePlan(plan ClaimDurableLifecyclePlan) ClaimDurableLifecyclePlan {
	plan.Begin = cloneClaimDurableBeginPlan(plan.Begin)
	plan.ProductionInitialized = cloneClaimProductionInitializationDurablePlan(plan.ProductionInitialized)
	plan.Commit = cloneClaimDurableCommitPlan(plan.Commit)
	return plan
}

func claimDurableLifecyclePlansEqual(left ClaimDurableLifecyclePlan, right ClaimDurableLifecyclePlan) bool {
	return reflect.DeepEqual(cloneClaimDurableLifecyclePlan(left), cloneClaimDurableLifecyclePlan(right))
}

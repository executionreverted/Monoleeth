package discovery

import (
	"errors"
	"testing"
	"time"
)

func TestClaimOutboxDispatchPlanFromClaimLifecycle(t *testing.T) {
	lifecycle := claimDurableLifecyclePlanForStoreTest(t)

	plan, err := NewClaimOutboxDispatchPlan(&lifecycle.Commit.Reference, &lifecycle.Commit.Outbox)
	if err != nil {
		t.Fatalf("NewClaimOutboxDispatchPlan() error = %v, want nil", err)
	}
	if plan.Reference.ClaimReference != lifecycle.Commit.Boundary.ClaimReference ||
		plan.Outbox.ClaimReference != lifecycle.Commit.Boundary.ClaimReference ||
		plan.Outbox.Event.Type != ClaimEventPlanetClaimed {
		t.Fatalf("dispatch plan = %+v, want committed claim reference/outbox evidence", plan)
	}

	plan.Outbox.OutboxID = "mutated-outbox"
	replayed, err := NewClaimOutboxDispatchPlan(&lifecycle.Commit.Reference, &lifecycle.Commit.Outbox)
	if err != nil {
		t.Fatalf("NewClaimOutboxDispatchPlan(replay) error = %v, want nil", err)
	}
	if replayed.Outbox.OutboxID == "mutated-outbox" {
		t.Fatal("mutating dispatch plan changed source outbox row")
	}
}

func TestClaimOutboxDispatchPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewClaimOutboxDispatchPlan(nil, nil); err != nil || plan.Reference.ClaimReference != "" || plan.Outbox.OutboxID != "" {
		t.Fatalf("NewClaimOutboxDispatchPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	lifecycle := claimDurableLifecyclePlanForStoreTest(t)
	cases := map[string]func(ClaimReferenceRecord, ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord){
		"missing reference": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			return nil, &outbox
		},
		"missing outbox": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			return &reference, nil
		},
		"already owned reference": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			reference.AlreadyOwned = true
			return &reference, &outbox
		},
		"published row": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			outbox.Status = ClaimOutboxStatusPublished
			return &reference, &outbox
		},
		"stale claim token": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			outbox.ClaimToken = "claim-outbox-1-attempt-1"
			return &reference, &outbox
		},
		"mismatched reference": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			outbox.ClaimReference = "claim-other"
			return &reference, &outbox
		},
		"wrong event": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			outbox.Event.PlanetID = "planet-other"
			return &reference, &outbox
		},
		"bad reference time": func(reference ClaimReferenceRecord, outbox ClaimOutboxRecord) (*ClaimReferenceRecord, *ClaimOutboxRecord) {
			reference.RecordedAt = time.Time{}
			return &reference, &outbox
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			reference, outbox := mutate(
				cloneClaimReferenceRecord(lifecycle.Commit.Reference),
				cloneClaimOutboxRecord(lifecycle.Commit.Outbox),
			)
			_, err := NewClaimOutboxDispatchPlan(reference, outbox)
			if !errors.Is(err, ErrInvalidClaimOutboxDispatch) {
				t.Fatalf("NewClaimOutboxDispatchPlan(%s) error = %v, want ErrInvalidClaimOutboxDispatch", name, err)
			}
		})
	}
}

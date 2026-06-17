package economy

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestReservationKindsSupportCraftMarketAndAuction(t *testing.T) {
	want := []struct {
		kind         ReservationKind
		locationKind LocationKind
	}{
		{ReservationKindCraft, LocationKindCraftingReserved},
		{ReservationKindMarket, LocationKindMarketEscrow},
		{ReservationKindAuction, LocationKindAuctionEscrow},
	}

	got := SupportedReservationKinds()
	if len(got) != len(want) {
		t.Fatalf("SupportedReservationKinds len = %d, want %d", len(got), len(want))
	}
	for i, tc := range want {
		if got[i] != tc.kind {
			t.Fatalf("SupportedReservationKinds[%d] = %q, want %q", i, got[i], tc.kind)
		}
		if err := tc.kind.Validate(); err != nil {
			t.Fatalf("%q Validate() = %v, want nil", tc.kind, err)
		}
		if tc.kind.String() != string(tc.kind) {
			t.Fatalf("%q String() = %q, want %q", tc.kind, tc.kind.String(), string(tc.kind))
		}
		locationKind, err := tc.kind.ReservedLocationKind()
		if err != nil {
			t.Fatalf("%q ReservedLocationKind() error = %v, want nil", tc.kind, err)
		}
		if locationKind != tc.locationKind {
			t.Fatalf("%q ReservedLocationKind() = %q, want %q", tc.kind, locationKind, tc.locationKind)
		}
	}
}

func TestReservationRejectsBlankIDsReferenceAndEmptyAssets(t *testing.T) {
	itemLine := validReservationItemLine(t)
	referenceKey := validReferenceKey(t, "craft_start:job-1")

	if _, err := NewReservation("", ReservationKindCraft, "player-1", referenceKey, []ReservationItemLine{itemLine}, nil); !errors.Is(err, ErrEmptyReservationID) {
		t.Fatalf("blank reservation id error = %v, want ErrEmptyReservationID", err)
	}
	if _, err := NewReservation("reservation-1", ReservationKind("trade"), "player-1", referenceKey, []ReservationItemLine{itemLine}, nil); !errors.Is(err, ErrInvalidReservationKind) {
		t.Fatalf("invalid reservation kind error = %v, want ErrInvalidReservationKind", err)
	}
	if _, err := NewReservation("reservation-1", ReservationKindCraft, "", referenceKey, []ReservationItemLine{itemLine}, nil); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank reservation player id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewReservation("reservation-1", ReservationKindCraft, "player-1", "", []ReservationItemLine{itemLine}, nil); !errors.Is(err, foundation.ErrEmptyIdempotencyKey) {
		t.Fatalf("blank reservation reference key error = %v, want foundation.ErrEmptyIdempotencyKey", err)
	}
	if _, err := NewReservation("reservation-1", ReservationKindCraft, "player-1", referenceKey, nil, nil); !errors.Is(err, ErrEmptyReservationAssets) {
		t.Fatalf("empty reservation assets error = %v, want ErrEmptyReservationAssets", err)
	}
}

func TestReservationRejectsZeroQuantityAndCurrencyAmount(t *testing.T) {
	itemLine := validReservationItemLine(t)
	itemLine.Quantity = foundation.Quantity{}
	if err := itemLine.Validate(); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("zero reservation quantity error = %v, want foundation.ErrNonPositiveAmount", err)
	}

	currencyLine := validReservationCurrencyLine(t)
	currencyLine.Amount = foundation.Money{}
	if err := currencyLine.Validate(); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("zero reservation currency amount error = %v, want foundation.ErrNonPositiveAmount", err)
	}
}

func TestReservationRejectsInvalidStateItemAndCurrencyFields(t *testing.T) {
	reservation := validReservation(t)
	reservation.State = ReservationState("pending")
	if err := reservation.Validate(); !errors.Is(err, ErrInvalidReservationState) {
		t.Fatalf("invalid reservation state error = %v, want ErrInvalidReservationState", err)
	}

	itemLine := validReservationItemLine(t)
	itemLine.ItemID = ""
	if err := itemLine.Validate(); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank reservation item id error = %v, want foundation.ErrEmptyID", err)
	}

	currencyLine := validReservationCurrencyLine(t)
	currencyLine.Currency = CurrencyBucket("gold")
	if err := currencyLine.Validate(); !errors.Is(err, ErrInvalidCurrencyBucket) {
		t.Fatalf("invalid reservation currency error = %v, want ErrInvalidCurrencyBucket", err)
	}
}

func TestReservationJSONAndStringBehaviorIsStable(t *testing.T) {
	reservation := validReservation(t)
	createdAt := time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC)
	reservation.CreatedAt = createdAt
	reservation.ExpiresAt = &expiresAt

	payload, err := json.Marshal(reservation)
	if err != nil {
		t.Fatalf("json marshal reservation: %v", err)
	}
	want := `{"reservation_id":"reservation-1","reservation_kind":"craft","state":"active","player_id":"player-1","reference_id":"craft_start:job-1","item_lines":[{"item_id":"iron_ore","quantity":5,"from_location":{"location_type":"account_inventory","location_id":"player-1"},"reserved_location":{"location_type":"crafting_reserved","location_id":"craft-job-1"}}],"currency_lines":[{"currency_type":"credits","amount":50}],"created_at":"2026-06-17T15:00:00Z","expires_at":"2026-06-17T16:00:00Z"}`
	if got := string(payload); got != want {
		t.Fatalf("reservation JSON = %s, want %s", got, want)
	}

	if got := ReservationStateActive.String(); got != "active" {
		t.Fatalf("ReservationState.String() = %q, want active", got)
	}
	if got := ReservationID("reservation-1").String(); got != "reservation-1" {
		t.Fatalf("ReservationID.String() = %q, want reservation-1", got)
	}
}

func validReservation(t *testing.T) Reservation {
	t.Helper()

	reservation, err := NewReservation(
		"reservation-1",
		ReservationKindCraft,
		"player-1",
		validReferenceKey(t, "craft_start:job-1"),
		[]ReservationItemLine{validReservationItemLine(t)},
		[]ReservationCurrencyLine{validReservationCurrencyLine(t)},
	)
	if err != nil {
		t.Fatalf("NewReservation valid value: %v", err)
	}
	return reservation
}

func validReservationItemLine(t *testing.T) ReservationItemLine {
	t.Helper()

	reservedLocation, err := NewItemLocation(LocationKindCraftingReserved, "craft-job-1")
	if err != nil {
		t.Fatalf("NewItemLocation reserved valid value: %v", err)
	}
	return ReservationItemLine{
		ItemID:           "iron_ore",
		Quantity:         validQuantity(t, 5),
		FromLocation:     validLocation(t),
		ReservedLocation: reservedLocation,
	}
}

func validReservationCurrencyLine(t *testing.T) ReservationCurrencyLine {
	t.Helper()

	return ReservationCurrencyLine{
		Currency: CurrencyBucketCredits,
		Amount:   validMoney(t, 50),
	}
}

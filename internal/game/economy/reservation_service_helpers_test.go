package economy

import (
	"testing"
)

func validReserveItemsInput(t *testing.T) ReserveItemsInput {
	t.Helper()

	return ReserveItemsInput{
		ReservationID:      "craft-reservation-1",
		Kind:               ReservationKindCraft,
		PlayerID:           "player-1",
		ReservedLocationID: "craft-job-1",
		Reason:             "reserve_items",
		ReferenceKey:       validReferenceKey(t, "craft_complete:job-1"),
		Requirements: []ReserveItemRequirement{
			{
				Definition:   validStackableDefinition(t),
				Quantity:     1,
				FromLocation: validLocation(t),
			},
		},
	}
}

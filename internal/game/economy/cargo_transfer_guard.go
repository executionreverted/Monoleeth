package economy

import "gameproject/internal/game/foundation"

// CargoTransferGuardInput describes a player-facing item movement that can
// affect ship cargo while another authoritative domain owns that state.
type CargoTransferGuardInput struct {
	PlayerID     foundation.PlayerID
	FromLocation ItemLocation
	ToLocation   ItemLocation
	Reason       LedgerReason
	ReferenceKey foundation.IdempotencyKey
}

// InvolvesShipCargo reports whether this movement reads from or writes to ship cargo.
func (input CargoTransferGuardInput) InvolvesShipCargo() bool {
	return input.FromLocation.Kind == LocationKindShipCargo || input.ToLocation.Kind == LocationKindShipCargo
}

// CargoTransferGuard can reject player-facing cargo moves while another domain
// is processing authoritative state for that player.
type CargoTransferGuard interface {
	ValidateCargoTransfer(CargoTransferGuardInput) error
}

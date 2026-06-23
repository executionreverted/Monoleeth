package intel

import "errors"

var (
	ErrInvalidIntel                  = errors.New("invalid planet intel")
	ErrInvalidIntelState             = errors.New("invalid intel state")
	ErrInvalidIntelSource            = errors.New("invalid intel source")
	ErrInvalidIntelConfidence        = errors.New("invalid intel confidence")
	ErrInvalidCoordinateItem         = errors.New("invalid coordinate item")
	ErrInvalidReference              = errors.New("invalid intel reference")
	ErrZeroTimestamp                 = errors.New("zero timestamp")
	ErrPlanetIntelNotKnown           = errors.New("planet intel not known")
	ErrPlanetIntelInvalidated        = errors.New("planet intel invalidated")
	ErrPlanetIntelNotShareable       = errors.New("planet intel not shareable")
	ErrReferenceConflict             = errors.New("intel reference conflict")
	ErrCoordinateItemAlreadyExists   = errors.New("coordinate item already exists")
	ErrCoordinateItemNotFound        = errors.New("coordinate item not found")
	ErrCoordinateItemNotOwned        = errors.New("coordinate item not owned")
	ErrCoordinateItemAlreadyUsed     = errors.New("coordinate item already used")
	ErrCoordinateItemPayloadMismatch = errors.New("coordinate item payload mismatch")
)

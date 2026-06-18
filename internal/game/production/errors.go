package production

import "errors"

var (
	ErrDuplicateProductionDefinition = errors.New("duplicate production definition")
	ErrDuplicateBuildingDefinition   = errors.New("duplicate building production definition")
	ErrUnknownProductionDefinition   = errors.New("unknown production definition")
	ErrProductionSourceMismatch      = errors.New("production source definition mismatch")
	ErrInvalidBuildingCategory       = errors.New("invalid building category")
	ErrInvalidBuildingType           = errors.New("invalid building type")
	ErrInvalidBuildingLevel          = errors.New("invalid building level")
	ErrInvalidBuildingState          = errors.New("invalid building state")
	ErrEmptyProductionOutputs        = errors.New("empty production outputs")
	ErrEmptyRefineryInputs           = errors.New("empty refinery inputs")
	ErrUnexpectedExtractorInputs     = errors.New("unexpected extractor inputs")
	ErrDuplicateProductionInput      = errors.New("duplicate production input")
	ErrDuplicateProductionOutput     = errors.New("duplicate production output")
	ErrInvalidProductionRate         = errors.New("invalid production rate per hour")
	ErrInvalidEnergyCost             = errors.New("invalid energy cost per hour")
)

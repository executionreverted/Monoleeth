package crafting

import "errors"

var (
	ErrDuplicateRecipeDefinition = errors.New("duplicate recipe definition")
	ErrUnknownRecipeDefinition   = errors.New("unknown recipe definition")
	ErrRecipeSourceMismatch      = errors.New("recipe source definition mismatch")
	ErrInvalidRecipeCategory     = errors.New("invalid recipe category")
	ErrInvalidRecipeOutputKind   = errors.New("invalid recipe output kind")
	ErrEmptyRecipeInputs         = errors.New("empty recipe inputs")
	ErrDuplicateRecipeInput      = errors.New("duplicate recipe input")
	ErrInvalidRequiredRank       = errors.New("invalid required rank")
	ErrInvalidRequiredRole       = errors.New("invalid required role")
	ErrInvalidRequiredRoleLevel  = errors.New("invalid required role level")
	ErrDuplicateRoleRequirement  = errors.New("duplicate role requirement")
	ErrInvalidCraftLocationType  = errors.New("invalid craft location type")
	ErrEmptyCraftLocationID      = errors.New("empty craft location id")
	ErrLocationRequirementNotMet = errors.New("location requirement not met")
	ErrRankRequirementNotMet     = errors.New("rank requirement not met")
	ErrRoleRequirementNotMet     = errors.New("role requirement not met")
	ErrInvalidCraftDuration      = errors.New("invalid craft duration")
	ErrEmptyCraftJobID           = errors.New("empty craft job id")
	ErrInvalidCraftJobState      = errors.New("invalid craft job state")
	ErrZeroCraftJobTime          = errors.New("zero craft job timestamp")
	ErrInvalidCraftJobTime       = errors.New("invalid craft job timestamp")
)

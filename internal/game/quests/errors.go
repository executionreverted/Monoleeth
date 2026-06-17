package quests

import "errors"

var (
	ErrInvalidQuestType              = errors.New("invalid quest type")
	ErrInvalidQuestState             = errors.New("invalid quest state")
	ErrInvalidQuestStateTransition   = errors.New("invalid quest state transition")
	ErrEmptyQuestTextKey             = errors.New("empty quest text key")
	ErrInvalidObjectiveKind          = errors.New("invalid objective kind")
	ErrObjectivePayloadMismatch      = errors.New("objective payload mismatch")
	ErrEmptyObjectiveSchema          = errors.New("empty objective schema")
	ErrEmptyObjectiveList            = errors.New("empty objective list")
	ErrDuplicateObjectiveID          = errors.New("duplicate objective id")
	ErrEmptyObjectiveTarget          = errors.New("empty objective target")
	ErrInvalidObjectiveRequired      = errors.New("invalid objective required amount")
	ErrInvalidScanTargetKind         = errors.New("invalid scan target kind")
	ErrInvalidDeliveryTargetKind     = errors.New("invalid delivery target kind")
	ErrInvalidGeneratedPayload       = errors.New("invalid generated payload")
	ErrEmptyRewardPayload            = errors.New("empty reward payload")
	ErrInvalidRewardKind             = errors.New("invalid reward kind")
	ErrInvalidRewardAmount           = errors.New("invalid reward amount")
	ErrInvalidRewardCurrency         = errors.New("invalid reward currency")
	ErrInvalidRewardItem             = errors.New("invalid reward item")
	ErrInvalidRewardRole             = errors.New("invalid reward role")
	ErrInvalidRewardHook             = errors.New("invalid reward hook")
	ErrDuplicateRewardHook           = errors.New("duplicate reward hook")
	ErrQuestSourceMismatch           = errors.New("quest source definition mismatch")
	ErrInvalidQuestRequirement       = errors.New("invalid quest requirement")
	ErrEmptyQuestCatalog             = errors.New("empty quest catalog")
	ErrDuplicateQuestTemplate        = errors.New("duplicate quest template")
	ErrUnknownQuestTemplate          = errors.New("unknown quest template")
	ErrInvalidBoardGenerationInput   = errors.New("invalid board generation input")
	ErrInsufficientEligibleTemplates = errors.New("insufficient eligible quest templates")
	ErrZeroQuestTime                 = errors.New("zero quest timestamp")
	ErrInvalidQuestTime              = errors.New("invalid quest timestamp")
	ErrInvalidQuestProgress          = errors.New("invalid quest progress")
	ErrUnexpectedQuestProgress       = errors.New("unexpected quest progress")
	ErrInvalidQuestCompletion        = errors.New("invalid quest completion")
	ErrInvalidQuestClaim             = errors.New("invalid quest claim")
	ErrAcceptedQuestExpiresTooEarly  = errors.New("accepted quest expires before acceptance")
	ErrDuplicateQuestOffer           = errors.New("duplicate quest offer")
	ErrQuestOfferNotFound            = errors.New("quest offer not found")
	ErrQuestOfferExpired             = errors.New("quest offer expired")
	ErrQuestOfferOwnerMismatch       = errors.New("quest offer owner mismatch")
	ErrQuestOfferAlreadyAccepted     = errors.New("quest offer already accepted")
	ErrQuestRequirementsNotMet       = errors.New("quest requirements not met")
	ErrTooManyActiveQuests           = errors.New("too many active quests")
)

var (
	ErrInvalidQuestStateJump    = ErrInvalidQuestStateTransition
	ErrInvalidQuestTimestamp    = ErrInvalidQuestTime
	ErrZeroQuestTimestamp       = ErrZeroQuestTime
	ErrInvalidObjectiveSchema   = ErrEmptyObjectiveSchema
	ErrInvalidObjectiveTarget   = ErrEmptyObjectiveTarget
	ErrInvalidObjectiveAmount   = ErrInvalidObjectiveRequired
	ErrInvalidObjectiveProgress = ErrInvalidQuestProgress
	ErrInvalidRewardHookKind    = ErrInvalidRewardHook
)

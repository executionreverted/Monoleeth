package discovery

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrInvalidShareConfig                     = errors.New("invalid planet intel share config")
	ErrInvalidPlanetIntelShare                = errors.New("invalid planet intel share")
	ErrPlanetIntelShareReferenceConflict      = errors.New("planet intel share reference conflict")
	ErrPlanetIntelShareRequiresSourceIntel    = errors.New("planet intel share requires source intel")
	ErrPlanetIntelShareSourceIntelInvalidated = errors.New("planet intel share source intel invalidated")
	ErrPlanetIntelShareQuotaReached           = errors.New("planet intel share quota reached")
	ErrInvalidShareQuota                      = errors.New("invalid planet intel share quota")
)

// PlanetIntelShareReference identifies one domain share attempt. It is stable
// across retries and distinct from transport request ids.
type PlanetIntelShareReference string

// PlanetIntelShareEventType names local share notification/event records.
type PlanetIntelShareEventType string

const (
	PlanetIntelShareEventShared PlanetIntelShareEventType = "intel.shared"
)

// SharePlanetIntelInput is the service command for sharing one known planet
// intel record from one player to another.
type SharePlanetIntelInput struct {
	FromPlayerID   foundation.PlayerID       `json:"from_player_id"`
	ToPlayerID     foundation.PlayerID       `json:"to_player_id"`
	PlanetID       foundation.PlanetID       `json:"planet_id"`
	ShareReference PlanetIntelShareReference `json:"share_reference"`
}

// SharePlanetIntelResult reports the personal intel state after a share.
type SharePlanetIntelResult struct {
	SourceIntel     PlayerPlanetIntel `json:"source_intel"`
	ReceiverIntel   PlayerPlanetIntel `json:"receiver_intel"`
	Shared          bool              `json:"shared"`
	ReceiverUpdated bool              `json:"receiver_updated"`
	Duplicate       bool              `json:"duplicate,omitempty"`
}

// PlanetIntelShareEventRecord is a local outbox-shaped notification skeleton.
// It intentionally excludes coordinates and other planet details beyond the
// planet id being shared.
type PlanetIntelShareEventRecord struct {
	EventID        foundation.EventID        `json:"event_id"`
	Type           PlanetIntelShareEventType `json:"type"`
	FromPlayerID   foundation.PlayerID       `json:"from_player_id"`
	ToPlayerID     foundation.PlayerID       `json:"to_player_id"`
	PlanetID       foundation.PlanetID       `json:"planet_id"`
	ShareReference PlanetIntelShareReference `json:"share_reference"`
	CreatedAt      time.Time                 `json:"created_at"`
}

// ShareQuotaCheckInput asks an anti-spam boundary whether a share can proceed.
type ShareQuotaCheckInput struct {
	FromPlayerID   foundation.PlayerID       `json:"from_player_id"`
	ToPlayerID     foundation.PlayerID       `json:"to_player_id"`
	PlanetID       foundation.PlanetID       `json:"planet_id"`
	ShareReference PlanetIntelShareReference `json:"share_reference"`
	CheckedAt      time.Time                 `json:"checked_at"`
}

// ShareQuotaCheckResult reports whether the share quota permits this share.
type ShareQuotaCheckResult struct {
	Allowed bool `json:"allowed"`
}

// ShareQuotaConsumeInput records quota consumption after quota has been
// accepted and before receiver intel is mutated.
type ShareQuotaConsumeInput struct {
	FromPlayerID   foundation.PlayerID       `json:"from_player_id"`
	ToPlayerID     foundation.PlayerID       `json:"to_player_id"`
	PlanetID       foundation.PlanetID       `json:"planet_id"`
	ShareReference PlanetIntelShareReference `json:"share_reference"`
	ConsumedAt     time.Time                 `json:"consumed_at"`
}

// ShareQuotaConsumeResult reports duplicate-safe quota handling at the quota
// boundary.
type ShareQuotaConsumeResult struct {
	Consumed  bool `json:"consumed"`
	Duplicate bool `json:"duplicate,omitempty"`
}

type ShareQuotaProvider interface {
	ShareQuotaAvailable(input ShareQuotaCheckInput) (ShareQuotaCheckResult, error)
}

type ShareQuotaConsumer interface {
	ConsumeShareQuota(input ShareQuotaConsumeInput) (ShareQuotaConsumeResult, error)
}

// ShareServiceConfig wires share boundaries without depending on a concrete
// account, social, notification, or rate-limit service.
type ShareServiceConfig struct {
	Store         *InMemoryStore
	Clock         foundation.Clock
	QuotaProvider ShareQuotaProvider
	QuotaConsumer ShareQuotaConsumer
}

// ShareService owns local MVP planet intel sharing idempotency and events.
type ShareService struct {
	mu sync.Mutex

	store         *InMemoryStore
	clock         foundation.Clock
	quotaProvider ShareQuotaProvider
	quotaConsumer ShareQuotaConsumer

	shares map[PlanetIntelShareReference]shareRecord
	events []PlanetIntelShareEventRecord
}

type shareRecord struct {
	input  SharePlanetIntelInput
	result SharePlanetIntelResult
}

// NewShareService returns a planet intel share service backed by InMemoryStore.
func NewShareService(config ShareServiceConfig) (*ShareService, error) {
	normalized, err := normalizeShareConfig(config)
	if err != nil {
		return nil, err
	}
	return &ShareService{
		store:         normalized.Store,
		clock:         normalized.Clock,
		quotaProvider: normalized.QuotaProvider,
		quotaConsumer: normalized.QuotaConsumer,
		shares:        make(map[PlanetIntelShareReference]shareRecord),
	}, nil
}

// SharePlanetIntel validates source knowledge and share quota before writing
// receiver fog memory and appending one notification/event record.
func (service *ShareService) SharePlanetIntel(input SharePlanetIntelInput) (SharePlanetIntelResult, error) {
	if err := input.Validate(); err != nil {
		return SharePlanetIntelResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if record, ok := service.shares[input.ShareReference]; ok {
		if !shareRecordMatchesInput(record, input) {
			return SharePlanetIntelResult{}, ErrPlanetIntelShareReferenceConflict
		}
		duplicate := cloneSharePlanetIntelResult(record.result)
		duplicate.Duplicate = true
		duplicate.ReceiverUpdated = false
		return duplicate, nil
	}

	sourceIntel, err := service.sourceIntel(input)
	if err != nil {
		return SharePlanetIntelResult{}, err
	}

	now := service.clock.Now().UTC()
	if err := service.validateQuota(input, now); err != nil {
		return SharePlanetIntelResult{}, err
	}
	if err := service.consumeQuota(input, now); err != nil {
		return SharePlanetIntelResult{}, err
	}

	receiverIntel := sharedReceiverIntel(sourceIntel, input)
	storedReceiverIntel, receiverUpdated, err := service.store.UpsertPlayerPlanetIntel(receiverIntel)
	if err != nil {
		return SharePlanetIntelResult{}, err
	}

	event := newPlanetIntelShareEvent(input, now)
	service.events = append(service.events, event)

	result := SharePlanetIntelResult{
		SourceIntel:     sourceIntel,
		ReceiverIntel:   storedReceiverIntel,
		Shared:          true,
		ReceiverUpdated: receiverUpdated,
	}
	service.shares[input.ShareReference] = shareRecord{input: input, result: cloneSharePlanetIntelResult(result)}
	return result, nil
}

// Events returns share notification/event records in append order.
func (service *ShareService) Events() []PlanetIntelShareEventRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	events := make([]PlanetIntelShareEventRecord, len(service.events))
	copy(events, service.events)
	return events
}

// Validate reports whether reference is a non-empty server-generated share reference.
func (ref PlanetIntelShareReference) Validate() error {
	if !validDiscoveryToken(string(ref)) {
		return fmt.Errorf("share_reference %q: %w", ref, ErrInvalidPlanetIntelShare)
	}
	return nil
}

// Validate reports whether input identifies one server-owned share command.
func (input SharePlanetIntelInput) Validate() error {
	if err := input.FromPlayerID.Validate(); err != nil {
		return fmt.Errorf("from_player_id: %w", err)
	}
	if err := input.ToPlayerID.Validate(); err != nil {
		return fmt.Errorf("to_player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ShareReference.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether input can be handed to a quota provider.
func (input ShareQuotaCheckInput) Validate() error {
	if err := input.FromPlayerID.Validate(); err != nil {
		return fmt.Errorf("from_player_id: %w", err)
	}
	if err := input.ToPlayerID.Validate(); err != nil {
		return fmt.Errorf("to_player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ShareReference.Validate(); err != nil {
		return err
	}
	if input.CheckedAt.IsZero() {
		return fmt.Errorf("checked_at: %w", ErrInvalidShareQuota)
	}
	return nil
}

// Validate reports whether input can be handed to a quota consumer.
func (input ShareQuotaConsumeInput) Validate() error {
	if err := input.FromPlayerID.Validate(); err != nil {
		return fmt.Errorf("from_player_id: %w", err)
	}
	if err := input.ToPlayerID.Validate(); err != nil {
		return fmt.Errorf("to_player_id: %w", err)
	}
	if err := input.PlanetID.Validate(); err != nil {
		return fmt.Errorf("planet_id: %w", err)
	}
	if err := input.ShareReference.Validate(); err != nil {
		return err
	}
	if input.ConsumedAt.IsZero() {
		return fmt.Errorf("consumed_at: %w", ErrInvalidShareQuota)
	}
	return nil
}

func normalizeShareConfig(config ShareServiceConfig) (ShareServiceConfig, error) {
	if config.Store == nil {
		config.Store = NewInMemoryStore()
	}
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.QuotaProvider == nil {
		return ShareServiceConfig{}, fmt.Errorf("quota_provider: %w", ErrInvalidShareConfig)
	}
	if config.QuotaConsumer == nil {
		return ShareServiceConfig{}, fmt.Errorf("quota_consumer: %w", ErrInvalidShareConfig)
	}
	return config, nil
}

func (service *ShareService) sourceIntel(input SharePlanetIntelInput) (PlayerPlanetIntel, error) {
	sourceIntel, ok, err := service.store.PlayerPlanetIntel(input.FromPlayerID, input.PlanetID)
	if err != nil {
		return PlayerPlanetIntel{}, err
	}
	if !ok {
		return PlayerPlanetIntel{}, fmt.Errorf(
			"player %q planet %q: %w",
			input.FromPlayerID,
			input.PlanetID,
			ErrPlanetIntelShareRequiresSourceIntel,
		)
	}
	if sourceIntel.State == IntelStateInvalidated {
		return PlayerPlanetIntel{}, fmt.Errorf(
			"player %q planet %q: %w",
			input.FromPlayerID,
			input.PlanetID,
			ErrPlanetIntelShareSourceIntelInvalidated,
		)
	}
	return sourceIntel, nil
}

func (service *ShareService) validateQuota(input SharePlanetIntelInput, now time.Time) error {
	quotaInput := ShareQuotaCheckInput{
		FromPlayerID:   input.FromPlayerID,
		ToPlayerID:     input.ToPlayerID,
		PlanetID:       input.PlanetID,
		ShareReference: input.ShareReference,
		CheckedAt:      now,
	}
	if err := quotaInput.Validate(); err != nil {
		return err
	}
	result, err := service.quotaProvider.ShareQuotaAvailable(quotaInput)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return ErrPlanetIntelShareQuotaReached
	}
	return nil
}

func (service *ShareService) consumeQuota(input SharePlanetIntelInput, now time.Time) error {
	consumeInput := ShareQuotaConsumeInput{
		FromPlayerID:   input.FromPlayerID,
		ToPlayerID:     input.ToPlayerID,
		PlanetID:       input.PlanetID,
		ShareReference: input.ShareReference,
		ConsumedAt:     now,
	}
	if err := consumeInput.Validate(); err != nil {
		return err
	}
	result, err := service.quotaConsumer.ConsumeShareQuota(consumeInput)
	if err != nil {
		return err
	}
	if !result.Consumed && !result.Duplicate {
		return ErrPlanetIntelShareQuotaReached
	}
	return nil
}

func sharedReceiverIntel(sourceIntel PlayerPlanetIntel, input SharePlanetIntelInput) PlayerPlanetIntel {
	receiverIntel := clonePlayerPlanetIntel(sourceIntel)
	receiverIntel.PlayerID = input.ToPlayerID
	receiverIntel.SourceType = IntelSourceShareReceived
	receiverIntel.SourceReference = string(input.ShareReference)
	return receiverIntel
}

func shareRecordMatchesInput(record shareRecord, input SharePlanetIntelInput) bool {
	return record.input.FromPlayerID == input.FromPlayerID &&
		record.input.ToPlayerID == input.ToPlayerID &&
		record.input.PlanetID == input.PlanetID &&
		record.input.ShareReference == input.ShareReference
}

func newPlanetIntelShareEvent(input SharePlanetIntelInput, createdAt time.Time) PlanetIntelShareEventRecord {
	digest := scannerDigest(
		"planet_intel_share_event",
		string(PlanetIntelShareEventShared),
		input.FromPlayerID.String(),
		input.ToPlayerID.String(),
		input.PlanetID.String(),
		string(input.ShareReference),
	)
	return PlanetIntelShareEventRecord{
		EventID:        foundation.EventID("event_" + fmt.Sprintf("%x", digest[:10])),
		Type:           PlanetIntelShareEventShared,
		FromPlayerID:   input.FromPlayerID,
		ToPlayerID:     input.ToPlayerID,
		PlanetID:       input.PlanetID,
		ShareReference: input.ShareReference,
		CreatedAt:      createdAt.UTC(),
	}
}

func cloneSharePlanetIntelResult(result SharePlanetIntelResult) SharePlanetIntelResult {
	result.SourceIntel = clonePlayerPlanetIntel(result.SourceIntel)
	result.ReceiverIntel = clonePlayerPlanetIntel(result.ReceiverIntel)
	return result
}

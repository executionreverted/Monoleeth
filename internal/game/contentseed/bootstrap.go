package contentseed

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
)

const (
	defaultSeedActor              = "contentseed.bootstrap"
	defaultSeedVersionIDNamespace = "gameproject.contentseed.published_version"
)

var (
	ErrNilSeedContext       = errors.New("nil content seed context")
	ErrNilSeedStore         = errors.New("nil content seed store")
	ErrInvalidSeedVersionID = errors.New("invalid seed version id")
)

type PublishedSnapshotInput = contentdb.PublishedSnapshotInput

type SeedStore interface {
	HasAnyContent(ctx context.Context) (bool, error)
	UpsertDraftRows(ctx context.Context, contentType content.ContentType, draftVersion string, rows []content.SnapshotRow, updatedBy string) error
	InsertPublishedSnapshot(ctx context.Context, input PublishedSnapshotInput) error
}

var _ SeedStore = (*contentdb.Store)(nil)

type SeedOptions struct {
	VersionID            string
	DraftVersionID       string
	IdempotencyKey       string
	Actor                string
	CreatedBy            string
	PublishedBy          string
	PublishedAt          time.Time
	Notes                string
	BalanceTag           string
	ValidationReportJSON json.RawMessage
}

type SeedResult struct {
	Seeded   bool
	Version  string
	RowCount int
}

func EnsurePublishedSeed(ctx context.Context, store SeedStore, snapshot content.Snapshot, options SeedOptions) (SeedResult, error) {
	if ctx == nil {
		return SeedResult{}, ErrNilSeedContext
	}
	if isNilSeedStore(store) {
		return SeedResult{}, ErrNilSeedStore
	}

	hasContent, err := store.HasAnyContent(ctx)
	if err != nil {
		return SeedResult{}, fmt.Errorf("check content seed state: %w", err)
	}
	if hasContent {
		return SeedResult{}, nil
	}
	if err := snapshot.Validate(); err != nil {
		return SeedResult{}, fmt.Errorf("validate content seed snapshot: %w", err)
	}

	options, err = normalizeSeedOptions(snapshot, options)
	if err != nil {
		return SeedResult{}, err
	}

	rowCount := 0
	for _, group := range snapshot.Groups() {
		rowCount += len(group.Rows)
		if err := store.UpsertDraftRows(ctx, group.Type, options.DraftVersionID, group.Rows, options.Actor); err != nil {
			return SeedResult{}, fmt.Errorf("upsert %s seed drafts: %w", group.Type, err)
		}
	}

	input := PublishedSnapshotInput{
		ID:                   options.VersionID,
		Version:              snapshot.Version,
		Snapshot:             snapshot,
		ValidationReportJSON: options.ValidationReportJSON,
		IdempotencyKey:       options.IdempotencyKey,
		Notes:                options.Notes,
		BalanceTag:           options.BalanceTag,
		CreatedBy:            options.CreatedBy,
		PublishedBy:          options.PublishedBy,
		PublishedAt:          options.PublishedAt,
	}
	if err := store.InsertPublishedSnapshot(ctx, input); err != nil {
		return SeedResult{}, fmt.Errorf("insert published seed snapshot: %w", err)
	}

	return SeedResult{
		Seeded:   true,
		Version:  snapshot.Version,
		RowCount: rowCount,
	}, nil
}

func normalizeSeedOptions(snapshot content.Snapshot, options SeedOptions) (SeedOptions, error) {
	if options.VersionID == "" {
		options.VersionID = deterministicSeedVersionID(snapshot.Version)
	}
	if err := validateSeedUUID("seed version id", options.VersionID); err != nil {
		return SeedOptions{}, err
	}
	if options.DraftVersionID != "" {
		if err := validateSeedUUID("seed draft version id", options.DraftVersionID); err != nil {
			return SeedOptions{}, err
		}
	}
	if options.Actor == "" {
		options.Actor = defaultSeedActor
	}
	if options.CreatedBy == "" {
		options.CreatedBy = options.Actor
	}
	if options.PublishedBy == "" {
		options.PublishedBy = options.Actor
	}
	if options.IdempotencyKey == "" {
		options.IdempotencyKey = "contentseed:" + snapshot.Version
	}
	if options.Notes == "" {
		options.Notes = content.DefaultStarterBalanceProfileNote
	}
	if options.BalanceTag == "" {
		options.BalanceTag = content.DefaultStarterBalanceProfileID
	}
	if len(options.ValidationReportJSON) == 0 {
		report, err := json.Marshal(map[string]string{
			"snapshot_version": snapshot.Version,
			"source":           "contentseed",
		})
		if err != nil {
			return SeedOptions{}, err
		}
		options.ValidationReportJSON = report
	}
	return options, nil
}

func deterministicSeedVersionID(version string) string {
	sum := sha256.Sum256([]byte(defaultSeedVersionIDNamespace + "\x00" + version))
	id := append([]byte(nil), sum[:16]...)
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

func validateSeedUUID(kind string, value string) error {
	if len(value) != 36 {
		return fmt.Errorf("%s %q: %w", kind, value, ErrInvalidSeedVersionID)
	}
	for index := 0; index < len(value); index++ {
		switch index {
		case 8, 13, 18, 23:
			if value[index] != '-' {
				return fmt.Errorf("%s %q: %w", kind, value, ErrInvalidSeedVersionID)
			}
		default:
			if !isHexByte(value[index]) {
				return fmt.Errorf("%s %q: %w", kind, value, ErrInvalidSeedVersionID)
			}
		}
	}
	return nil
}

func isHexByte(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'a' && value <= 'f') ||
		(value >= 'A' && value <= 'F')
}

func isNilSeedStore(store SeedStore) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

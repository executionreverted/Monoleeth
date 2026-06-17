package catalog

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEmptyDefinitionID reports a missing or blank static definition id.
var ErrEmptyDefinitionID = errors.New("empty catalog definition id")

// ErrEmptyVersion reports a missing or blank catalog version.
var ErrEmptyVersion = errors.New("empty catalog version")

// DefinitionID identifies one static gameplay definition inside a catalog.
type DefinitionID string

// Version identifies the static catalog version that produced a definition.
type Version string

// VersionedDefinition records the source definition and catalog version used to
// create long-lived state such as craft jobs, quest offers, loot rolls, or lots.
type VersionedDefinition struct {
	DefinitionID DefinitionID `json:"definition_id"`
	Version      Version      `json:"catalog_version"`
}

// ParseDefinitionID validates value and returns a DefinitionID.
func ParseDefinitionID(value string) (DefinitionID, error) {
	return parseDefinitionID("definition", value)
}

// ParseVersion validates value and returns a Version.
func ParseVersion(value string) (Version, error) {
	return parseVersion("catalog", value)
}

// NewVersionedDefinition validates and returns a source definition reference.
func NewVersionedDefinition(definitionID DefinitionID, version Version) (VersionedDefinition, error) {
	source := VersionedDefinition{
		DefinitionID: definitionID,
		Version:      version,
	}
	if err := source.Validate(); err != nil {
		return VersionedDefinition{}, err
	}
	return source, nil
}

// NewVersionedDefinitionFromStrings validates strings and returns a source definition reference.
func NewVersionedDefinitionFromStrings(definitionID string, version string) (VersionedDefinition, error) {
	return newVersionedDefinition("definition", definitionID, version)
}

// NewRecipeSource validates and returns the recipe definition source used by durable state.
func NewRecipeSource(recipeID string, version string) (VersionedDefinition, error) {
	return newVersionedDefinition("recipe", recipeID, version)
}

// NewQuestSource validates and returns the quest definition source used by durable state.
func NewQuestSource(questID string, version string) (VersionedDefinition, error) {
	return newVersionedDefinition("quest", questID, version)
}

// NewLootTableSource validates and returns the loot table source used by durable state.
func NewLootTableSource(lootTableID string, version string) (VersionedDefinition, error) {
	return newVersionedDefinition("loot table", lootTableID, version)
}

// NewAuctionLotSource validates and returns the auction lot source used by durable state.
func NewAuctionLotSource(lotID string, version string) (VersionedDefinition, error) {
	return newVersionedDefinition("auction lot", lotID, version)
}

// String returns the stable definition id representation.
func (id DefinitionID) String() string { return string(id) }

// Validate reports whether id is non-blank.
func (id DefinitionID) Validate() error { return validateDefinitionID("definition", string(id)) }

// IsZero reports whether id is the zero value.
func (id DefinitionID) IsZero() bool { return id == "" }

// String returns the stable catalog version representation.
func (version Version) String() string { return string(version) }

// Validate reports whether version is non-blank.
func (version Version) Validate() error { return validateVersion("catalog", string(version)) }

// IsZero reports whether version is the zero value.
func (version Version) IsZero() bool { return version == "" }

// Validate reports whether source has both a definition id and catalog version.
func (source VersionedDefinition) Validate() error {
	if err := source.DefinitionID.Validate(); err != nil {
		return err
	}
	if err := source.Version.Validate(); err != nil {
		return err
	}
	return nil
}

// IsZero reports whether source is the zero value.
func (source VersionedDefinition) IsZero() bool {
	return source.DefinitionID.IsZero() && source.Version.IsZero()
}

func newVersionedDefinition(kind, definitionID, version string) (VersionedDefinition, error) {
	parsedID, err := parseDefinitionID(kind, definitionID)
	if err != nil {
		return VersionedDefinition{}, err
	}
	parsedVersion, err := parseVersion(kind, version)
	if err != nil {
		return VersionedDefinition{}, err
	}
	return VersionedDefinition{
		DefinitionID: parsedID,
		Version:      parsedVersion,
	}, nil
}

func parseDefinitionID(kind, value string) (DefinitionID, error) {
	if err := validateDefinitionID(kind, value); err != nil {
		return "", err
	}
	return DefinitionID(value), nil
}

func parseVersion(kind, value string) (Version, error) {
	if err := validateVersion(kind, value); err != nil {
		return "", err
	}
	return Version(value), nil
}

func validateDefinitionID(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s id: %w", kind, ErrEmptyDefinitionID)
	}
	return nil
}

func validateVersion(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s version: %w", kind, ErrEmptyVersion)
	}
	return nil
}

package catalog

import (
	"encoding/json"
	"errors"
	"testing"
)

type sourceHelperCase struct {
	name    string
	id      string
	version string
	helper  func(string, string) (VersionedDefinition, error)
}

func TestSourceHelpersPreserveDefinitionIDsAndVersions(t *testing.T) {
	for _, tc := range sourceHelperCases() {
		t.Run(tc.name, func(t *testing.T) {
			source, err := tc.helper(tc.id, tc.version)
			if err != nil {
				t.Fatalf("helper returned error: %v", err)
			}

			if got := source.DefinitionID.String(); got != tc.id {
				t.Fatalf("DefinitionID.String() = %q, want %q", got, tc.id)
			}
			if got := source.Version.String(); got != tc.version {
				t.Fatalf("Version.String() = %q, want %q", got, tc.version)
			}
			if err := source.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if source.IsZero() {
				t.Fatal("IsZero() = true, want false")
			}
		})
	}
}

func TestCatalogPrimitivesRejectBlankDefinitionIDsAndVersions(t *testing.T) {
	blankValues := []string{"", " ", "\t"}

	for _, value := range blankValues {
		if _, err := ParseDefinitionID(value); !errors.Is(err, ErrEmptyDefinitionID) {
			t.Fatalf("ParseDefinitionID(%q) error = %v, want ErrEmptyDefinitionID", value, err)
		}
		if _, err := ParseVersion(value); !errors.Is(err, ErrEmptyVersion) {
			t.Fatalf("ParseVersion(%q) error = %v, want ErrEmptyVersion", value, err)
		}
	}

	if _, err := NewVersionedDefinition("", Version("recipe_catalog_v1")); !errors.Is(err, ErrEmptyDefinitionID) {
		t.Fatalf("NewVersionedDefinition blank id error = %v, want ErrEmptyDefinitionID", err)
	}
	if _, err := NewVersionedDefinition(DefinitionID("laser_beta_t2"), ""); !errors.Is(err, ErrEmptyVersion) {
		t.Fatalf("NewVersionedDefinition blank version error = %v, want ErrEmptyVersion", err)
	}
	if err := (VersionedDefinition{}).Validate(); !errors.Is(err, ErrEmptyDefinitionID) {
		t.Fatalf("zero VersionedDefinition Validate() = %v, want ErrEmptyDefinitionID", err)
	}
	if err := (VersionedDefinition{DefinitionID: "laser_beta_t2"}).Validate(); !errors.Is(err, ErrEmptyVersion) {
		t.Fatalf("blank source version Validate() = %v, want ErrEmptyVersion", err)
	}
}

func TestSourceHelpersRejectBlankDefinitionIDsAndVersions(t *testing.T) {
	for _, tc := range sourceHelperCases() {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.helper("", tc.version); !errors.Is(err, ErrEmptyDefinitionID) {
				t.Fatalf("blank id error = %v, want ErrEmptyDefinitionID", err)
			}
			if _, err := tc.helper(tc.id, ""); !errors.Is(err, ErrEmptyVersion) {
				t.Fatalf("blank version error = %v, want ErrEmptyVersion", err)
			}
		})
	}
}

func TestCatalogStringAndJSONBehaviorIsStable(t *testing.T) {
	definitionID, err := ParseDefinitionID("laser_beta_t2")
	if err != nil {
		t.Fatalf("ParseDefinitionID valid value: %v", err)
	}
	version, err := ParseVersion("recipe_catalog_v1")
	if err != nil {
		t.Fatalf("ParseVersion valid value: %v", err)
	}
	source, err := NewVersionedDefinition(definitionID, version)
	if err != nil {
		t.Fatalf("NewVersionedDefinition valid values: %v", err)
	}

	if got := definitionID.String(); got != "laser_beta_t2" {
		t.Fatalf("DefinitionID.String() = %q, want %q", got, "laser_beta_t2")
	}
	if got := version.String(); got != "recipe_catalog_v1" {
		t.Fatalf("Version.String() = %q, want %q", got, "recipe_catalog_v1")
	}

	definitionPayload, err := json.Marshal(definitionID)
	if err != nil {
		t.Fatalf("json marshal definition id: %v", err)
	}
	if got := string(definitionPayload); got != `"laser_beta_t2"` {
		t.Fatalf("definition id JSON = %s, want %s", got, `"laser_beta_t2"`)
	}

	versionPayload, err := json.Marshal(version)
	if err != nil {
		t.Fatalf("json marshal version: %v", err)
	}
	if got := string(versionPayload); got != `"recipe_catalog_v1"` {
		t.Fatalf("version JSON = %s, want %s", got, `"recipe_catalog_v1"`)
	}

	sourcePayload, err := json.Marshal(source)
	if err != nil {
		t.Fatalf("json marshal source: %v", err)
	}
	want := `{"definition_id":"laser_beta_t2","catalog_version":"recipe_catalog_v1"}`
	if got := string(sourcePayload); got != want {
		t.Fatalf("source JSON = %s, want %s", got, want)
	}
}

func sourceHelperCases() []sourceHelperCase {
	return []sourceHelperCase{
		{
			name:    "recipe",
			id:      "laser_beta_t2",
			version: "recipe_catalog_v1",
			helper:  NewRecipeSource,
		},
		{
			name:    "quest",
			id:      "frontier_kill_01",
			version: "quest_catalog_v1",
			helper:  NewQuestSource,
		},
		{
			name:    "loot table",
			id:      "starter_pirate_drop",
			version: "loot_table_v1",
			helper:  NewLootTableSource,
		},
		{
			name:    "auction lot",
			id:      "weekly_x_core_bundle",
			version: "auction_catalog_v1",
			helper:  NewAuctionLotSource,
		},
	}
}

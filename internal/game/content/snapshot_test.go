package content

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestSnapshotValidatesMinimalRows(t *testing.T) {
	snapshot := Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []SnapshotRow{{
			ContentID:   "raw_ore",
			Enabled:     true,
			DisplayJSON: json.RawMessage(`{"name":"Raw Ore"}`),
			DataJSON:    json.RawMessage(`{"item_type":"stackable"}`),
		}},
		CraftRecipes: []SnapshotRow{{
			ContentID: "refined_alloy_batch",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"craft_duration_ms":300000}`),
		}},
	}

	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestSnapshotRejectsEmptyVersion(t *testing.T) {
	err := (Snapshot{}).Validate()

	if !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("Validate() error = %v, want ErrEmptyID", err)
	}
}

func TestSnapshotRejectsDuplicateIDsWithinType(t *testing.T) {
	snapshot := Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []SnapshotRow{
			{ContentID: "raw_ore", Enabled: true, DataJSON: json.RawMessage(`{}`)},
			{ContentID: "raw_ore", Enabled: true, DataJSON: json.RawMessage(`{}`)},
		},
	}

	err := snapshot.Validate()

	if !errors.Is(err, ErrDuplicateContentID) {
		t.Fatalf("Validate() error = %v, want ErrDuplicateContentID", err)
	}
}

func TestSnapshotRejectsInvalidJSON(t *testing.T) {
	snapshot := Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []SnapshotRow{{
			ContentID: "raw_ore",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"broken"`),
		}},
	}

	err := snapshot.Validate()

	if !errors.Is(err, ErrInvalidContentJSON) {
		t.Fatalf("Validate() error = %v, want ErrInvalidContentJSON", err)
	}
}

func TestSnapshotRejectsNonObjectDataJSON(t *testing.T) {
	snapshot := Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []SnapshotRow{{
			ContentID: "raw_ore",
			Enabled:   true,
			DataJSON:  json.RawMessage(`[]`),
		}},
	}

	err := snapshot.Validate()

	if !errors.Is(err, ErrInvalidContentJSON) {
		t.Fatalf("Validate() error = %v, want ErrInvalidContentJSON", err)
	}
}

func TestSnapshotRejectsForbiddenDSLFields(t *testing.T) {
	snapshot := Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []SnapshotRow{{
			ContentID: "raw_ore",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"rules":[{"expression":"amount * 2"}]}`),
		}},
	}

	err := snapshot.Validate()

	if !errors.Is(err, ErrForbiddenContentField) {
		t.Fatalf("Validate() error = %v, want ErrForbiddenContentField", err)
	}
}

func TestSnapshotRejectsColonAndControlIDs(t *testing.T) {
	for _, contentID := range []ContentID{"item:raw_ore", "raw_ore\n"} {
		snapshot := Snapshot{
			Version: "content_mvp_seed_v1",
			Items: []SnapshotRow{{
				ContentID: contentID,
				Enabled:   true,
				DataJSON:  json.RawMessage(`{}`),
			}},
		}

		err := snapshot.Validate()

		if !errors.Is(err, foundation.ErrInvalidID) {
			t.Fatalf("Validate(%q) error = %v, want ErrInvalidID", contentID, err)
		}
	}
}

func TestSnapshotAllowsSameIDAcrossDifferentTypes(t *testing.T) {
	snapshot := Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []SnapshotRow{{
			ContentID: "starter",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{}`),
		}},
		Modules: []SnapshotRow{{
			ContentID: "starter",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{}`),
		}},
	}

	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestSnapshotMarshalUsesStableGroupKeys(t *testing.T) {
	encoded, err := json.Marshal(Snapshot{Version: "content_mvp_seed_v1"})
	if err != nil {
		t.Fatalf("Marshal() error = %v, want nil", err)
	}
	body := string(encoded)
	for _, key := range []string{`"items"`, `"modules"`, `"craft_recipes"`, `"production_buildings"`, `"quest_reward_tables"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("encoded snapshot missing %s: %s", key, body)
		}
	}
}

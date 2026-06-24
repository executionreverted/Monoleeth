package contentdb

import (
	"encoding/json"
	"errors"
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
)

var ErrContentRowIDMismatch = errors.New("content row id mismatch")

func decodeSnapshotRow(contentType content.ContentType, row content.SnapshotRow, out any) error {
	if err := json.Unmarshal(row.DataJSON, out); err != nil {
		return fmt.Errorf("%s %q data_json: %w", contentType, row.ContentID, err)
	}
	return nil
}

func requireRowID(contentType content.ContentType, row content.SnapshotRow, decodedID string) error {
	if string(row.ContentID) != decodedID {
		return fmt.Errorf("%s row %q decoded id %q: %w", contentType, row.ContentID, decodedID, ErrContentRowIDMismatch)
	}
	return nil
}

func publishedVersion(snapshot content.Snapshot) catalog.Version {
	return catalog.Version(snapshot.Version)
}

func forceSourceVersion(source catalog.VersionedDefinition, version catalog.Version) catalog.VersionedDefinition {
	source.Version = version
	return source
}

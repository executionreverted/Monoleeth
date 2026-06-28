package contentdb

import (
	"errors"
	"testing"

	"gameproject/internal/game/content"
)

func TestContentTableNameAllowlist(t *testing.T) {
	tests := map[content.ContentType]string{
		content.ContentTypeCraftRecipe: "content_craft_recipes",
		content.ContentTypeMap:         "content_maps",
		content.ContentTypeMapPortal:   "content_map_portals",
	}
	for contentType, want := range tests {
		table, err := ContentTableName(contentType)
		if err != nil {
			t.Fatalf("ContentTableName(%s) error = %v, want nil", contentType, err)
		}
		if table != want {
			t.Fatalf("ContentTableName(%s) = %q, want %s", contentType, table, want)
		}
	}
}

func TestContentTableNameRejectsUnknown(t *testing.T) {
	_, err := ContentTableName(content.ContentType("craft_recipe;drop table content_versions"))

	if !errors.Is(err, ErrUnknownContentType) {
		t.Fatalf("ContentTableName(unknown) error = %v, want ErrUnknownContentType", err)
	}
}

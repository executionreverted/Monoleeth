package contentdb

import (
	"errors"
	"testing"

	"gameproject/internal/game/content"
)

func TestContentTableNameAllowlist(t *testing.T) {
	table, err := ContentTableName(content.ContentTypeCraftRecipe)
	if err != nil {
		t.Fatalf("ContentTableName(craft) error = %v, want nil", err)
	}
	if table != "content_craft_recipes" {
		t.Fatalf("ContentTableName(craft) = %q, want content_craft_recipes", table)
	}
}

func TestContentTableNameRejectsUnknown(t *testing.T) {
	_, err := ContentTableName(content.ContentType("craft_recipe;drop table content_versions"))

	if !errors.Is(err, ErrUnknownContentType) {
		t.Fatalf("ContentTableName(unknown) error = %v, want ErrUnknownContentType", err)
	}
}

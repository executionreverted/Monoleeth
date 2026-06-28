package kalaazu

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParserParsesInsertColumnsAndRows(t *testing.T) {
	source := "INSERT INTO `demo` (`id`, `name`, `enabled`) VALUES (1, 'Alpha', true), (2, 'Beta', false);"

	rows, err := ParseDump("demo.sql", source)
	if err != nil {
		t.Fatalf("ParseDump() error = %v, want nil", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].Table != "demo" {
		t.Fatalf("table = %q, want demo", rows[0].Table)
	}
	if got := rows[0].Columns; strings.Join(got, ",") != "id,name,enabled" {
		t.Fatalf("columns = %v, want id,name,enabled", got)
	}
	if got, err := rows[0].Int("id"); err != nil || got != 1 {
		t.Fatalf("row[0].Int(id) = %d, %v; want 1,nil", got, err)
	}
	if got, err := rows[1].String("name"); err != nil || got != "Beta" {
		t.Fatalf("row[1].String(name) = %q, %v; want Beta,nil", got, err)
	}
	if got, err := rows[1].Bool("enabled"); err != nil || got {
		t.Fatalf("row[1].Bool(enabled) = %v, %v; want false,nil", got, err)
	}
}

func TestParserHandlesEscapedQuotedStringsAndNull(t *testing.T) {
	source := "INSERT INTO `items` (`id`, `description`, `flag`, `missing`) VALUES (9, 'Don\\'t panic\\nLine 2', false, NULL);"

	rows, err := ParseDump("items.sql", source)
	if err != nil {
		t.Fatalf("ParseDump() error = %v, want nil", err)
	}
	description, err := rows[0].String("description")
	if err != nil {
		t.Fatalf("String(description) error = %v, want nil", err)
	}
	if description != "Don't panic\nLine 2" {
		t.Fatalf("description = %q, want escaped text", description)
	}
	if value, ok := rows[0].Value("missing"); !ok || !value.Null {
		t.Fatalf("missing value = %+v ok=%v, want NULL", value, ok)
	}
}

func TestLoadDumpRowsLoadsCheckedInDumps(t *testing.T) {
	cases := []struct {
		path      string
		table     string
		rowCount  int
		firstKeys []string
	}{
		{path: "testdata/items.sql", table: "items", rowCount: 359, firstKeys: []string{"id", "name", "loot_id", "category"}},
		{path: "testdata/maps.sql", table: "maps", rowCount: 87, firstKeys: []string{"id", "name", "is_pvp", "is_starter", "limits"}},
		{path: "testdata/maps_npcs.sql", table: "maps_npcs", rowCount: 137, firstKeys: []string{"maps_id", "npcs_id", "amount"}},
		{path: "testdata/maps_portals.sql", table: "maps_portals", rowCount: 106, firstKeys: []string{"maps_id", "position", "target_position", "target_maps_id"}},
		{path: "testdata/npcs.sql", table: "npcs", rowCount: 80, firstKeys: []string{"id", "name", "health", "shield", "damage"}},
		{path: "testdata/ships.sql", table: "ships", rowCount: 13, firstKeys: []string{"id", "items_id", "health", "speed", "cargo"}},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rows, err := LoadDumpRows(os.DirFS("."), tc.path)
			if err != nil {
				t.Fatalf("LoadDumpRows(%s) error = %v, want nil", tc.path, err)
			}
			if len(rows) != tc.rowCount {
				t.Fatalf("rows len = %d, want %d", len(rows), tc.rowCount)
			}
			if rows[0].Table != tc.table {
				t.Fatalf("table = %q, want %q", rows[0].Table, tc.table)
			}
			for _, key := range tc.firstKeys {
				if _, ok := rows[0].Value(key); !ok {
					t.Fatalf("first row missing key %q in %+v", key, rows[0].Values)
				}
			}
		})
	}
}

func TestParserRejectsMalformedDump(t *testing.T) {
	_, err := ParseDump("bad.sql", "SELECT 1;")
	if !errors.Is(err, ErrUnsupportedDumpSQL) {
		t.Fatalf("ParseDump(unsupported) error = %v, want %v", err, ErrUnsupportedDumpSQL)
	}

	_, err = ParseDump("bad.sql", "INSERT INTO `demo` (`id`) VALUES (1, 2);")
	if !errors.Is(err, ErrMalformedDumpSQL) {
		t.Fatalf("ParseDump(malformed) error = %v, want %v", err, ErrMalformedDumpSQL)
	}
}

func TestLoadDumpRowsWrapsReadError(t *testing.T) {
	_, err := LoadDumpRows(fstest.MapFS{}, "missing.sql")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("LoadDumpRows(missing) error = %v, want fs.ErrNotExist", err)
	}
}

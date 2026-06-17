package game

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGameplayPackagesDoNotImportSymphony(t *testing.T) {
	const forbiddenImport = "internal/symphony"

	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, importSpec := range file.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return err
			}
			if isForbiddenImport(importPath, forbiddenImport) {
				t.Errorf("%s imports forbidden Symphony package %q", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isForbiddenImport(importPath, forbiddenImport string) bool {
	return importPath == forbiddenImport ||
		strings.HasPrefix(importPath, forbiddenImport+"/") ||
		strings.HasSuffix(importPath, "/"+forbiddenImport) ||
		strings.Contains(importPath, "/"+forbiddenImport+"/")
}

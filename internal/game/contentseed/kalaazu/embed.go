package kalaazu

import (
	"embed"
	"io/fs"
)

//go:embed testdata/*.sql
var seedFS embed.FS

// DefaultSeedFS returns the checked-in Kalaazu seed dump files embedded into the
// server binary. Runtime seed generation must not depend on the process cwd.
func DefaultSeedFS() fs.FS {
	return seedFS
}

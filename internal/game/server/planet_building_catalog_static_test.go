package server

import (
	"bytes"
	"os"
	"testing"
)

func TestPlanetBuildingHandlerUsesInjectedProductionCatalog(t *testing.T) {
	source, err := os.ReadFile("planet_building_handlers.go")
	if err != nil {
		t.Fatalf("ReadFile(planet_building_handlers.go) error = %v, want nil", err)
	}
	for _, forbidden := range [][]byte{
		[]byte("production.MustMVPCatalog()"),
		[]byte("production.MVPCatalog()"),
	} {
		if bytes.Contains(source, forbidden) {
			t.Fatalf("planet building handler contains %q, want injected runtime catalog", forbidden)
		}
	}
}

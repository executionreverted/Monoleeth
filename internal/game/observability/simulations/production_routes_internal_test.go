package simulations

import "testing"

func TestSimulationRoutePolicyCarriesInternalMapIdentity(t *testing.T) {
	policy := simulationRoutePolicy()
	if err := policy.Validate(); err != nil {
		t.Fatalf("simulationRoutePolicy().Validate() error = %v, want nil", err)
	}
	if policy.SourceMapID != simulationRouteSourceMapID {
		t.Fatalf("SourceMapID = %q, want %q", policy.SourceMapID, simulationRouteSourceMapID)
	}
	if policy.DestinationMapID != simulationRouteDestinationMapID {
		t.Fatalf("DestinationMapID = %q, want %q", policy.DestinationMapID, simulationRouteDestinationMapID)
	}
}

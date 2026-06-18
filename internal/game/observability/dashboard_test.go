package observability

import "testing"

func TestRequiredDashboardSpecsArePresentAndStable(t *testing.T) {
	specs := RequiredDashboardSpecs()
	wantKeys := []DashboardKey{
		DashboardCreditsFaucetSink,
		DashboardXCoreSupply,
		DashboardRawMaterialSupply,
		DashboardProcessedMaterialSupply,
		DashboardMarketAveragePrices,
		DashboardAuctionClearingPrices,
		DashboardRepairTotals,
		DashboardCraftFees,
		DashboardRouteLoss,
		DashboardPlanetProduction,
	}

	if len(specs) != len(wantKeys) {
		t.Fatalf("dashboard specs = %d, want %d", len(specs), len(wantKeys))
	}
	seen := map[DashboardKey]bool{}
	for i, spec := range specs {
		if spec.Key != wantKeys[i] {
			t.Fatalf("dashboard key[%d] = %q, want %q", i, spec.Key, wantKeys[i])
		}
		if spec.Name == "" {
			t.Fatalf("dashboard %q has blank name", spec.Key)
		}
		if len(spec.Sources) == 0 {
			t.Fatalf("dashboard %q has no sources", spec.Key)
		}
		if seen[spec.Key] {
			t.Fatalf("duplicate dashboard key %q", spec.Key)
		}
		seen[spec.Key] = true
	}
}

func TestRequiredDashboardSpecsAreCloneSafe(t *testing.T) {
	specs := RequiredDashboardSpecs()
	specs[0].Key = DashboardKey("mutated")
	specs[0].Sources[0] = "mutated"

	next := RequiredDashboardSpecs()
	if next[0].Key != DashboardCreditsFaucetSink {
		t.Fatalf("dashboard key mutated through returned slice: got %q", next[0].Key)
	}
	if next[0].Sources[0] == "mutated" {
		t.Fatal("dashboard source mutated through returned slice")
	}
}

func TestPriceDashboardSpecsUseQuantityAndCountSources(t *testing.T) {
	specs := RequiredDashboardSpecs()

	market := findDashboardSpec(t, specs, DashboardMarketAveragePrices)
	assertSources(t, market.Sources, []string{
		DashboardSourceMarketVolume,
		DashboardSourceMarketQuantity,
		DashboardSourceMarketSales,
	})

	auction := findDashboardSpec(t, specs, DashboardAuctionClearingPrices)
	assertSources(t, auction.Sources, []string{
		DashboardSourceAuctionClearingVolume,
		DashboardSourceAuctionClearingQuantity,
		DashboardSourceAuctionClears,
	})
}

func findDashboardSpec(t *testing.T, specs []DashboardSpec, key DashboardKey) DashboardSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Key == key {
			return spec
		}
	}
	t.Fatalf("missing dashboard spec %q", key)
	return DashboardSpec{}
}

func assertSources(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sources = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("source[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

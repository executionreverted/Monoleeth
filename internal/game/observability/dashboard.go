package observability

// DashboardKey is a stable identifier for one production-readiness dashboard.
type DashboardKey string

const (
	DashboardCreditsFaucetSink       DashboardKey = "credits_faucet_sink"
	DashboardXCoreSupply             DashboardKey = "x_core_supply"
	DashboardRawMaterialSupply       DashboardKey = "raw_material_supply"
	DashboardProcessedMaterialSupply DashboardKey = "processed_material_supply"
	DashboardMarketAveragePrices     DashboardKey = "market_average_prices"
	DashboardAuctionClearingPrices   DashboardKey = "auction_clearing_prices"
	DashboardRepairTotals            DashboardKey = "repair_totals"
	DashboardCraftFees               DashboardKey = "craft_fees"
	DashboardRouteLoss               DashboardKey = "route_loss"
	DashboardPlanetProduction        DashboardKey = "planet_production"
)

const (
	DashboardSourceCurrencyFaucets   = "economy_flow.currency_faucets"
	DashboardSourceCurrencySinks     = "economy_flow.currency_sinks"
	DashboardSourceItemFaucets       = "economy_flow.item_faucets"
	DashboardSourceItemSinks         = "economy_flow.item_sinks"
	DashboardSourceMarketVolume      = MetricMarketVolume
	DashboardSourceAuctionVolume     = MetricAuctionVolume
	DashboardSourcePlanetSettlements = MetricPlanetSettlements
	DashboardSourceRouteSettlements  = MetricRouteSettlements
)

// DashboardSpec names one stable dashboard and the local observability sources it needs.
type DashboardSpec struct {
	Key     DashboardKey `json:"key"`
	Name    string       `json:"name"`
	Sources []string     `json:"sources"`
}

var requiredDashboardSpecs = []DashboardSpec{
	{
		Key:  DashboardCreditsFaucetSink,
		Name: "Credits Faucet/Sink",
		Sources: []string{
			DashboardSourceCurrencyFaucets + ":credits",
			DashboardSourceCurrencySinks + ":credits",
		},
	},
	{
		Key:  DashboardXCoreSupply,
		Name: "X Core Supply",
		Sources: []string{
			DashboardSourceItemFaucets + ":x_core",
			DashboardSourceItemSinks + ":x_core",
		},
	},
	{
		Key:  DashboardRawMaterialSupply,
		Name: "Raw Material Supply",
		Sources: []string{
			DashboardSourceItemFaucets + ":raw_material",
			DashboardSourceItemSinks + ":raw_material",
		},
	},
	{
		Key:  DashboardProcessedMaterialSupply,
		Name: "Processed Material Supply",
		Sources: []string{
			DashboardSourceItemFaucets + ":processed_material",
			DashboardSourceItemSinks + ":processed_material",
		},
	},
	{
		Key:  DashboardMarketAveragePrices,
		Name: "Market Average Prices",
		Sources: []string{
			DashboardSourceMarketVolume,
		},
	},
	{
		Key:  DashboardAuctionClearingPrices,
		Name: "Auction Clearing Prices",
		Sources: []string{
			DashboardSourceAuctionVolume,
		},
	},
	{
		Key:  DashboardRepairTotals,
		Name: "Repair Totals",
		Sources: []string{
			DashboardSourceCurrencySinks + ":ship_repair",
		},
	},
	{
		Key:  DashboardCraftFees,
		Name: "Craft Fees",
		Sources: []string{
			DashboardSourceCurrencySinks + ":craft_fee",
		},
	},
	{
		Key:  DashboardRouteLoss,
		Name: "Route Loss",
		Sources: []string{
			DashboardSourceRouteSettlements,
			DashboardSourceItemSinks + ":route_loss",
		},
	},
	{
		Key:  DashboardPlanetProduction,
		Name: "Planet Production",
		Sources: []string{
			DashboardSourcePlanetSettlements,
			DashboardSourceItemFaucets + ":planet_production",
		},
	},
}

// RequiredDashboardSpecs returns deterministic clones of all Phase 12 dashboard specs.
func RequiredDashboardSpecs() []DashboardSpec {
	specs := make([]DashboardSpec, len(requiredDashboardSpecs))
	for i, spec := range requiredDashboardSpecs {
		specs[i] = cloneDashboardSpec(spec)
	}
	return specs
}

func cloneDashboardSpec(spec DashboardSpec) DashboardSpec {
	cloned := spec
	if len(spec.Sources) > 0 {
		cloned.Sources = make([]string, len(spec.Sources))
		copy(cloned.Sources, spec.Sources)
	}
	return cloned
}

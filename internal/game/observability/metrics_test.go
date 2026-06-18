package observability

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestMetricRecorderAggregatesBySortedLabelSets(t *testing.T) {
	recorder := NewMetricRecorder()

	if err := recorder.AddCounter("commands_per_sec", Labels{"zone_id": "zone-1", "op": "combat.use_skill"}, 2); err != nil {
		t.Fatalf("add counter: %v", err)
	}
	if err := recorder.AddCounter("commands_per_sec", Labels{"op": "combat.use_skill", "zone_id": "zone-1"}, 3); err != nil {
		t.Fatalf("add counter same label set: %v", err)
	}
	if err := recorder.SetGauge("visible_entity_count", Labels{"zone_id": "zone-1", "world_id": "world-1"}, 42); err != nil {
		t.Fatalf("set gauge: %v", err)
	}
	if err := recorder.ObserveDuration("zone_tick_ms", Labels{"zone_id": "zone-1", "world_id": "world-1"}, 10*time.Millisecond); err != nil {
		t.Fatalf("observe duration 1: %v", err)
	}
	if err := recorder.ObserveDuration("zone_tick_ms", Labels{"world_id": "world-1", "zone_id": "zone-1"}, 20*time.Millisecond); err != nil {
		t.Fatalf("observe duration 2: %v", err)
	}

	snapshot := recorder.Snapshot()
	if len(snapshot.Counters) != 1 {
		t.Fatalf("counter series = %d, want 1", len(snapshot.Counters))
	}
	counter := snapshot.Counters[0]
	if counter.Value != 5 {
		t.Fatalf("counter value = %d, want 5", counter.Value)
	}
	assertLabels(t, counter.Labels, []Label{
		{Name: "op", Value: "combat.use_skill"},
		{Name: "zone_id", Value: "zone-1"},
	})

	if len(snapshot.Gauges) != 1 {
		t.Fatalf("gauge series = %d, want 1", len(snapshot.Gauges))
	}
	gauge := snapshot.Gauges[0]
	if gauge.Value != 42 {
		t.Fatalf("gauge value = %d, want 42", gauge.Value)
	}
	assertLabels(t, gauge.Labels, []Label{
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "zone-1"},
	})

	if len(snapshot.Durations) != 1 {
		t.Fatalf("duration series = %d, want 1", len(snapshot.Durations))
	}
	duration := snapshot.Durations[0]
	if duration.Count != 2 {
		t.Fatalf("duration count = %d, want 2", duration.Count)
	}
	if duration.Total != 30*time.Millisecond {
		t.Fatalf("duration total = %s, want 30ms", duration.Total)
	}
	if duration.Minimum != 10*time.Millisecond || duration.Maximum != 20*time.Millisecond {
		t.Fatalf("duration min/max = %s/%s, want 10ms/20ms", duration.Minimum, duration.Maximum)
	}
	assertLabels(t, duration.Labels, []Label{
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "zone-1"},
	})

	snapshot.Counters[0].Labels[0].Value = "mutated"
	next := recorder.Snapshot()
	if next.Counters[0].Labels[0].Value != "combat.use_skill" {
		t.Fatalf("snapshot mutation changed stored labels: got %q", next.Counters[0].Labels[0].Value)
	}
}

func TestMetricSnapshotsAreSortedDeterministically(t *testing.T) {
	recorder := NewMetricRecorder()

	if err := recorder.AddCounter("wallet_delta_by_reason", Labels{"reason": "market_sale"}, 10); err != nil {
		t.Fatalf("add wallet counter: %v", err)
	}
	if err := recorder.AddCounter("commands_per_sec", Labels{"op": "loot.pickup"}, 1); err != nil {
		t.Fatalf("add command counter: %v", err)
	}
	if err := recorder.SetGauge("visible_entity_count", Labels{"zone_id": "zone-b"}, 2); err != nil {
		t.Fatalf("set gauge b: %v", err)
	}
	if err := recorder.SetGauge("visible_entity_count", Labels{"zone_id": "zone-a"}, 1); err != nil {
		t.Fatalf("set gauge a: %v", err)
	}

	snapshot := recorder.Snapshot()
	if got := snapshot.Counters[0].Name; got != "commands_per_sec" {
		t.Fatalf("first counter name = %q, want commands_per_sec", got)
	}
	if got := snapshot.Gauges[0].Labels[0].Value; got != "zone-a" {
		t.Fatalf("first gauge label value = %q, want zone-a", got)
	}
}

func TestCommandErrorMetricUsesStableCodeLabelOnly(t *testing.T) {
	recorder := NewMetricRecorder()
	domainErr := foundation.NewDomainError(
		foundation.CodeNotVisible,
		"No valid signal found.",
		foundation.WithDetail("hidden planet at x=10 y=20"),
	)
	code, ok := foundation.CodeOf(domainErr)
	if !ok {
		t.Fatal("domain error did not expose code")
	}

	if err := recorder.RecordCommandError(Operation("scan.pulse"), code); err != nil {
		t.Fatalf("record command error: %v", err)
	}

	snapshot := recorder.Snapshot()
	if len(snapshot.Counters) != 1 {
		t.Fatalf("counter series = %d, want 1", len(snapshot.Counters))
	}
	counter := snapshot.Counters[0]
	if counter.Name != MetricErrorsByCode {
		t.Fatalf("counter name = %q, want %q", counter.Name, MetricErrorsByCode)
	}
	assertLabels(t, counter.Labels, []Label{
		{Name: "code", Value: foundation.CodeNotVisible.String()},
		{Name: "op", Value: "scan.pulse"},
	})

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	got := string(payload)
	for _, leaked := range []string{"No valid signal found", "hidden planet", "x=10", "y=20"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("command error metric leaked %q in %s", leaked, got)
		}
	}
}

func TestMetricRecorderRejectsInvalidNamesLabelsAndNegativeValues(t *testing.T) {
	recorder := NewMetricRecorder()

	tests := []struct {
		name      string
		record    func() error
		wantError error
	}{
		{
			name: "blank metric name",
			record: func() error {
				return recorder.AddCounter("", nil, 1)
			},
			wantError: ErrBlankMetricName,
		},
		{
			name: "unsafe metric name",
			record: func() error {
				return recorder.AddCounter("bad metric", nil, 1)
			},
			wantError: ErrUnsafeMetricName,
		},
		{
			name: "blank label name",
			record: func() error {
				return recorder.AddCounter("commands_per_sec", Labels{"": "combat.use_skill"}, 1)
			},
			wantError: ErrUnsafeLabelName,
		},
		{
			name: "unsafe label name",
			record: func() error {
				return recorder.AddCounter("commands_per_sec", Labels{"bad label": "combat.use_skill"}, 1)
			},
			wantError: ErrUnsafeLabelName,
		},
		{
			name: "unsafe label value",
			record: func() error {
				return recorder.AddCounter("errors_by_code", Labels{"message": "hidden planet at x=10"}, 1)
			},
			wantError: ErrUnsafeLabelValue,
		},
		{
			name: "negative counter",
			record: func() error {
				return recorder.AddCounter("commands_per_sec", nil, -1)
			},
			wantError: ErrNegativeMetricValue,
		},
		{
			name: "negative gauge",
			record: func() error {
				return recorder.SetGauge("visible_entity_count", nil, -1)
			},
			wantError: ErrNegativeMetricValue,
		},
		{
			name: "negative duration",
			record: func() error {
				return recorder.ObserveDuration("zone_tick_ms", nil, -time.Millisecond)
			},
			wantError: ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.record()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestMetricHelpersRecordPhase12Series(t *testing.T) {
	recorder := NewMetricRecorder()

	helpers := []func() error{
		func() error { return recorder.RecordCommandCount(Operation("combat.use_skill")) },
		func() error {
			return recorder.RecordCommandError(Operation("combat.use_skill"), foundation.CodeCooldown)
		},
		func() error {
			return recorder.RecordZoneTickDuration(foundation.WorldID("world-1"), foundation.ZoneID("zone-1"), time.Millisecond)
		},
		func() error {
			return recorder.RecordVisibleEntityCount(foundation.WorldID("world-1"), foundation.ZoneID("zone-1"), 7)
		},
		func() error { return recorder.RecordWalletDelta("market_sale", "credits", "increase", 50) },
		func() error {
			return recorder.RecordItemDelta("loot_pickup", foundation.ItemID("item-1"), "increase", 3)
		},
		func() error { return recorder.RecordCraftJobStarted() },
		func() error { return recorder.RecordCraftJobCompleted() },
		func() error { return recorder.RecordQuestReward("credits") },
		func() error { return recorder.RecordPlanetSettlement("settled") },
		func() error { return recorder.RecordRouteSettlement("settled") },
		func() error { return recorder.RecordMarketSale("credits", 25) },
		func() error { return recorder.RecordAuctionBid("credits", 40) },
	}
	for i, helper := range helpers {
		if err := helper(); err != nil {
			t.Fatalf("helper %d returned error: %v", i, err)
		}
	}

	snapshot := recorder.Snapshot()
	metricNames := map[string]bool{}
	for _, counter := range snapshot.Counters {
		metricNames[counter.Name] = true
	}
	for _, gauge := range snapshot.Gauges {
		metricNames[gauge.Name] = true
	}
	for _, duration := range snapshot.Durations {
		metricNames[duration.Name] = true
	}

	for _, want := range []string{
		MetricCommandsPerSecond,
		MetricErrorsByCode,
		MetricZoneTickMS,
		MetricVisibleEntityCount,
		MetricWalletDeltaByReason,
		MetricItemDeltaByReason,
		MetricCraftJobsStarted,
		MetricCraftJobsCompleted,
		MetricQuestRewardsClaimed,
		MetricPlanetSettlements,
		MetricRouteSettlements,
		MetricMarketVolume,
		MetricAuctionVolume,
	} {
		if !metricNames[want] {
			t.Fatalf("missing helper metric %q in snapshot %#v", want, snapshot)
		}
	}
}

func assertLabels(t *testing.T, got, want []Label) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("labels length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("label[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

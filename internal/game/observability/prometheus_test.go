package observability

import (
	"strings"
	"testing"
	"time"
)

func TestPrometheusTextExportsCountersGaugesAndDurations(t *testing.T) {
	recorder := NewMetricRecorder()
	if err := recorder.RecordCommandCount(Operation("session.snapshot")); err != nil {
		t.Fatalf("record command count: %v", err)
	}
	if err := recorder.RecordVisibleEntityCount("world-1", "map_1_1", 7); err != nil {
		t.Fatalf("record visible count: %v", err)
	}
	if err := recorder.RecordZoneTickDuration("world-1", "map_1_1", 25*time.Millisecond); err != nil {
		t.Fatalf("record zone tick: %v", err)
	}

	text := PrometheusText(recorder.Snapshot())
	for _, want := range []string{
		"# TYPE commands_per_sec counter\n",
		"commands_per_sec{op=\"session.snapshot\"} 1\n",
		"# TYPE visible_entity_count gauge\n",
		"visible_entity_count{world_id=\"world-1\",zone_id=\"map_1_1\"} 7\n",
		"# TYPE zone_tick_ms summary\n",
		"zone_tick_ms{world_id=\"world-1\",zone_id=\"map_1_1\",quantile=\"0.5\"} 25\n",
		"zone_tick_ms_sum{world_id=\"world-1\",zone_id=\"map_1_1\"} 25\n",
		"zone_tick_ms_count{world_id=\"world-1\",zone_id=\"map_1_1\"} 1\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prometheus text missing %q in:\n%s", want, text)
		}
	}
}

func TestPrometheusTextNormalizesIdentifiersAndEscapesLabelValues(t *testing.T) {
	text := PrometheusText(MetricSnapshot{
		Counters: []CounterSnapshot{
			{
				Name: "1.bad-metric",
				Labels: []Label{
					{Name: "bad.label", Value: `quote"slash\ok`},
				},
				Value: 3,
			},
		},
	})
	want := `_bad_metric{bad_label="quote\"slash\\ok"} 3`
	if !strings.Contains(text, want) {
		t.Fatalf("prometheus text missing %q in:\n%s", want, text)
	}
}

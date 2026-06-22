package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

const (
	MetricCommandsPerSecond             = "commands_per_sec"
	MetricErrorsByCode                  = "errors_by_code"
	MetricZoneTickMS                    = "zone_tick_ms"
	MetricVisibleEntityCount            = "visible_entity_count"
	MetricCombatActionsPerSecond        = "combat_actions_per_sec"
	MetricLootCreatedPerSecond          = "loot_created_per_sec"
	MetricLootPickedPerSecond           = "loot_picked_per_sec"
	MetricWalletDeltaByReason           = "wallet_delta_by_reason"
	MetricItemDeltaByReason             = "item_delta_by_reason"
	MetricCraftJobsStarted              = "craft_jobs_started"
	MetricCraftJobsCompleted            = "craft_jobs_completed"
	MetricQuestRewardsClaimed           = "quest_rewards_claimed"
	MetricPlanetSettlements             = "planet_settlements"
	MetricRouteSettlements              = "route_settlements"
	MetricMarketVolume                  = "market_volume"
	MetricMarketQuantity                = "market_quantity"
	MetricMarketSales                   = "market_sales"
	MetricAuctionVolume                 = "auction_volume"
	MetricAuctionClearingVolume         = "auction_clearing_volume"
	MetricAuctionClearingQuantity       = "auction_clearing_quantity"
	MetricAuctionClears                 = "auction_clears"
	MetricEnemySpawnDecisions           = "enemy_spawn_decisions"
	MetricEnemyRespawnDecisions         = "enemy_respawn_decisions"
	MetricEnemyDeathAccounting          = "enemy_death_accounting"
	MetricNPCLootSelectorDecisions      = "npc_loot_selector_decisions"
	MetricEnemyLootSelection            = MetricNPCLootSelectorDecisions
	MetricEnemyAggroDecisions           = "enemy_aggro_decisions"
	MetricEnemySpawnerCommandRejections = "enemy_spawner_command_rejections"
	MetricEnemySpawnerRejections        = MetricEnemySpawnerCommandRejections
)

// Labels is a caller-supplied metric label set.
type Labels map[string]string

// Label is the deterministic snapshot form of one metric label.
type Label struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CounterSnapshot reports a counter value for one metric series.
type CounterSnapshot struct {
	Name   string  `json:"name"`
	Labels []Label `json:"labels,omitempty"`
	Value  int64   `json:"value"`
}

// GaugeSnapshot reports a gauge value for one metric series.
type GaugeSnapshot struct {
	Name   string  `json:"name"`
	Labels []Label `json:"labels,omitempty"`
	Value  int64   `json:"value"`
}

// DurationSummarySnapshot reports aggregate duration observations for one series.
type DurationSummarySnapshot struct {
	Name    string        `json:"name"`
	Labels  []Label       `json:"labels,omitempty"`
	Count   int64         `json:"count"`
	Total   time.Duration `json:"total"`
	Minimum time.Duration `json:"minimum"`
	Maximum time.Duration `json:"maximum"`
	P50     time.Duration `json:"p50"`
	P95     time.Duration `json:"p95"`
	P99     time.Duration `json:"p99"`
}

// MetricSnapshot is a deterministic clone of all recorded metric series.
type MetricSnapshot struct {
	Counters  []CounterSnapshot         `json:"counters,omitempty"`
	Gauges    []GaugeSnapshot           `json:"gauges,omitempty"`
	Durations []DurationSummarySnapshot `json:"durations,omitempty"`
}

type metricKey struct {
	name     string
	labelKey string
}

type counterSeries struct {
	name   string
	labels []Label
	value  int64
}

type gaugeSeries struct {
	name   string
	labels []Label
	value  int64
}

type durationSeries struct {
	name         string
	labels       []Label
	count        int64
	total        time.Duration
	minimum      time.Duration
	maximum      time.Duration
	observations []time.Duration
}

// MetricRecorder stores in-memory gameplay metrics for tests and local runtime slices.
type MetricRecorder struct {
	mu        sync.Mutex
	counters  map[metricKey]counterSeries
	gauges    map[metricKey]gaugeSeries
	durations map[metricKey]durationSeries
}

// NewMetricRecorder returns an empty metric recorder.
func NewMetricRecorder() *MetricRecorder {
	return &MetricRecorder{
		counters:  make(map[metricKey]counterSeries),
		gauges:    make(map[metricKey]gaugeSeries),
		durations: make(map[metricKey]durationSeries),
	}
}

// AddCounter adds delta to a non-negative counter series.
func (recorder *MetricRecorder) AddCounter(name string, labels Labels, delta int64) error {
	normalized, key, err := normalizeMetricInput(name, labels)
	if err != nil {
		return err
	}
	if delta < 0 {
		return fmt.Errorf("counter %q delta %d: %w", name, delta, ErrNegativeMetricValue)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.ensureMaps()

	series := recorder.counters[key]
	if series.name == "" {
		series.name = name
		series.labels = normalized
	}
	series.value += delta
	recorder.counters[key] = series
	return nil
}

// SetGauge records a non-negative gauge value for a metric series.
func (recorder *MetricRecorder) SetGauge(name string, labels Labels, value int64) error {
	normalized, key, err := normalizeMetricInput(name, labels)
	if err != nil {
		return err
	}
	if value < 0 {
		return fmt.Errorf("gauge %q value %d: %w", name, value, ErrNegativeMetricValue)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.ensureMaps()

	recorder.gauges[key] = gaugeSeries{
		name:   name,
		labels: normalized,
		value:  value,
	}
	return nil
}

// ObserveDuration records a non-negative duration for a metric series.
func (recorder *MetricRecorder) ObserveDuration(name string, labels Labels, duration time.Duration) error {
	normalized, key, err := normalizeMetricInput(name, labels)
	if err != nil {
		return err
	}
	if duration < 0 {
		return fmt.Errorf("duration %s for %q: %w", duration, name, ErrInvalidDuration)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.ensureMaps()

	series := recorder.durations[key]
	if series.name == "" {
		series.name = name
		series.labels = normalized
		series.minimum = duration
		series.maximum = duration
	}
	series.count++
	series.total += duration
	series.observations = append(series.observations, duration)
	if duration < series.minimum {
		series.minimum = duration
	}
	if duration > series.maximum {
		series.maximum = duration
	}
	recorder.durations[key] = series
	return nil
}

// Snapshot returns deterministic clones of all metric series.
func (recorder *MetricRecorder) Snapshot() MetricSnapshot {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	snapshot := MetricSnapshot{
		Counters:  make([]CounterSnapshot, 0, len(recorder.counters)),
		Gauges:    make([]GaugeSnapshot, 0, len(recorder.gauges)),
		Durations: make([]DurationSummarySnapshot, 0, len(recorder.durations)),
	}

	for _, series := range recorder.counters {
		snapshot.Counters = append(snapshot.Counters, CounterSnapshot{
			Name:   series.name,
			Labels: cloneLabels(series.labels),
			Value:  series.value,
		})
	}
	for _, series := range recorder.gauges {
		snapshot.Gauges = append(snapshot.Gauges, GaugeSnapshot{
			Name:   series.name,
			Labels: cloneLabels(series.labels),
			Value:  series.value,
		})
	}
	for _, series := range recorder.durations {
		observations := cloneDurations(series.observations)
		sort.Slice(observations, func(i, j int) bool { return observations[i] < observations[j] })
		snapshot.Durations = append(snapshot.Durations, DurationSummarySnapshot{
			Name:    series.name,
			Labels:  cloneLabels(series.labels),
			Count:   series.count,
			Total:   series.total,
			Minimum: series.minimum,
			Maximum: series.maximum,
			P50:     percentileDuration(observations, 50),
			P95:     percentileDuration(observations, 95),
			P99:     percentileDuration(observations, 99),
		})
	}

	sort.Slice(snapshot.Counters, func(i, j int) bool {
		return lessCounter(snapshot.Counters[i], snapshot.Counters[j])
	})
	sort.Slice(snapshot.Gauges, func(i, j int) bool {
		return lessGauge(snapshot.Gauges[i], snapshot.Gauges[j])
	})
	sort.Slice(snapshot.Durations, func(i, j int) bool {
		return lessDuration(snapshot.Durations[i], snapshot.Durations[j])
	})

	return snapshot
}

// RecordCommandCount increments the command counter for op.
func (recorder *MetricRecorder) RecordCommandCount(op Operation) error {
	if err := op.Validate(); err != nil {
		return err
	}
	return recorder.AddCounter(MetricCommandsPerSecond, Labels{"op": op.String()}, 1)
}

// RecordCommandError increments the command error counter by stable error code.
func (recorder *MetricRecorder) RecordCommandError(op Operation, code foundation.Code) error {
	if err := op.Validate(); err != nil {
		return err
	}
	return recorder.AddCounter(MetricErrorsByCode, Labels{
		"code": code.String(),
		"op":   op.String(),
	}, 1)
}

// RecordZoneTickDuration observes a zone tick duration.
func (recorder *MetricRecorder) RecordZoneTickDuration(worldID foundation.WorldID, zoneID foundation.ZoneID, duration time.Duration) error {
	if err := worldID.Validate(); err != nil {
		return err
	}
	if err := zoneID.Validate(); err != nil {
		return err
	}
	return recorder.ObserveDuration(MetricZoneTickMS, Labels{
		"world_id": worldID.String(),
		"zone_id":  zoneID.String(),
	}, duration)
}

// RecordVisibleEntityCount sets the visible entity count for a world zone.
func (recorder *MetricRecorder) RecordVisibleEntityCount(worldID foundation.WorldID, zoneID foundation.ZoneID, count int64) error {
	if err := worldID.Validate(); err != nil {
		return err
	}
	if err := zoneID.Validate(); err != nil {
		return err
	}
	return recorder.SetGauge(MetricVisibleEntityCount, Labels{
		"world_id": worldID.String(),
		"zone_id":  zoneID.String(),
	}, count)
}

// RecordCombatAction increments the combat action counter by action and result.
func (recorder *MetricRecorder) RecordCombatAction(action string, result string) error {
	return recorder.AddCounter(MetricCombatActionsPerSecond, Labels{
		"action": action,
		"result": result,
	}, 1)
}

// RecordLootCreated records created loot item quantity by source and item.
func (recorder *MetricRecorder) RecordLootCreated(sourceType string, itemID foundation.ItemID, quantity int64) error {
	if err := itemID.Validate(); err != nil {
		return err
	}
	return recorder.AddCounter(MetricLootCreatedPerSecond, Labels{
		"item_id":     itemID.String(),
		"source_type": sourceType,
	}, quantity)
}

// RecordLootPicked records picked-up loot item quantity by source and item.
func (recorder *MetricRecorder) RecordLootPicked(sourceType string, itemID foundation.ItemID, quantity int64) error {
	if err := itemID.Validate(); err != nil {
		return err
	}
	return recorder.AddCounter(MetricLootPickedPerSecond, Labels{
		"item_id":     itemID.String(),
		"source_type": sourceType,
	}, quantity)
}

// RecordWalletDelta records a non-negative wallet movement by stable reason.
func (recorder *MetricRecorder) RecordWalletDelta(reason, currencyType, action string, amount int64) error {
	return recorder.AddCounter(MetricWalletDeltaByReason, Labels{
		"action":        action,
		"currency_type": currencyType,
		"reason":        reason,
	}, amount)
}

// RecordItemDelta records a non-negative item movement by stable reason.
func (recorder *MetricRecorder) RecordItemDelta(reason string, itemID foundation.ItemID, action string, quantity int64) error {
	if err := itemID.Validate(); err != nil {
		return err
	}
	return recorder.AddCounter(MetricItemDeltaByReason, Labels{
		"action":  action,
		"item_id": itemID.String(),
		"reason":  reason,
	}, quantity)
}

// RecordCraftJobStarted increments the craft job start counter.
func (recorder *MetricRecorder) RecordCraftJobStarted() error {
	return recorder.AddCounter(MetricCraftJobsStarted, nil, 1)
}

// RecordCraftJobCompleted increments the craft job completion counter.
func (recorder *MetricRecorder) RecordCraftJobCompleted() error {
	return recorder.AddCounter(MetricCraftJobsCompleted, nil, 1)
}

// RecordQuestReward increments the quest reward counter by stable reward type.
func (recorder *MetricRecorder) RecordQuestReward(rewardType string) error {
	return recorder.AddCounter(MetricQuestRewardsClaimed, Labels{"reward_type": rewardType}, 1)
}

// RecordPlanetSettlement increments the planet settlement counter by status.
func (recorder *MetricRecorder) RecordPlanetSettlement(status string) error {
	return recorder.AddCounter(MetricPlanetSettlements, Labels{"status": status}, 1)
}

// RecordRouteSettlement increments the route settlement counter by status.
func (recorder *MetricRecorder) RecordRouteSettlement(status string) error {
	return recorder.AddCounter(MetricRouteSettlements, Labels{"status": status}, 1)
}

// RecordMarketSale records market sale volume, quantity, and count by item.
func (recorder *MetricRecorder) RecordMarketSale(currencyType string, itemID foundation.ItemID, quantity int64, totalPrice int64) error {
	if err := validateLabelValue(currencyType); err != nil {
		return err
	}
	if err := itemID.Validate(); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(quantity); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(totalPrice); err != nil {
		return err
	}

	labels := Labels{
		"currency_type": currencyType,
		"item_id":       itemID.String(),
	}
	if err := recorder.AddCounter(MetricMarketVolume, labels, totalPrice); err != nil {
		return err
	}
	if err := recorder.AddCounter(MetricMarketQuantity, labels, quantity); err != nil {
		return err
	}
	return recorder.AddCounter(MetricMarketSales, labels, 1)
}

// RecordAuctionBid adds bid amount to auction volume by currency type.
func (recorder *MetricRecorder) RecordAuctionBid(currencyType string, amount int64) error {
	return recorder.AddCounter(MetricAuctionVolume, Labels{"currency_type": currencyType}, amount)
}

// RecordAuctionClearing records completed auction sale volume, quantity, and count by item.
func (recorder *MetricRecorder) RecordAuctionClearing(currencyType string, itemID foundation.ItemID, quantity int64, totalPrice int64) error {
	if err := validateLabelValue(currencyType); err != nil {
		return err
	}
	if err := itemID.Validate(); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(quantity); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(totalPrice); err != nil {
		return err
	}

	labels := Labels{
		"currency_type": currencyType,
		"item_id":       itemID.String(),
	}
	if err := recorder.AddCounter(MetricAuctionClearingVolume, labels, totalPrice); err != nil {
		return err
	}
	if err := recorder.AddCounter(MetricAuctionClearingQuantity, labels, quantity); err != nil {
		return err
	}
	return recorder.AddCounter(MetricAuctionClears, labels, 1)
}

// RecordEnemySpawnDecision increments a safe Phase08 enemy spawn decision counter.
func (recorder *MetricRecorder) RecordEnemySpawnDecision(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason, npcType, spawnMode string) error {
	labels, err := enemyLifecycleLabels(worldID, zoneID, mapKey, riskBand, stage, result, reason)
	if err != nil {
		return err
	}
	labels["npc_type"] = safeMetricLabelValue(npcType)
	labels["spawn_mode"] = safeMetricLabelValue(spawnMode)
	return recorder.AddCounter(MetricEnemySpawnDecisions, labels, 1)
}

// RecordEnemyRespawnDecision increments a safe Phase08 enemy respawn decision counter.
func (recorder *MetricRecorder) RecordEnemyRespawnDecision(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason, npcType, spawnMode string) error {
	labels, err := enemyLifecycleLabels(worldID, zoneID, mapKey, riskBand, stage, result, reason)
	if err != nil {
		return err
	}
	labels["npc_type"] = safeMetricLabelValue(npcType)
	labels["spawn_mode"] = safeMetricLabelValue(spawnMode)
	return recorder.AddCounter(MetricEnemyRespawnDecisions, labels, 1)
}

// RecordEnemyDeathAccounting increments a safe Phase08 enemy death accounting counter.
func (recorder *MetricRecorder) RecordEnemyDeathAccounting(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason, npcType string) error {
	labels, err := enemyLifecycleLabels(worldID, zoneID, mapKey, riskBand, stage, result, reason)
	if err != nil {
		return err
	}
	labels["npc_type"] = safeMetricLabelValue(npcType)
	return recorder.AddCounter(MetricEnemyDeathAccounting, labels, 1)
}

// RecordNPCLootSelectorDecision increments a safe Phase08 NPC loot selector counter.
func (recorder *MetricRecorder) RecordNPCLootSelectorDecision(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason, npcType string) error {
	labels, err := enemyLifecycleLabels(worldID, zoneID, mapKey, riskBand, stage, result, reason)
	if err != nil {
		return err
	}
	labels["npc_type"] = safeMetricLabelValue(npcType)
	return recorder.AddCounter(MetricNPCLootSelectorDecisions, labels, 1)
}

// RecordEnemyAggroDecision increments a safe Phase08 enemy aggro decision counter.
func (recorder *MetricRecorder) RecordEnemyAggroDecision(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason, npcType string) error {
	labels, err := enemyLifecycleLabels(worldID, zoneID, mapKey, riskBand, stage, result, reason)
	if err != nil {
		return err
	}
	labels["npc_type"] = safeMetricLabelValue(npcType)
	return recorder.AddCounter(MetricEnemyAggroDecisions, labels, 1)
}

// RecordEnemySpawnerCommandRejection increments a safe Phase08 worker command rejection counter.
func (recorder *MetricRecorder) RecordEnemySpawnerCommandRejection(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason string) error {
	labels, err := enemyLifecycleLabels(worldID, zoneID, mapKey, riskBand, stage, result, reason)
	if err != nil {
		return err
	}
	return recorder.AddCounter(MetricEnemySpawnerCommandRejections, labels, 1)
}

func (recorder *MetricRecorder) ensureMaps() {
	if recorder.counters == nil {
		recorder.counters = make(map[metricKey]counterSeries)
	}
	if recorder.gauges == nil {
		recorder.gauges = make(map[metricKey]gaugeSeries)
	}
	if recorder.durations == nil {
		recorder.durations = make(map[metricKey]durationSeries)
	}
}

func normalizeMetricInput(name string, labels Labels) ([]Label, metricKey, error) {
	if err := validateMetricName(name); err != nil {
		return nil, metricKey{}, err
	}
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return nil, metricKey{}, err
	}
	return normalized, metricKey{
		name:     name,
		labelKey: buildLabelKey(normalized),
	}, nil
}

func validateMetricName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrBlankMetricName
	}
	if !isSafeName(name) {
		return fmt.Errorf("metric name %q: %w", name, ErrUnsafeMetricName)
	}
	return nil
}

func normalizeLabels(labels Labels) ([]Label, error) {
	normalized := make([]Label, 0, len(labels))
	for name, value := range labels {
		if err := validateLabelName(name); err != nil {
			return nil, err
		}
		if err := validateLabelValue(value); err != nil {
			return nil, err
		}
		normalized = append(normalized, Label{Name: name, Value: value})
	}
	sortLabels(normalized)
	return normalized, nil
}

func validateLabelName(name string) error {
	if strings.TrimSpace(name) == "" || !isSafeName(name) {
		return fmt.Errorf("label name %q: %w", name, ErrUnsafeLabelName)
	}
	return nil
}

func validateLabelValue(value string) error {
	if strings.TrimSpace(value) == "" || !isSafeName(value) {
		return fmt.Errorf("label value %q: %w", value, ErrUnsafeLabelValue)
	}
	return nil
}

func enemyLifecycleLabels(worldID foundation.WorldID, zoneID foundation.ZoneID, mapKey, riskBand, stage, result, reason string) (Labels, error) {
	if err := worldID.Validate(); err != nil {
		return nil, err
	}
	if err := zoneID.Validate(); err != nil {
		return nil, err
	}
	return Labels{
		"map_key":   safeMetricLabelValue(mapKey),
		"reason":    safeMetricLabelValue(reason),
		"result":    safeMetricLabelValue(result),
		"risk_band": safeMetricLabelValue(riskBand),
		"stage":     safeMetricLabelValue(stage),
		"world_id":  worldID.String(),
		"zone_id":   zoneID.String(),
	}, nil
}

func safeMetricLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if err := validateLabelValue(value); err != nil {
		return "unknown"
	}
	return value
}

func isSafeName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		switch char {
		case '_', ':', '-', '.':
			continue
		default:
			return false
		}
	}
	return true
}

func cloneLabels(labels []Label) []Label {
	if len(labels) == 0 {
		return nil
	}
	cloned := make([]Label, len(labels))
	copy(cloned, labels)
	return cloned
}

func cloneDurations(durations []time.Duration) []time.Duration {
	if len(durations) == 0 {
		return nil
	}
	cloned := make([]time.Duration, len(durations))
	copy(cloned, durations)
	return cloned
}

func percentileDuration(sortedObservations []time.Duration, percentile int) time.Duration {
	if len(sortedObservations) == 0 {
		return 0
	}
	index := ((len(sortedObservations) * percentile) + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(sortedObservations) {
		index = len(sortedObservations)
	}
	return sortedObservations[index-1]
}

func sortLabels(labels []Label) {
	sort.Slice(labels, func(i, j int) bool {
		if labels[i].Name != labels[j].Name {
			return labels[i].Name < labels[j].Name
		}
		return labels[i].Value < labels[j].Value
	})
}

func buildLabelKey(labels []Label) string {
	var builder strings.Builder
	for _, label := range labels {
		builder.WriteString(fmt.Sprintf("%d:%s=%d:%s;", len(label.Name), label.Name, len(label.Value), label.Value))
	}
	return builder.String()
}

func lessCounter(left, right CounterSnapshot) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return lessLabelSet(left.Labels, right.Labels)
}

func lessGauge(left, right GaugeSnapshot) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return lessLabelSet(left.Labels, right.Labels)
}

func lessDuration(left, right DurationSummarySnapshot) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return lessLabelSet(left.Labels, right.Labels)
}

func lessLabelSet(left, right []Label) bool {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i].Name != right[i].Name {
			return left[i].Name < right[i].Name
		}
		if left[i].Value != right[i].Value {
			return left[i].Value < right[i].Value
		}
	}
	return len(left) < len(right)
}

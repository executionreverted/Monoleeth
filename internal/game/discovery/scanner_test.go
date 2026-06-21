package discovery

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
)

const (
	scannerTestPlayerID foundation.PlayerID = "player_scout"
	scannerTestWorldID  foundation.WorldID  = "world_1"
	scannerTestZoneID   foundation.ZoneID   = "zone_1"
	scannerTestShipID   foundation.ShipID   = "ship_1"
)

func TestStartScanPulseWithoutModuleFailsGenericallyWithoutMutation(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	xp := &recordingScanXPProvider{}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:       seed,
		store:      store,
		position:   candidate.Position(),
		snapshot:   scannerSnapshot(candidate, 10_000, 10_000, 0),
		moduleOK:   false,
		cooldownOK: true,
		xp:         xp,
	})

	_, err := service.StartScanPulse(scannerStartInput("pulse_no_module"))
	if !errors.Is(err, ErrScannerUnavailable) {
		t.Fatalf("StartScanPulse() error = %v, want ErrScannerUnavailable", err)
	}
	if got := len(store.Planets()); got != 0 {
		t.Fatalf("planets materialized = %d, want 0", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("scanner events = %d, want 0", got)
	}
	if got := len(xp.calls); got != 0 {
		t.Fatalf("xp grants = %d, want 0", got)
	}
}

func TestStartScanPulseCooldownBlocksSpamWithoutMutation(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	xp := &recordingScanXPProvider{}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:       seed,
		store:      store,
		position:   candidate.Position(),
		snapshot:   scannerSnapshot(candidate, 10_000, 10_000, 0),
		moduleOK:   true,
		cooldownOK: false,
		xp:         xp,
	})

	_, err := service.StartScanPulse(scannerStartInput("pulse_cooldown"))
	if !errors.Is(err, ErrScanCooldownActive) {
		t.Fatalf("StartScanPulse() error = %v, want ErrScanCooldownActive", err)
	}
	if got := len(store.Planets()); got != 0 {
		t.Fatalf("planets materialized = %d, want 0", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("scanner events = %d, want 0", got)
	}
	if got := len(xp.calls); got != 0 {
		t.Fatalf("xp grants = %d, want 0", got)
	}
}

func TestStartScanPulseEnergyUnavailableFailsBeforeCooldownAndMutation(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	cooldowns := &recordingScannerCooldownProvider{accepted: true}
	energy := &recordingScannerEnergyProvider{accepted: false}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:      seed,
		store:     NewInMemoryStore(),
		position:  candidate.Position(),
		snapshot:  scannerSnapshot(candidate, 10_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:  true,
		cooldowns: cooldowns,
		energy:    energy,
		xp:        &recordingScanXPProvider{},
	})

	_, err := service.StartScanPulse(scannerStartInput("pulse_no_energy"))
	if !errors.Is(err, ErrScannerEnergyUnavailable) {
		t.Fatalf("StartScanPulse() error = %v, want ErrScannerEnergyUnavailable", err)
	}
	if cooldowns.calls != 0 {
		t.Fatalf("cooldown calls after energy rejection = %d, want 0", cooldowns.calls)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("scanner events after energy rejection = %d, want 0", got)
	}
}

func TestStartScanPulseMovingShipFailsBeforeCooldownAndMutation(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	cooldowns := &recordingScannerCooldownProvider{accepted: true}
	movement, err := world.NewTimedMovementState(
		candidate.Position(),
		world.Vec2{X: candidate.Position().X + 1, Y: candidate.Position().Y + 1},
		10,
		scannerTestTime(),
	)
	if err != nil {
		t.Fatalf("NewTimedMovementState() error = %v, want nil", err)
	}
	position := ScannerPosition{
		WorldID:  scannerTestWorldID,
		ZoneID:   scannerTestZoneID,
		Position: candidate.Position(),
		Movement: movement,
	}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:            seed,
		store:           NewInMemoryStore(),
		scannerPosition: position,
		snapshot:        scannerSnapshot(candidate, 10_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:        true,
		cooldowns:       cooldowns,
		xp:              &recordingScanXPProvider{},
	})

	_, err = service.StartScanPulse(scannerStartInput("pulse_moving"))
	if !errors.Is(err, ErrScanMovementRestricted) {
		t.Fatalf("StartScanPulse() error = %v, want ErrScanMovementRestricted", err)
	}
	if cooldowns.calls != 0 {
		t.Fatalf("cooldown calls after movement rejection = %d, want 0", cooldowns.calls)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("scanner events after movement rejection = %d, want 0", got)
	}
}

func TestStartScanPulseDuplicateReferenceIsScopedAndDoesNotRestartCooldown(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	cooldowns := &recordingScannerCooldownProvider{accepted: true, readyAt: scannerTestTime()}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:      seed,
		store:     NewInMemoryStore(),
		position:  candidate.Position(),
		snapshot:  scannerSnapshot(candidate, 10_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:  true,
		cooldowns: cooldowns,
		xp:        &recordingScanXPProvider{},
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_duplicate_start")); err != nil {
		t.Fatalf("first StartScanPulse() error = %v, want nil", err)
	}
	if _, err := service.StartScanPulse(scannerStartInput("pulse_duplicate_start")); err != nil {
		t.Fatalf("duplicate StartScanPulse() error = %v, want nil", err)
	}
	if got := cooldowns.calls; got != 1 {
		t.Fatalf("cooldown calls after duplicate start = %d, want 1", got)
	}

	otherPlayer := scannerStartInput("pulse_duplicate_start")
	otherPlayer.PlayerID = "player_other"
	if _, err := service.StartScanPulse(otherPlayer); !errors.Is(err, ErrScanPulseNotFound) {
		t.Fatalf("cross-player duplicate StartScanPulse() error = %v, want ErrScanPulseNotFound", err)
	}
	if got := cooldowns.calls; got != 1 {
		t.Fatalf("cooldown calls after cross-player duplicate = %d, want 1", got)
	}
}

func TestStartScanPulseZeroScanIntervalUsesMinimumCooldown(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	cooldowns := &recordingScannerCooldownProvider{accepted: true}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:      seed,
		store:     NewInMemoryStore(),
		position:  candidate.Position(),
		snapshot:  scannerSnapshot(candidate, 10_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:  true,
		cooldowns: cooldowns,
		xp:        &recordingScanXPProvider{},
	})

	result, err := service.StartScanPulse(scannerStartInput("pulse_min_cooldown"))
	if err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	if got := cooldowns.inputs[0].Duration; got != time.Second {
		t.Fatalf("cooldown duration = %s, want %s", got, time.Second)
	}
	if want := scannerTestTime().Add(time.Second); !result.ResolveAfter.Equal(want) {
		t.Fatalf("resolve_after = %s, want %s", result.ResolveAfter, want)
	}
}

func TestResolveScanPulseRadarTooLowReturnsGenericNoSignal(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	xp := &recordingScanXPProvider{}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:       seed,
		store:      store,
		position:   candidate.Position(),
		snapshot:   scannerSnapshot(candidate, 10_000, 0, 0),
		moduleOK:   true,
		cooldownOK: true,
		xp:         xp,
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_low_radar")); err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	result, err := service.ResolveScanPulse(scannerResolveInput("pulse_low_radar"))
	if err != nil {
		t.Fatalf("ResolveScanPulse() error = %v, want nil", err)
	}
	if result.Status != ScanPulseStatusNoSignal {
		t.Fatalf("result status = %q, want %q", result.Status, ScanPulseStatusNoSignal)
	}
	if result.Signal != nil || result.PlanetID != "" {
		t.Fatalf("no-signal result leaked signal/planet data: %+v", result)
	}
	assertNoScannerHiddenLeak(t, result, candidate)
	if got := len(store.Planets()); got != 0 {
		t.Fatalf("planets materialized = %d, want 0", got)
	}
	if got := len(xp.calls); got != 0 {
		t.Fatalf("xp grants = %d, want 0", got)
	}
}

func TestResolveScanPulseDiscoversPlanetWritesIntelEventAndXPOnce(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	xp := &recordingScanXPProvider{}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:       seed,
		store:      store,
		position:   candidate.Position(),
		snapshot:   scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:   true,
		cooldownOK: true,
		xp:         xp,
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_success")); err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	result, err := service.ResolveScanPulse(scannerResolveInput("pulse_success"))
	if err != nil {
		t.Fatalf("ResolveScanPulse() error = %v, want nil", err)
	}
	if result.Status != ScanPulseStatusPlanetDiscovered {
		t.Fatalf("result status = %q, want %q", result.Status, ScanPulseStatusPlanetDiscovered)
	}
	if result.PlanetID == "" {
		t.Fatal("result PlanetID is empty, want materialized planet id")
	}
	if result.Signal == nil {
		t.Fatal("result Signal is nil, want client-safe signal")
	}

	planets := store.Planets()
	if len(planets) != 1 {
		t.Fatalf("planets materialized = %d, want 1", len(planets))
	}
	if planets[0].ID != result.PlanetID {
		t.Fatalf("materialized planet id = %q, want result planet id %q", planets[0].ID, result.PlanetID)
	}
	intel, ok, err := store.PlayerPlanetIntel(scannerTestPlayerID, result.PlanetID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel() ok = %v err = %v, want true nil", ok, err)
	}
	if intel.SourceType != IntelSourceScanSuccess || intel.SourceReference != "pulse_success" {
		t.Fatalf("intel source = %s/%s, want scan_success/pulse_success", intel.SourceType, intel.SourceReference)
	}
	if !hasScannerEvent(service.Events(), ScannerEventPlanetDiscovered) {
		t.Fatalf("scanner events = %+v, want %s", service.Events(), ScannerEventPlanetDiscovered)
	}
	if len(xp.calls) != 1 {
		t.Fatalf("xp grants = %d, want 1", len(xp.calls))
	}
	grant := xp.calls[0]
	if grant.SourceType != progression.XPSourceTypeScan {
		t.Fatalf("xp source type = %q, want %q", grant.SourceType, progression.XPSourceTypeScan)
	}
	if grant.Authority != progression.XPGrantAuthorityScannerService {
		t.Fatalf("xp authority = %q, want %q", grant.Authority, progression.XPGrantAuthorityScannerService)
	}
	if !strings.Contains(grant.SourceID.String(), result.PlanetID.String()) {
		t.Fatalf("xp source id = %q, want planet id %q", grant.SourceID, result.PlanetID)
	}
	if grant.IdempotencyKey.IsZero() {
		t.Fatal("xp idempotency key is empty")
	}
}

func TestResolveScanPulseMaterializationAndIntelAreZoneScoped(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	zoneOne := foundation.ZoneID("map_1_1")
	zoneTwo := foundation.ZoneID("map_1_2")

	serviceOne := newScannerTestService(t, scannerTestServiceOptions{
		seed:     seed,
		store:    store,
		position: candidate.Position(),
		scannerPosition: ScannerPosition{
			WorldID:  scannerTestWorldID,
			ZoneID:   zoneOne,
			Position: candidate.Position(),
		},
		snapshot:   scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:   true,
		cooldownOK: true,
		xp:         &recordingScanXPProvider{},
	})
	serviceTwo := newScannerTestService(t, scannerTestServiceOptions{
		seed:     seed,
		store:    store,
		position: candidate.Position(),
		scannerPosition: ScannerPosition{
			WorldID:  scannerTestWorldID,
			ZoneID:   zoneTwo,
			Position: candidate.Position(),
		},
		snapshot:   scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:   true,
		cooldownOK: true,
		xp:         &recordingScanXPProvider{},
	})

	inputOne := scannerStartInputForZone("pulse_map_one", zoneOne)
	if _, err := serviceOne.StartScanPulse(inputOne); err != nil {
		t.Fatalf("StartScanPulse(map one) error = %v, want nil", err)
	}
	resultOne, err := serviceOne.ResolveScanPulse(scannerResolveInputForZone("pulse_map_one", zoneOne))
	if err != nil {
		t.Fatalf("ResolveScanPulse(map one) error = %v, want nil", err)
	}

	inputTwo := scannerStartInputForZone("pulse_map_two", zoneTwo)
	if _, err := serviceTwo.StartScanPulse(inputTwo); err != nil {
		t.Fatalf("StartScanPulse(map two) error = %v, want nil", err)
	}
	resultTwo, err := serviceTwo.ResolveScanPulse(scannerResolveInputForZone("pulse_map_two", zoneTwo))
	if err != nil {
		t.Fatalf("ResolveScanPulse(map two) error = %v, want nil", err)
	}

	if resultOne.PlanetID == "" || resultTwo.PlanetID == "" || resultOne.PlanetID == resultTwo.PlanetID {
		t.Fatalf("planet ids map one=%q map two=%q, want distinct map-scoped materializations", resultOne.PlanetID, resultTwo.PlanetID)
	}
	planets := store.Planets()
	if len(planets) != 2 {
		t.Fatalf("materialized planets = %d, want 2 for same local cell in different maps", len(planets))
	}
	planetZones := map[foundation.ZoneID]bool{}
	for _, planet := range planets {
		planetZones[planet.ZoneID] = true
	}
	if !planetZones[zoneOne] || !planetZones[zoneTwo] {
		t.Fatalf("planet zones = %#v, want both %s and %s", planetZones, zoneOne, zoneTwo)
	}

	intelRows, err := store.PlayerPlanetIntelRecords(scannerTestPlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intelRows) != 2 {
		t.Fatalf("intel rows = %d, want two map-scoped rows", len(intelRows))
	}
	intelZones := map[foundation.ZoneID]bool{}
	for _, intel := range intelRows {
		intelZones[intel.ZoneID] = true
		if intel.WorldID != scannerTestWorldID {
			t.Fatalf("intel world = %q, want %q", intel.WorldID, scannerTestWorldID)
		}
	}
	if !intelZones[zoneOne] || !intelZones[zoneTwo] {
		t.Fatalf("intel zones = %#v, want both %s and %s", intelZones, zoneOne, zoneTwo)
	}
}

func TestResolveScanPulsePlayerRevealDoesNotCreatePlanetIntelOrXP(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	xp := &recordingScanXPProvider{}
	reveals := &recordingScannerPlayerRevealProvider{revealed: true}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:          seed,
		store:         store,
		position:      candidate.Position(),
		snapshot:      scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:      true,
		cooldownOK:    true,
		xp:            xp,
		playerReveals: reveals,
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_player_reveal")); err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	result, err := service.ResolveScanPulse(scannerResolveInput("pulse_player_reveal"))
	if err != nil {
		t.Fatalf("ResolveScanPulse() error = %v, want nil", err)
	}
	if result.Status != ScanPulseStatusPlayerRevealed {
		t.Fatalf("result status = %q, want %q", result.Status, ScanPulseStatusPlayerRevealed)
	}
	if result.Signal != nil || result.PlanetID != "" || result.XPGranted {
		t.Fatalf("player reveal result = %+v, want no signal, planet, or XP", result)
	}
	if got := len(store.Planets()); got != 0 {
		t.Fatalf("planets materialized = %d, want 0", got)
	}
	intel, err := store.PlayerPlanetIntelRecords(scannerTestPlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intel) != 0 {
		t.Fatalf("intel records = %+v, want none", intel)
	}
	if len(xp.calls) != 0 {
		t.Fatalf("xp grants = %d, want 0", len(xp.calls))
	}
	if !hasScannerEvent(service.Events(), ScannerEventPlayerRevealed) {
		t.Fatalf("scanner events = %+v, want %s", service.Events(), ScannerEventPlayerRevealed)
	}
	if reveals.calls != 1 {
		t.Fatalf("player reveal calls = %d, want 1", reveals.calls)
	}
	assertNoScannerHiddenLeak(t, result, candidate)
}

func TestResolveScanPulsePlayerRevealDuplicateDoesNotCallProviderTwice(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	reveals := &recordingScannerPlayerRevealProvider{revealed: true}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:          seed,
		store:         NewInMemoryStore(),
		position:      candidate.Position(),
		snapshot:      scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:      true,
		cooldownOK:    true,
		xp:            &recordingScanXPProvider{},
		playerReveals: reveals,
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_player_reveal_duplicate")); err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	first, err := service.ResolveScanPulse(scannerResolveInput("pulse_player_reveal_duplicate"))
	if err != nil {
		t.Fatalf("first ResolveScanPulse() error = %v, want nil", err)
	}
	duplicate, err := service.ResolveScanPulse(scannerResolveInput("pulse_player_reveal_duplicate"))
	if err != nil {
		t.Fatalf("duplicate ResolveScanPulse() error = %v, want nil", err)
	}
	if first.Status != ScanPulseStatusPlayerRevealed || duplicate.Status != ScanPulseStatusPlayerRevealed || !duplicate.Duplicate {
		t.Fatalf("first=%+v duplicate=%+v, want duplicate player reveal", first, duplicate)
	}
	if reveals.calls != 1 {
		t.Fatalf("player reveal calls = %d, want 1", reveals.calls)
	}
}

func TestResolveScanPulseDuplicateIsIdempotent(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	store := NewInMemoryStore()
	xp := &recordingScanXPProvider{}
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:       seed,
		store:      store,
		position:   candidate.Position(),
		snapshot:   scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:   true,
		cooldownOK: true,
		xp:         xp,
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_duplicate")); err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	first, err := service.ResolveScanPulse(scannerResolveInput("pulse_duplicate"))
	if err != nil {
		t.Fatalf("first ResolveScanPulse() error = %v, want nil", err)
	}
	eventCount := len(service.Events())

	duplicate, err := service.ResolveScanPulse(scannerResolveInput("pulse_duplicate"))
	if err != nil {
		t.Fatalf("duplicate ResolveScanPulse() error = %v, want nil", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate result Duplicate = false, want true")
	}
	if duplicate.XPGranted {
		t.Fatal("duplicate result XPGranted = true, want false")
	}
	if duplicate.PlanetID != first.PlanetID || duplicate.Status != first.Status {
		t.Fatalf("duplicate result = %+v, want same planet/status as %+v", duplicate, first)
	}
	if got := len(store.Planets()); got != 1 {
		t.Fatalf("planets materialized after duplicate = %d, want 1", got)
	}
	intelRecords, err := store.PlayerPlanetIntelRecords(scannerTestPlayerID)
	if err != nil {
		t.Fatalf("PlayerPlanetIntelRecords() error = %v, want nil", err)
	}
	if len(intelRecords) != 1 {
		t.Fatalf("intel records after duplicate = %d, want 1", len(intelRecords))
	}
	if got := len(xp.calls); got != 1 {
		t.Fatalf("xp grants after duplicate = %d, want 1", got)
	}
	if got := len(service.Events()); got != eventCount {
		t.Fatalf("scanner events after duplicate = %d, want unchanged %d", got, eventCount)
	}

	crossPlayer := scannerResolveInput("pulse_duplicate")
	crossPlayer.PlayerID = "player_other"
	if _, err := service.ResolveScanPulse(crossPlayer); !errors.Is(err, ErrScanPulseNotFound) {
		t.Fatalf("cross-player duplicate ResolveScanPulse() error = %v, want ErrScanPulseNotFound", err)
	}
}

func TestResolveScanPulseResultJSONOmitsHiddenTruth(t *testing.T) {
	seed := scannerTestSeed(t)
	_, candidate := findScannerTestCandidate(t, seed)
	service := newScannerTestService(t, scannerTestServiceOptions{
		seed:       seed,
		store:      NewInMemoryStore(),
		position:   candidate.Position(),
		snapshot:   scannerSnapshot(candidate, 100_000, scannerRadarRangeFor(candidate), 0),
		moduleOK:   true,
		cooldownOK: true,
		xp:         &recordingScanXPProvider{},
	})

	if _, err := service.StartScanPulse(scannerStartInput("pulse_json")); err != nil {
		t.Fatalf("StartScanPulse() error = %v, want nil", err)
	}
	result, err := service.ResolveScanPulse(scannerResolveInput("pulse_json"))
	if err != nil {
		t.Fatalf("ResolveScanPulse() error = %v, want nil", err)
	}
	assertNoScannerHiddenLeak(t, result, candidate)
}

type scannerTestServiceOptions struct {
	seed            WorldSeed
	store           *InMemoryStore
	position        world.Vec2
	scannerPosition ScannerPosition
	snapshot        stats.StatSnapshot
	moduleOK        bool
	cooldownOK      bool
	cooldowns       ScannerCooldownProvider
	energy          ScannerEnergyProvider
	playerReveals   ScannerPlayerRevealProvider
	xp              *recordingScanXPProvider
}

func newScannerTestService(t *testing.T, options scannerTestServiceOptions) *ScannerService {
	t.Helper()
	scannerPosition := ScannerPosition{
		WorldID:  scannerTestWorldID,
		ZoneID:   scannerTestZoneID,
		Position: options.position,
	}
	if options.scannerPosition.WorldID != "" || options.scannerPosition.ZoneID != "" {
		scannerPosition = options.scannerPosition
	}
	service, err := NewScannerService(ScannerServiceConfig{
		Store:     options.store,
		WorldSeed: options.seed,
		Clock:     fixedScannerClock{now: scannerTestTime()},
		Modules:   fixedScannerModuleProvider{equipped: options.moduleOK},
		Stats:     fixedScannerStatsProvider{snapshot: options.snapshot},
		Positions: fixedScannerPositionProvider{
			position: scannerPosition,
		},
		Cooldowns: scannerCooldownProviderForTest(options),
		Energy:    scannerEnergyProviderForTest(options),
		Reveals:   options.playerReveals,
		XP:        options.xp,
		CandidateOptions: CandidateGenerationOptions{
			DiscoveryHorizon: defaultScannerDiscoveryHorizon,
			SpawnBudget:      defaultScannerSpawnBudget,
		},
	})
	if err != nil {
		t.Fatalf("NewScannerService() error = %v, want nil", err)
	}
	return service
}

func scannerCooldownProviderForTest(options scannerTestServiceOptions) ScannerCooldownProvider {
	if options.cooldowns != nil {
		return options.cooldowns
	}
	return fixedScannerCooldownProvider{accepted: options.cooldownOK, readyAt: scannerTestTime()}
}

func scannerEnergyProviderForTest(options scannerTestServiceOptions) ScannerEnergyProvider {
	if options.energy != nil {
		return options.energy
	}
	return fixedScannerEnergyProvider{accepted: true}
}

func scannerStartInput(ref ScanPulseReference) StartScanPulseInput {
	return scannerStartInputForZone(ref, scannerTestZoneID)
}

func scannerStartInputForZone(ref ScanPulseReference, zoneID foundation.ZoneID) StartScanPulseInput {
	return StartScanPulseInput{
		PlayerID:       scannerTestPlayerID,
		WorldID:        scannerTestWorldID,
		ZoneID:         zoneID,
		ShipID:         scannerTestShipID,
		PulseReference: ref,
	}
}

func scannerResolveInput(ref ScanPulseReference) ResolveScanPulseInput {
	return scannerResolveInputForZone(ref, scannerTestZoneID)
}

func scannerResolveInputForZone(ref ScanPulseReference, zoneID foundation.ZoneID) ResolveScanPulseInput {
	return ResolveScanPulseInput{
		PlayerID:       scannerTestPlayerID,
		WorldID:        scannerTestWorldID,
		ZoneID:         zoneID,
		PulseReference: ref,
	}
}

func scannerSnapshot(candidate PlanetCandidate, scanPower float64, radarRange float64, scanInterval float64) stats.StatSnapshot {
	return stats.NewStatSnapshot(
		scannerTestPlayerID,
		scannerTestShipID,
		1,
		stats.EffectiveStats{
			Exploration: stats.ExplorationStats{
				RadarRange:   radarRange,
				ScanPower:    scanPower,
				ScanRadius:   candidate.Position().Distance(world.Vec2{}) + DefaultScanCellSize + 10_000,
				ScanInterval: scanInterval,
			},
		},
		scannerTestTime(),
	)
}

func scannerRadarRangeFor(candidate PlanetCandidate) float64 {
	return float64(candidate.MinRadarLevel()) * DefaultScanCellSize
}

func scannerTestSeed(t *testing.T) WorldSeed {
	t.Helper()
	seed, err := NewWorldSeed(WorldSeedInput{StaticSeed: []byte("scanner-test-static-seed")})
	if err != nil {
		t.Fatalf("NewWorldSeed() error = %v, want nil", err)
	}
	return seed
}

func findScannerTestCandidate(t *testing.T, seed WorldSeed) (ScanCellCoord, PlanetCandidate) {
	t.Helper()
	options := CandidateGenerationOptions{
		DiscoveryHorizon: defaultScannerDiscoveryHorizon,
		SpawnBudget:      defaultScannerSpawnBudget,
	}
	for y := int64(-50); y <= 50; y++ {
		for x := int64(-50); x <= 50; x++ {
			cell := ScanCellCoord{X: x, Y: y}
			candidates, err := GeneratePlanetCandidates(seed, cell, options)
			if err != nil {
				t.Fatalf("GeneratePlanetCandidates(%+v) error = %v", cell, err)
			}
			if len(candidates) > 0 {
				return cell, candidates[0]
			}
		}
	}
	t.Fatal("no scanner test candidate found")
	return ScanCellCoord{}, PlanetCandidate{}
}

func scannerTestTime() time.Time {
	return time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC)
}

func hasScannerEvent(events []ScannerEventRecord, eventType ScannerEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func assertNoScannerHiddenLeak(t *testing.T, result ResolveScanPulseResult, candidate PlanetCandidate) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal(ResolveScanPulseResult) error = %v, want nil", err)
	}
	payload := string(data)
	for _, leaked := range []string{
		"candidate",
		"key",
		"position",
		"coordinates",
		"target_player_id",
		"witness",
		"witness_expires_at",
		"witness_expiry",
		"hidden",
		"hidden_target_metadata",
		"scan_roll",
		"scan_candidate",
		"scan_candidates",
		"candidate_data",
		"roll",
		"seed",
		"cell",
		"chunk",
		"signature",
		"min_radar",
		"level",
		"rarity",
		strconv.FormatUint(candidate.Key(), 10),
		strconv.FormatUint(candidate.Key(), 16),
		strconv.FormatFloat(candidate.Position().X, 'f', -1, 64),
		strconv.FormatFloat(candidate.Position().Y, 'f', -1, 64),
		`"x"`,
		`"y"`,
	} {
		if leaked != "" && strings.Contains(payload, leaked) {
			t.Fatalf("scanner result JSON %s leaked %q", payload, leaked)
		}
	}
}

type fixedScannerClock struct {
	now time.Time
}

func (clock fixedScannerClock) Now() time.Time {
	return clock.now
}

type fixedScannerModuleProvider struct {
	equipped bool
}

func (provider fixedScannerModuleProvider) HasEquippedScannerModule(ScannerModuleInput) (bool, error) {
	return provider.equipped, nil
}

type fixedScannerStatsProvider struct {
	snapshot stats.StatSnapshot
}

func (provider fixedScannerStatsProvider) ScanStats(ScannerStatsInput) (stats.StatSnapshot, error) {
	return provider.snapshot, nil
}

type fixedScannerPositionProvider struct {
	position ScannerPosition
}

func (provider fixedScannerPositionProvider) PlayerScanPosition(ScannerPositionInput) (ScannerPosition, error) {
	return provider.position, nil
}

type fixedScannerEnergyProvider struct {
	accepted bool
}

func (provider fixedScannerEnergyProvider) CheckScanEnergy(input ScannerEnergyInput) (ScannerEnergyResult, error) {
	if err := input.Validate(); err != nil {
		return ScannerEnergyResult{}, err
	}
	return ScannerEnergyResult{Accepted: provider.accepted}, nil
}

type fixedScannerCooldownProvider struct {
	accepted bool
	readyAt  time.Time
}

func (provider fixedScannerCooldownProvider) StartScanCooldown(ScannerCooldownInput) (ScannerCooldownResult, error) {
	return ScannerCooldownResult{
		Accepted: provider.accepted,
		ReadyAt:  provider.readyAt,
	}, nil
}

type recordingScannerCooldownProvider struct {
	accepted bool
	readyAt  time.Time
	calls    int
	inputs   []ScannerCooldownInput
}

func (provider *recordingScannerCooldownProvider) StartScanCooldown(input ScannerCooldownInput) (ScannerCooldownResult, error) {
	provider.calls++
	provider.inputs = append(provider.inputs, input)
	return ScannerCooldownResult{
		Accepted: provider.accepted,
		ReadyAt:  provider.readyAt,
	}, nil
}

type recordingScannerEnergyProvider struct {
	accepted bool
	calls    int
	inputs   []ScannerEnergyInput
}

func (provider *recordingScannerEnergyProvider) CheckScanEnergy(input ScannerEnergyInput) (ScannerEnergyResult, error) {
	if err := input.Validate(); err != nil {
		return ScannerEnergyResult{}, err
	}
	provider.calls++
	provider.inputs = append(provider.inputs, input)
	return ScannerEnergyResult{Accepted: provider.accepted}, nil
}

type recordingScannerPlayerRevealProvider struct {
	revealed bool
	calls    int
	inputs   []ScannerPlayerRevealInput
}

func (provider *recordingScannerPlayerRevealProvider) RevealHiddenPlayer(input ScannerPlayerRevealInput) (ScannerPlayerRevealResult, error) {
	if err := input.Validate(); err != nil {
		return ScannerPlayerRevealResult{}, err
	}
	provider.calls++
	provider.inputs = append(provider.inputs, input)
	return ScannerPlayerRevealResult{Revealed: provider.revealed}, nil
}

type recordingScanXPProvider struct {
	calls []ScanXPGrantInput
	seen  map[progression.XPIdempotencyKey]struct{}
}

func (provider *recordingScanXPProvider) GrantScanXP(input ScanXPGrantInput) (ScanXPGrantResult, error) {
	if err := input.Validate(); err != nil {
		return ScanXPGrantResult{}, err
	}
	if provider.seen == nil {
		provider.seen = make(map[progression.XPIdempotencyKey]struct{})
	}
	if _, ok := provider.seen[input.IdempotencyKey]; ok {
		return ScanXPGrantResult{Duplicate: true}, nil
	}
	provider.seen[input.IdempotencyKey] = struct{}{}
	provider.calls = append(provider.calls, input)
	return ScanXPGrantResult{}, nil
}

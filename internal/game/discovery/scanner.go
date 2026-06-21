package discovery

import (
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// NewScannerService returns a scanner discovery service backed by InMemoryStore.
func NewScannerService(config ScannerServiceConfig) (*ScannerService, error) {
	normalized, err := normalizeScannerConfig(config)
	if err != nil {
		return nil, err
	}
	return &ScannerService{
		store:             normalized.Store,
		seed:              normalized.WorldSeed,
		clock:             normalized.Clock,
		modules:           normalized.Modules,
		stats:             normalized.Stats,
		positions:         normalized.Positions,
		cooldowns:         normalized.Cooldowns,
		energy:            normalized.Energy,
		reveals:           normalized.Reveals,
		xp:                normalized.XP,
		candidateOptions:  normalized.CandidateOptions,
		scanCellSize:      normalized.ScanCellSize,
		chunkSize:         normalized.ChunkSize,
		radarLevelUnit:    normalized.RadarLevelUnit,
		discoveryXPAmount: normalized.DiscoveryXPAmount,
		pulses:            make(map[ScanPulseReference]scanPulse),
		results:           make(map[ScanPulseReference]ResolveScanPulseResult),
	}, nil
}

// StartScanPulse validates scanner prerequisites and records a server-owned pulse.
func (service *ScannerService) StartScanPulse(input StartScanPulseInput) (StartScanPulseResult, error) {
	if err := input.Validate(); err != nil {
		return StartScanPulseResult{}, err
	}

	service.mu.Lock()
	if existing, ok := service.pulses[input.PulseReference]; ok {
		service.mu.Unlock()
		if !scanPulseMatchesStartInput(existing, input) {
			return StartScanPulseResult{}, ErrScanPulseNotFound
		}
		return StartScanPulseResult{
			PulseReference: existing.reference,
			Status:         ScanPulseStatusStarted,
			ResolveAfter:   existing.resolveAfter,
		}, nil
	}
	service.mu.Unlock()

	equipped, err := service.modules.HasEquippedScannerModule(ScannerModuleInput{
		PlayerID: input.PlayerID,
		ShipID:   input.ShipID,
	})
	if err != nil {
		return StartScanPulseResult{}, err
	}
	if !equipped {
		return StartScanPulseResult{}, ErrScannerUnavailable
	}

	position, err := service.positions.PlayerScanPosition(ScannerPositionInput{
		PlayerID: input.PlayerID,
		WorldID:  input.WorldID,
		ZoneID:   input.ZoneID,
	})
	if err != nil {
		return StartScanPulseResult{}, err
	}
	if err := position.ValidateFor(input.PlayerID, input.WorldID, input.ZoneID); err != nil {
		return StartScanPulseResult{}, err
	}

	snapshot, err := service.stats.ScanStats(ScannerStatsInput{
		PlayerID: input.PlayerID,
		ShipID:   input.ShipID,
	})
	if err != nil {
		return StartScanPulseResult{}, err
	}
	effective, err := validateScannerSnapshot(snapshot, input.PlayerID, input.ShipID)
	if err != nil {
		return StartScanPulseResult{}, err
	}

	if err := position.ValidateStationaryForScan(); err != nil {
		return StartScanPulseResult{}, err
	}

	now := service.clock.Now().UTC()
	cooldownDuration := scannerCooldownDuration(effective)
	cell, err := ScanCellCoordForPosition(position.Position, service.scanCellSize)
	if err != nil {
		return StartScanPulseResult{}, err
	}

	energy, err := service.energy.CheckScanEnergy(ScannerEnergyInput{
		PlayerID:       input.PlayerID,
		ShipID:         input.ShipID,
		WorldID:        input.WorldID,
		ZoneID:         input.ZoneID,
		PulseReference: input.PulseReference,
		CheckedAt:      now,
		Stats:          effective,
	})
	if err != nil {
		return StartScanPulseResult{}, err
	}
	if !energy.Accepted {
		return StartScanPulseResult{}, ErrScannerEnergyUnavailable
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if existing, ok := service.pulses[input.PulseReference]; ok {
		if !scanPulseMatchesStartInput(existing, input) {
			return StartScanPulseResult{}, ErrScanPulseNotFound
		}
		return StartScanPulseResult{
			PulseReference: existing.reference,
			Status:         ScanPulseStatusStarted,
			ResolveAfter:   existing.resolveAfter,
		}, nil
	}

	cooldown, err := service.cooldowns.StartScanCooldown(ScannerCooldownInput{
		PlayerID:       input.PlayerID,
		ShipID:         input.ShipID,
		WorldID:        input.WorldID,
		ZoneID:         input.ZoneID,
		PulseReference: input.PulseReference,
		StartedAt:      now,
		Duration:       cooldownDuration,
	})
	if err != nil {
		return StartScanPulseResult{}, err
	}
	if !cooldown.Accepted {
		return StartScanPulseResult{}, ErrScanCooldownActive
	}

	resolveAfter := cooldown.ReadyAt.UTC()
	if resolveAfter.IsZero() {
		resolveAfter = now.Add(cooldownDuration).UTC()
	}

	pulse := scanPulse{
		reference:    input.PulseReference,
		playerID:     input.PlayerID,
		worldID:      input.WorldID,
		zoneID:       input.ZoneID,
		shipID:       input.ShipID,
		position:     position.Position,
		cell:         cell,
		stats:        effective,
		startedAt:    now,
		resolveAfter: resolveAfter,
	}

	service.pulses[input.PulseReference] = pulse
	service.appendEventLocked(newScannerEvent(ScannerEventPulseStarted, pulse, "", now))

	return StartScanPulseResult{
		PulseReference: input.PulseReference,
		Status:         ScanPulseStatusStarted,
		ResolveAfter:   resolveAfter,
	}, nil
}

// ResolveScanPulse resolves one local server-owned pulse. Duplicate resolves
// return the first result without repeating planet, intel, XP, or event writes.
func (service *ScannerService) ResolveScanPulse(input ResolveScanPulseInput) (ResolveScanPulseResult, error) {
	if err := input.Validate(); err != nil {
		return ResolveScanPulseResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	pulse, ok := service.pulses[input.PulseReference]
	if !ok || !scanPulseMatchesResolveInput(pulse, input) {
		return ResolveScanPulseResult{}, ErrScanPulseNotFound
	}

	if result, ok := service.results[input.PulseReference]; ok {
		duplicate := cloneResolveScanPulseResult(result)
		duplicate.Duplicate = true
		duplicate.XPGranted = false
		return duplicate, nil
	}

	now := service.clock.Now().UTC()
	if now.Before(pulse.resolveAfter) {
		return ResolveScanPulseResult{}, ErrScanPulseNotReady
	}

	result, err := service.resolvePulseLocked(pulse, now)
	if err != nil {
		return ResolveScanPulseResult{}, err
	}
	service.results[input.PulseReference] = cloneResolveScanPulseResult(result)
	return result, nil
}

// Events returns scanner event records in append order.
func (service *ScannerService) Events() []ScannerEventRecord {
	service.mu.Lock()
	defer service.mu.Unlock()

	events := make([]ScannerEventRecord, len(service.events))
	copy(events, service.events)
	return events
}

func (service *ScannerService) resolvePulseLocked(pulse scanPulse, now time.Time) (ResolveScanPulseResult, error) {
	if service.reveals != nil {
		reveal, err := service.reveals.RevealHiddenPlayer(ScannerPlayerRevealInput{
			PlayerID:       pulse.playerID,
			ShipID:         pulse.shipID,
			WorldID:        pulse.worldID,
			ZoneID:         pulse.zoneID,
			PulseReference: pulse.reference,
			Position:       pulse.position,
			Stats:          pulse.stats,
			RevealedAt:     now,
		})
		if err != nil {
			return ResolveScanPulseResult{}, err
		}
		if reveal.Revealed {
			service.appendEventLocked(newScannerEvent(ScannerEventPulseResolved, pulse, "", now))
			service.appendEventLocked(newScannerEvent(ScannerEventPlayerRevealed, pulse, "", now))
			return ResolveScanPulseResult{
				PulseReference: pulse.reference,
				Status:         ScanPulseStatusPlayerRevealed,
				Message:        "Scanner revealed a radar contact.",
			}, nil
		}
		if reveal.NoSignal {
			service.appendEventLocked(newScannerEvent(ScannerEventPulseResolved, pulse, "", now))
			return ResolveScanPulseResult{
				PulseReference: pulse.reference,
				Status:         ScanPulseStatusNoSignal,
				Message:        "No valid signal found.",
			}, nil
		}
	}

	candidates, err := GeneratePlanetCandidates(service.seed, pulse.cell, service.candidateOptionsForPulse(pulse))
	if err != nil {
		return ResolveScanPulseResult{}, err
	}

	candidate, ok, err := service.detectCandidate(pulse, candidates)
	if err != nil {
		return ResolveScanPulseResult{}, err
	}
	if !ok {
		service.appendEventLocked(newScannerEvent(ScannerEventPulseResolved, pulse, "", now))
		return ResolveScanPulseResult{
			PulseReference: pulse.reference,
			Status:         ScanPulseStatusNoSignal,
			Message:        "No valid signal found.",
		}, nil
	}

	materializationKey := scannerMaterializationKey(pulse.worldID, pulse.zoneID, pulse.cell, candidate)
	planet := service.planetFromCandidate(pulse, candidate, materializationKey, now)
	materialized, err := service.store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: materializationKey,
		Planet:       planet,
	})
	if err != nil {
		return ResolveScanPulseResult{}, err
	}

	intel := PlayerPlanetIntel{
		PlayerID:        pulse.playerID,
		PlanetID:        materialized.Planet.ID,
		WorldID:         materialized.Planet.WorldID,
		ZoneID:          materialized.Planet.ZoneID,
		Coordinates:     materialized.Planet.Coordinates,
		State:           IntelStateFresh,
		Confidence:      scannerIntelConfidence(candidate),
		LastSeenAt:      now,
		SourceType:      IntelSourceScanSuccess,
		SourceReference: string(pulse.reference),
	}
	if _, _, err := service.store.UpsertPlayerPlanetIntel(intel); err != nil {
		return ResolveScanPulseResult{}, err
	}

	xpResult, err := service.grantDiscoveryXP(pulse.playerID, materialized.Planet.ID)
	if err != nil {
		return ResolveScanPulseResult{}, err
	}

	service.appendEventLocked(newScannerEvent(ScannerEventPulseResolved, pulse, "", now))
	service.appendEventLocked(newScannerEvent(ScannerEventPlanetDiscovered, pulse, materialized.Planet.ID, now))
	signal := candidate.ClientSafeSignal()
	return ResolveScanPulseResult{
		PulseReference: pulse.reference,
		Status:         ScanPulseStatusPlanetDiscovered,
		Signal:         &signal,
		PlanetID:       materialized.Planet.ID,
		XPGranted:      !xpResult.Duplicate,
	}, nil
}

func (service *ScannerService) detectCandidate(pulse scanPulse, candidates []PlanetCandidate) (PlanetCandidate, bool, error) {
	radarLevel := scannerRadarLevel(pulse.stats, service.radarLevelUnit)
	for _, candidate := range candidates {
		if radarLevel < candidate.MinRadarLevel() {
			continue
		}
		distance := pulse.position.Distance(candidate.Position())
		if pulse.stats.Exploration.ScanRadius <= 0 || distance > pulse.stats.Exploration.ScanRadius {
			continue
		}
		chance := scannerDetectionChance(pulse.stats, candidate, distance)
		if chance <= 0 {
			continue
		}
		roll, err := scannerDetectionRoll(service.seed, pulse, candidate)
		if err != nil {
			return PlanetCandidate{}, false, err
		}
		if roll <= chance {
			return candidate, true, nil
		}
	}
	return PlanetCandidate{}, false, nil
}

func (service *ScannerService) planetFromCandidate(
	pulse scanPulse,
	candidate PlanetCandidate,
	materializationKey PlanetMaterializationKey,
	discoveredAt time.Time,
) Planet {
	return Planet{
		ID:           scannerPlanetID(materializationKey),
		WorldID:      pulse.worldID,
		ZoneID:       pulse.zoneID,
		Coordinates:  candidate.Position(),
		Biome:        scannerPlanetBiome(candidate.Biome()),
		Type:         scannerPlanetType(materializationKey),
		Rarity:       scannerPlanetRarity(candidate.Rarity()),
		Level:        candidate.Level(),
		DiscoveredAt: discoveredAt,
		DiscoveredBy: pulse.playerID,
	}
}

func (service *ScannerService) grantDiscoveryXP(playerID foundation.PlayerID, planetID foundation.PlanetID) (ScanXPGrantResult, error) {
	input := ScanXPGrantInput{
		PlayerID:       playerID,
		Amount:         service.discoveryXPAmount,
		SourceType:     progression.XPSourceTypeScan,
		SourceID:       progression.XPSourceID("planet_discovery:" + planetID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("scan_xp:" + playerID.String() + ":" + planetID.String()),
		Authority:      progression.XPGrantAuthorityScannerService,
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeScout, Amount: service.discoveryXPAmount},
		},
	}
	if err := input.Validate(); err != nil {
		return ScanXPGrantResult{}, err
	}
	return service.xp.GrantScanXP(input)
}

func (service *ScannerService) appendEventLocked(event ScannerEventRecord) {
	service.events = append(service.events, event)
}

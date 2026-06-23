package discovery

import "fmt"

// ClaimDurableLifecyclePlan returns the validated durable row bundle for a
// completed claim reference when this process has observed every lifecycle step.
func (service *ClaimService) ClaimDurableLifecyclePlan(
	reference PlanetClaimReference,
) (ClaimDurableLifecyclePlan, bool, error) {
	if service == nil {
		return ClaimDurableLifecyclePlan{}, false, ErrInvalidClaimConfig
	}
	if err := reference.Validate(); err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}

	beginPlan, ok, err := service.claimDurableBeginPlan(reference)
	if err != nil || !ok {
		return ClaimDurableLifecyclePlan{}, ok, err
	}
	boundary, ok, err := service.claimBoundaries.ClaimBoundary(reference)
	if err != nil || !ok || boundary.Status != ClaimBoundaryStatusComplete {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	referenceRecord, ok, err := service.ClaimReference(reference)
	if err != nil || !ok {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	event, ok := service.claimEventForLifecycle(reference, boundary.EventID.String())
	if !ok {
		return ClaimDurableLifecyclePlan{}, false, nil
	}
	outbox, ok := service.claimOutboxForLifecycle(reference, boundary.EventID.String())
	if !ok {
		return ClaimDurableLifecyclePlan{}, false, nil
	}
	commitPlan, err := NewClaimDurableCommitPlan(&boundary, &referenceRecord, &event, &outbox, &beginPlan.XCoreConsumption)
	if err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}

	var productionInit *ClaimProductionInitializationDurablePlan
	initRecord, hasInit, err := service.claimProductionInitialization(reference)
	if err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	if hasInit {
		initPlan, err := initRecord.DurablePlan(&boundary)
		if err != nil {
			return ClaimDurableLifecyclePlan{}, false, err
		}
		productionInit = &initPlan
	}
	plan, err := NewClaimDurableLifecyclePlan(&beginPlan, productionInit, &commitPlan)
	if err != nil {
		return ClaimDurableLifecyclePlan{}, false, err
	}
	return plan, true, nil
}

func (service *ClaimService) recordClaimDurableBeginPlanLocked(
	reference PlanetClaimReference,
	plan ClaimDurableBeginPlan,
) {
	if service.claimDurableBegins == nil {
		service.claimDurableBegins = make(map[PlanetClaimReference]ClaimDurableBeginPlan)
	}
	service.claimDurableBegins[reference] = cloneClaimDurableBeginPlan(plan)
}

func (service *ClaimService) claimDurableBeginPlan(
	reference PlanetClaimReference,
) (ClaimDurableBeginPlan, bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	plan, ok := service.claimDurableBegins[reference]
	if !ok {
		return ClaimDurableBeginPlan{}, false, nil
	}
	cloned := cloneClaimDurableBeginPlan(plan)
	if err := validateClaimDurableBeginPlan(cloned); err != nil {
		return ClaimDurableBeginPlan{}, false, fmt.Errorf("claim_durable_begin: %w", err)
	}
	return cloned, true, nil
}

func (service *ClaimService) claimProductionInitialization(
	reference PlanetClaimReference,
) (ClaimProductionInitializationRecord, bool, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	record, ok := service.productionInitializations[reference]
	if !ok {
		return ClaimProductionInitializationRecord{}, false, nil
	}
	cloned := cloneClaimProductionInitializationRecord(record)
	if err := validateClaimProductionInitializationDurableRecord(cloned); err != nil {
		return ClaimProductionInitializationRecord{}, false, fmt.Errorf("claim_production_initialization: %w", err)
	}
	return cloned, true, nil
}

func (service *ClaimService) claimEventForLifecycle(
	reference PlanetClaimReference,
	eventID string,
) (ClaimEventRecord, bool) {
	for _, event := range service.Events() {
		if event.ClaimReference == reference && event.EventID.String() == eventID {
			return cloneClaimEventRecord(event), true
		}
	}
	return ClaimEventRecord{}, false
}

func (service *ClaimService) claimOutboxForLifecycle(
	reference PlanetClaimReference,
	eventID string,
) (ClaimOutboxRecord, bool) {
	for _, outbox := range service.ClaimOutboxRecords() {
		if outbox.ClaimReference == reference && outbox.Event.EventID.String() == eventID {
			return cloneClaimOutboxRecord(outbox), true
		}
	}
	return ClaimOutboxRecord{}, false
}

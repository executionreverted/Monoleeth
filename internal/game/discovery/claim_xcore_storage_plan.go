package discovery

import (
	"fmt"

	"gameproject/internal/game/economy"
)

// ClaimXCoreStorageMutationPlan validates the inventory rows a future durable
// claim/storage transaction must couple to the X Core claim-debit evidence.
type ClaimXCoreStorageMutationPlan struct {
	Input       ClaimXCoreConsumeInput
	Result      ClaimXCoreConsumeResult
	Consumption ClaimXCoreConsumptionRecord
	Boundary    ClaimBoundaryRecord
	HasBoundary bool
}

// NewClaimXCoreStorageMutationPlan validates one X Core storage mutation row
// bundle. A boundary may be omitted for debit-only owner-begin failure recovery.
func NewClaimXCoreStorageMutationPlan(
	input ClaimXCoreConsumeInput,
	result ClaimXCoreConsumeResult,
	consumption ClaimXCoreConsumptionRecord,
	boundary *ClaimBoundaryRecord,
) (ClaimXCoreStorageMutationPlan, error) {
	if err := validateClaimXCoreStorageMutationInput(input, consumption); err != nil {
		return ClaimXCoreStorageMutationPlan{}, err
	}
	clonedResult := cloneClaimXCoreConsumeResult(result)
	if err := validateClaimXCoreStorageMutationResult(input, consumption, clonedResult); err != nil {
		return ClaimXCoreStorageMutationPlan{}, err
	}

	plan := ClaimXCoreStorageMutationPlan{
		Input:       input,
		Result:      clonedResult,
		Consumption: cloneClaimXCoreConsumptionRecord(consumption),
	}
	if boundary != nil {
		clonedBoundary := cloneClaimBoundaryRecord(*boundary)
		if clonedBoundary.ClaimReference == "" {
			return ClaimXCoreStorageMutationPlan{}, fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
		}
		if err := validateClaimXCoreStorageMutationBoundary(input, clonedBoundary); err != nil {
			return ClaimXCoreStorageMutationPlan{}, err
		}
		plan.Boundary = clonedBoundary
		plan.HasBoundary = true
	}
	return plan, nil
}

func validateClaimXCoreStorageMutationPlan(plan ClaimXCoreStorageMutationPlan) error {
	if err := validateClaimXCoreStorageMutationInput(plan.Input, plan.Consumption); err != nil {
		return err
	}
	if err := validateClaimXCoreStorageMutationResult(plan.Input, plan.Consumption, plan.Result); err != nil {
		return err
	}
	if plan.HasBoundary {
		if plan.Boundary.ClaimReference == "" {
			return fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
		}
		return validateClaimXCoreStorageMutationBoundary(plan.Input, plan.Boundary)
	}
	if plan.Boundary.ClaimReference != "" {
		return fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimXCoreStorageMutationInput(input ClaimXCoreConsumeInput, consumption ClaimXCoreConsumptionRecord) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if err := validateClaimDurableBeginXCore(consumption); err != nil {
		return err
	}
	expectedKey, ok := input.Reference.IdempotencyKey(input.PlayerID, input.PlanetID)
	if !ok {
		return fmt.Errorf("x_core_storage.reference_key: %w", ErrInvalidClaimDurableCommit)
	}
	if consumption.ClaimReference != input.Reference ||
		consumption.ReferenceKey != expectedKey ||
		consumption.PlayerID != input.PlayerID ||
		consumption.PlanetID != input.PlanetID ||
		consumption.SourceLocation != input.SourceLocation ||
		consumption.Quantity != input.Quantity ||
		consumption.Reason != input.Reason {
		return fmt.Errorf("x_core_storage.consumption: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimXCoreStorageMutationResult(
	input ClaimXCoreConsumeInput,
	consumption ClaimXCoreConsumptionRecord,
	result ClaimXCoreConsumeResult,
) error {
	if result.Duplicate != consumption.Duplicate || result.StorageMutation.Duplicate != result.Duplicate {
		return fmt.Errorf("x_core_storage.duplicate: %w", ErrInvalidClaimDurableCommit)
	}
	if len(result.StorageMutation.LedgerEntries) != 1 {
		return fmt.Errorf("x_core_storage.ledger_count: %w", ErrInvalidClaimDurableCommit)
	}
	if err := validateClaimXCoreStorageMutationLedger(input, consumption, result.StorageMutation.LedgerEntries[0]); err != nil {
		return err
	}
	for index, row := range result.StorageMutation.StackableItems {
		if err := validateClaimXCoreStorageMutationStackable(input, row); err != nil {
			return fmt.Errorf("x_core_storage.stackable[%d]: %w", index, err)
		}
	}
	for index, row := range result.StorageMutation.InstanceItems {
		if err := validateClaimXCoreStorageMutationInstance(input, row); err != nil {
			return fmt.Errorf("x_core_storage.instance[%d]: %w", index, err)
		}
	}
	return nil
}

func validateClaimXCoreStorageMutationLedger(
	input ClaimXCoreConsumeInput,
	consumption ClaimXCoreConsumptionRecord,
	entry economy.ItemLedgerEntry,
) error {
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("x_core_storage.ledger: %w", err)
	}
	if entry.Action != economy.LedgerActionDecrease ||
		entry.PlayerID != input.PlayerID ||
		entry.ItemID != input.ItemRef.Definition.ItemID ||
		entry.Quantity.Int64() != input.Quantity ||
		entry.Location != input.SourceLocation ||
		entry.Reason != input.Reason ||
		entry.ReferenceKey != consumption.ReferenceKey {
		return fmt.Errorf("x_core_storage.ledger: %w", ErrInvalidClaimDurableCommit)
	}
	if !input.ItemRef.ItemInstanceID.IsZero() && entry.ItemInstanceID != input.ItemRef.ItemInstanceID {
		return fmt.Errorf("x_core_storage.ledger_instance: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func validateClaimXCoreStorageMutationStackable(input ClaimXCoreConsumeInput, row economy.StackableItem) error {
	if err := row.Validate(); err != nil {
		return err
	}
	if row.OwnerPlayerID != input.PlayerID ||
		row.ItemID != input.ItemRef.Definition.ItemID ||
		row.Location != input.SourceLocation {
		return ErrInvalidClaimDurableCommit
	}
	return nil
}

func validateClaimXCoreStorageMutationInstance(input ClaimXCoreConsumeInput, row economy.InstanceItem) error {
	if err := row.Validate(); err != nil {
		return err
	}
	if row.OwnerPlayerID != input.PlayerID ||
		row.ItemID != input.ItemRef.Definition.ItemID ||
		row.Location != input.SourceLocation {
		return ErrInvalidClaimDurableCommit
	}
	if !input.ItemRef.ItemInstanceID.IsZero() && row.ItemInstanceID != input.ItemRef.ItemInstanceID {
		return ErrInvalidClaimDurableCommit
	}
	return nil
}

func validateClaimXCoreStorageMutationBoundary(input ClaimXCoreConsumeInput, boundary ClaimBoundaryRecord) error {
	if err := validateClaimDurableBeginBoundary(boundary); err != nil {
		return err
	}
	if boundary.ClaimReference != input.Reference ||
		boundary.PlayerID != input.PlayerID ||
		boundary.PlanetID != input.PlanetID {
		return fmt.Errorf("x_core_storage.boundary: %w", ErrInvalidClaimDurableCommit)
	}
	return nil
}

func cloneClaimXCoreConsumeResult(result ClaimXCoreConsumeResult) ClaimXCoreConsumeResult {
	result.StorageMutation.StackableItems = append([]economy.StackableItem(nil), result.StorageMutation.StackableItems...)
	result.StorageMutation.InstanceItems = append([]economy.InstanceItem(nil), result.StorageMutation.InstanceItems...)
	result.StorageMutation.LedgerEntries = append([]economy.ItemLedgerEntry(nil), result.StorageMutation.LedgerEntries...)
	return result
}

func cloneClaimXCoreStorageMutationPlan(plan ClaimXCoreStorageMutationPlan) ClaimXCoreStorageMutationPlan {
	plan.Result = cloneClaimXCoreConsumeResult(plan.Result)
	plan.Consumption = cloneClaimXCoreConsumptionRecord(plan.Consumption)
	plan.Boundary = cloneClaimBoundaryRecord(plan.Boundary)
	return plan
}

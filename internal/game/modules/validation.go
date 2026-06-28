package modules

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	maxOffensiveModuleSlots = 4
	maxDefensiveModuleSlots = 3
	maxUtilityModuleSlots   = 4
)

// Validate reports whether slotType is supported.
func (slotType ModuleSlotType) Validate() error {
	switch slotType {
	case ModuleSlotTypeOffensive, ModuleSlotTypeDefensive, ModuleSlotTypeUtility:
		return nil
	default:
		return fmt.Errorf("module slot type %q: %w", slotType, ErrInvalidModuleSlotType)
	}
}

// Validate reports whether slotID is supported.
func (slotID ModuleSlotID) Validate() error {
	_, _, err := slotID.SlotTypeAndOrdinal()
	return err
}

// SlotType returns the slot type required by slotID.
func (slotID ModuleSlotID) SlotType() (ModuleSlotType, error) {
	slotType, _, err := slotID.SlotTypeAndOrdinal()
	return slotType, err
}

// SlotTypeAndOrdinal returns the family and 1-based index encoded in slotID.
func (slotID ModuleSlotID) SlotTypeAndOrdinal() (ModuleSlotType, int, error) {
	raw := string(slotID)
	prefix, ordinalText, ok := strings.Cut(raw, "_")
	if !ok || strings.TrimSpace(prefix) == "" || strings.TrimSpace(ordinalText) == "" {
		return "", 0, fmt.Errorf("module slot id %q: %w", slotID, ErrInvalidModuleSlotID)
	}
	ordinal, err := strconv.Atoi(ordinalText)
	if err != nil || ordinal <= 0 {
		return "", 0, fmt.Errorf("module slot id %q: %w", slotID, ErrInvalidModuleSlotID)
	}

	slotType := ModuleSlotType(prefix)
	if err := slotType.Validate(); err != nil {
		return "", 0, fmt.Errorf("module slot id %q: %w", slotID, ErrInvalidModuleSlotID)
	}
	maxSlots, err := maxSlotsForType(slotType)
	if err != nil {
		return "", 0, err
	}
	if ordinal > maxSlots {
		return "", 0, fmt.Errorf("module slot id %q max %d: %w", slotID, maxSlots, ErrInvalidModuleSlotID)
	}
	return slotType, ordinal, nil
}

// Validate reports whether category is supported.
func (category ModuleCategory) Validate() error {
	switch category {
	case ModuleCategoryOffensive, ModuleCategoryDefensive, ModuleCategoryUtility:
		return nil
	default:
		return fmt.Errorf("module category %q: %w", category, ErrInvalidModuleCategory)
	}
}

// SlotType returns the default slot type compatible with this category.
func (category ModuleCategory) SlotType() (ModuleSlotType, error) {
	if err := category.Validate(); err != nil {
		return "", err
	}
	switch category {
	case ModuleCategoryOffensive:
		return ModuleSlotTypeOffensive, nil
	case ModuleCategoryDefensive:
		return ModuleSlotTypeDefensive, nil
	case ModuleCategoryUtility:
		return ModuleSlotTypeUtility, nil
	default:
		return "", fmt.Errorf("module category %q: %w", category, ErrInvalidModuleCategory)
	}
}

// Validate reports whether role is supported.
func (role PilotRole) Validate() error {
	switch role {
	case PilotRoleCombat, PilotRoleScout, PilotRoleCrafting, PilotRoleConstruction:
		return nil
	default:
		return fmt.Errorf("pilot role %q: %w", role, ErrInvalidPilotRole)
	}
}

// Validate reports whether stat is supported by the module catalog model.
func (stat StatKey) Validate() error {
	switch stat {
	case StatWeaponDamage,
		StatWeaponRange,
		StatAccuracy,
		StatSpeed,
		StatShieldMax,
		StatShieldRegen,
		StatScanPower,
		StatScanRadius,
		StatRadarRange,
		StatDetectionPower,
		StatJammerResistance,
		StatStealthDetectionBonus,
		StatCargoCapacity:
		return nil
	default:
		return fmt.Errorf("stat key %q: %w", stat, ErrInvalidStatKey)
	}
}

// Validate reports whether kind is supported.
func (kind StatModifierKind) Validate() error {
	switch kind {
	case StatModifierFlat, StatModifierPercent:
		return nil
	default:
		return fmt.Errorf("stat modifier kind %q: %w", kind, ErrInvalidStatModifierKind)
	}
}

// Validate reports whether key is supported.
func (key CooldownKey) Validate() error {
	switch key {
	case CooldownBasicAttack, CooldownScanPulse, CooldownRadarSweep:
		return nil
	default:
		return fmt.Errorf("cooldown key %q: %w", key, ErrInvalidCooldownKey)
	}
}

// Validate reports whether slot has a valid id and type mapping.
func (slot SlotDefinition) Validate() error {
	slotType, err := slot.SlotID.SlotType()
	if err != nil {
		return err
	}
	if err := slot.Type.Validate(); err != nil {
		return err
	}
	if slot.Type != slotType {
		return fmt.Errorf("slot id %q type %q: %w", slot.SlotID, slot.Type, ErrSlotCategoryMismatch)
	}
	return nil
}

// Validate reports whether requirement has a supported role and positive level.
func (requirement RoleRequirement) Validate() error {
	if err := requirement.Role.Validate(); err != nil {
		return err
	}
	if requirement.Level <= 0 {
		return fmt.Errorf("required role level %d: %w", requirement.Level, ErrInvalidRequiredRoleLevel)
	}
	return nil
}

// Validate reports whether modifier names a known stat, operation, and value.
func (modifier StatModifier) Validate() error {
	if err := modifier.Stat.Validate(); err != nil {
		return err
	}
	if err := modifier.Kind.Validate(); err != nil {
		return err
	}
	if modifier.Value == 0 {
		return fmt.Errorf("stat modifier value %d: %w", modifier.Value, ErrInvalidStatModifierValue)
	}
	if modifier.Value > maxCatalogNumericValue || modifier.Value < -maxCatalogNumericValue {
		return fmt.Errorf("stat modifier value %d exceeds max %d: %w", modifier.Value, maxCatalogNumericValue, ErrInvalidStatModifierValue)
	}
	if modifier.Kind == StatModifierPercent && modifier.Value <= -10_000 {
		return fmt.Errorf("percent modifier value %d: %w", modifier.Value, ErrInvalidStatModifierValue)
	}
	return nil
}

// Validate reports whether energy costs are non-negative and bounded.
func (energy EnergyProfile) Validate() error {
	if err := validateNonNegativeBounded("activation energy", energy.ActivationCost, ErrInvalidEnergyValue); err != nil {
		return err
	}
	if err := validateNonNegativeBounded("upkeep energy", energy.Upkeep, ErrInvalidEnergyValue); err != nil {
		return err
	}
	return nil
}

// Validate reports whether cooldown has a known key and positive duration.
func (cooldown Cooldown) Validate() error {
	if err := cooldown.Key.Validate(); err != nil {
		return err
	}
	if cooldown.DurationMS <= 0 {
		return fmt.Errorf("cooldown duration %d: %w", cooldown.DurationMS, ErrInvalidCooldownDuration)
	}
	if cooldown.DurationMS > maxCatalogNumericValue {
		return fmt.Errorf("cooldown duration %d exceeds max %d: %w", cooldown.DurationMS, maxCatalogNumericValue, ErrInvalidCooldownDuration)
	}
	return nil
}

// Validate reports whether durability has a positive max.
func (durability DurabilityProfile) Validate() error {
	if durability.Max <= 0 {
		return fmt.Errorf("durability max %d: %w", durability.Max, ErrInvalidDurabilityMax)
	}
	if durability.Max > maxCatalogNumericValue {
		return fmt.Errorf("durability max %d exceeds max %d: %w", durability.Max, maxCatalogNumericValue, ErrInvalidDurabilityMax)
	}
	return nil
}

// Validate reports whether definition has complete and internally consistent metadata.
func (definition ModuleDefinition) Validate() error {
	if err := definition.Source.Validate(); err != nil {
		return err
	}
	if err := definition.ItemID.Validate(); err != nil {
		return err
	}
	if definition.Source.DefinitionID.String() != definition.ItemID.String() {
		return fmt.Errorf("source %q item %q: %w", definition.Source.DefinitionID, definition.ItemID, ErrModuleSourceMismatch)
	}
	if strings.TrimSpace(definition.Name) == "" {
		return ErrEmptyModuleName
	}
	if err := definition.Category.Validate(); err != nil {
		return err
	}
	if err := definition.SlotType.Validate(); err != nil {
		return err
	}
	if err := definition.validateCategorySlotCompatibility(); err != nil {
		return err
	}
	if definition.Tier <= 0 {
		return fmt.Errorf("module tier %d: %w", definition.Tier, ErrInvalidModuleTier)
	}
	if err := definition.Rarity.Validate(); err != nil {
		return err
	}
	if definition.RequiredRank <= 0 {
		return fmt.Errorf("required rank %d: %w", definition.RequiredRank, ErrInvalidRequiredRank)
	}
	if err := validateRoleRequirements(definition.RequiredRoleLevels); err != nil {
		return err
	}
	if err := validateStatModifiers(definition.StatModifiers); err != nil {
		return err
	}
	if err := definition.Energy.Validate(); err != nil {
		return err
	}
	if err := validateCooldowns(definition.Cooldowns); err != nil {
		return err
	}
	if err := definition.Durability.Validate(); err != nil {
		return err
	}
	for _, flag := range definition.TradeFlags {
		if err := flag.Validate(); err != nil {
			return err
		}
	}
	for _, rule := range definition.BindRules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	if err := definition.validateCompatibleSlotTypes(); err != nil {
		return err
	}
	if err := definition.validateCompatibleCategories(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether equipped module state has valid durable identifiers.
func (equipped EquippedModule) Validate() error {
	if err := equipped.PlayerID.Validate(); err != nil {
		return err
	}
	if err := equipped.ShipID.Validate(); err != nil {
		return err
	}
	if err := equipped.SlotID.Validate(); err != nil {
		return err
	}
	if err := equipped.ItemInstanceID.Validate(); err != nil {
		return err
	}
	if equipped.EquippedAt.IsZero() {
		return ErrZeroEquippedAt
	}
	return nil
}

func (definition ModuleDefinition) validateCategorySlotCompatibility() error {
	wantSlotType, err := definition.Category.SlotType()
	if err != nil {
		return err
	}
	if definition.SlotType != wantSlotType {
		return fmt.Errorf("category %q slot type %q: %w", definition.Category, definition.SlotType, ErrSlotCategoryMismatch)
	}
	return nil
}

func (definition ModuleDefinition) validateCompatibleSlotTypes() error {
	if len(definition.CompatibleSlotTypes) == 0 {
		return fmt.Errorf("module %q compatible slot types: %w", definition.ItemID, ErrInvalidModuleSlotType)
	}
	matchedPrimary := false
	seen := make(map[ModuleSlotType]struct{}, len(definition.CompatibleSlotTypes))
	for _, slotType := range definition.CompatibleSlotTypes {
		if err := slotType.Validate(); err != nil {
			return err
		}
		if _, ok := seen[slotType]; ok {
			return fmt.Errorf("module %q compatible slot type %q: %w", definition.ItemID, slotType, ErrInvalidModuleSlotType)
		}
		seen[slotType] = struct{}{}
		if slotType == definition.SlotType {
			matchedPrimary = true
		}
	}
	if !matchedPrimary {
		return fmt.Errorf("module %q slot type %q absent from compatibility: %w", definition.ItemID, definition.SlotType, ErrSlotCategoryMismatch)
	}
	return nil
}

func (definition ModuleDefinition) validateCompatibleCategories() error {
	if len(definition.CompatibleCategories) == 0 {
		return fmt.Errorf("module %q compatible categories: %w", definition.ItemID, ErrInvalidModuleCategory)
	}
	matchedPrimary := false
	seen := make(map[ModuleCategory]struct{}, len(definition.CompatibleCategories))
	for _, category := range definition.CompatibleCategories {
		if err := category.Validate(); err != nil {
			return err
		}
		if _, ok := seen[category]; ok {
			return fmt.Errorf("module %q compatible category %q: %w", definition.ItemID, category, ErrInvalidModuleCategory)
		}
		seen[category] = struct{}{}
		if category == definition.Category {
			matchedPrimary = true
		}
	}
	if !matchedPrimary {
		return fmt.Errorf("module %q category %q absent from compatibility: %w", definition.ItemID, definition.Category, ErrSlotCategoryMismatch)
	}
	return nil
}

func validateRoleRequirements(requirements []RoleRequirement) error {
	seen := make(map[PilotRole]struct{}, len(requirements))
	for _, requirement := range requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		if _, ok := seen[requirement.Role]; ok {
			return fmt.Errorf("role %q: %w", requirement.Role, ErrDuplicateRoleRequirement)
		}
		seen[requirement.Role] = struct{}{}
	}
	return nil
}

func validateStatModifiers(modifiers []StatModifier) error {
	seen := make(map[string]struct{}, len(modifiers))
	for _, modifier := range modifiers {
		if err := modifier.Validate(); err != nil {
			return err
		}
		key := modifier.Stat.String() + ":" + modifier.Kind.String()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("stat modifier %q: %w", key, ErrDuplicateStatModifier)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateCooldowns(cooldowns []Cooldown) error {
	seen := make(map[CooldownKey]struct{}, len(cooldowns))
	for _, cooldown := range cooldowns {
		if err := cooldown.Validate(); err != nil {
			return err
		}
		if _, ok := seen[cooldown.Key]; ok {
			return fmt.Errorf("cooldown %q: %w", cooldown.Key, ErrDuplicateCooldown)
		}
		seen[cooldown.Key] = struct{}{}
	}
	return nil
}

func validateNonNegativeBounded(name string, value int64, target error) error {
	if value < 0 {
		return fmt.Errorf("%s %d: %w", name, value, target)
	}
	if value > maxCatalogNumericValue {
		return fmt.Errorf("%s %d exceeds max %d: %w", name, value, maxCatalogNumericValue, target)
	}
	return nil
}

func maxSlotsForType(slotType ModuleSlotType) (int, error) {
	switch slotType {
	case ModuleSlotTypeOffensive:
		return maxOffensiveModuleSlots, nil
	case ModuleSlotTypeDefensive:
		return maxDefensiveModuleSlots, nil
	case ModuleSlotTypeUtility:
		return maxUtilityModuleSlots, nil
	default:
		return 0, fmt.Errorf("module slot type %q: %w", slotType, ErrInvalidModuleSlotType)
	}
}

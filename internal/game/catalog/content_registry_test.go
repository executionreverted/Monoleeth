package catalog

import (
	"errors"
	"testing"
)

func TestContentRegistryValidatesShopProductReferencesAndDisplayNames(t *testing.T) {
	registry, err := NewContentRegistry(
		ContentRegistryVersion,
		[]ContentCategory{{ID: "weapons", DisplayName: "Weapons", SortOrder: 10}},
		[]ShopProductDefinition{validShopProductDefinition()},
	)
	if err != nil {
		t.Fatalf("NewContentRegistry() = %v, want nil", err)
	}

	err = registry.ValidateReferences(ReferenceResolver{
		HasModule: func(id string) bool { return id == "laser_alpha_t1" },
	})
	if err != nil {
		t.Fatalf("ValidateReferences() = %v, want nil", err)
	}
}

func TestContentRegistryRejectsRawDisplayNames(t *testing.T) {
	product := validShopProductDefinition()
	product.Display.DisplayName = "laser_alpha_t1"

	_, err := NewContentRegistry(
		ContentRegistryVersion,
		[]ContentCategory{{ID: "weapons", DisplayName: "Weapons", SortOrder: 10}},
		[]ShopProductDefinition{product},
	)
	if !errors.Is(err, ErrInvalidDisplayMetadata) {
		t.Fatalf("raw display error = %v, want ErrInvalidDisplayMetadata", err)
	}
}

func TestContentRegistryRejectsMissingGrantReferences(t *testing.T) {
	registry, err := NewContentRegistry(
		ContentRegistryVersion,
		[]ContentCategory{{ID: "weapons", DisplayName: "Weapons", SortOrder: 10}},
		[]ShopProductDefinition{validShopProductDefinition()},
	)
	if err != nil {
		t.Fatalf("NewContentRegistry() = %v, want nil", err)
	}

	err = registry.ValidateReferences(ReferenceResolver{
		HasModule: func(id string) bool { return false },
	})
	if !errors.Is(err, ErrMissingContentReference) {
		t.Fatalf("missing reference error = %v, want ErrMissingContentReference", err)
	}
}

func TestContentRegistryRejectsDuplicateProducts(t *testing.T) {
	product := validShopProductDefinition()
	_, err := NewContentRegistry(
		ContentRegistryVersion,
		[]ContentCategory{{ID: "weapons", DisplayName: "Weapons", SortOrder: 10}},
		[]ShopProductDefinition{product, product},
	)
	if !errors.Is(err, ErrDuplicateShopProduct) {
		t.Fatalf("duplicate product error = %v, want ErrDuplicateShopProduct", err)
	}
}

func validShopProductDefinition() ShopProductDefinition {
	return ShopProductDefinition{
		ProductID:   "product_module_laser_alpha_t1",
		ProductType: ShopProductTypeModule,
		Display: DisplayMetadata{
			DisplayName: "Prism Lance I",
			Description: "Entry laser array.",
			Category:    "weapons",
			Subcategory: "Laser",
			ArtKey:      "module.prism_lance_1",
			Rarity:      "common",
			Tier:        1,
			SortOrder:   10,
		},
		GrantTarget:  GrantTarget{Kind: GrantTargetKindModule, RefID: "laser_alpha_t1", Quantity: 1},
		Price:        PricePolicy{Currency: PriceCurrencyCredits, Amount: 450, Fixed: true},
		Stock:        StockPolicy{Kind: StockPolicyUnlimited},
		Availability: AvailabilityRule{Available: false, LockedReason: "Purchase locked."},
	}
}

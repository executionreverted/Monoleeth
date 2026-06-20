package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var (
	ErrDuplicateContentCategory = errors.New("duplicate content category")
	ErrDuplicateShopProduct     = errors.New("duplicate shop product")
	ErrInvalidContentCategory   = errors.New("invalid content category")
	ErrInvalidDisplayMetadata   = errors.New("invalid display metadata")
	ErrInvalidShopProduct       = errors.New("invalid shop product")
	ErrMissingContentReference  = errors.New("missing content reference")
)

const ContentRegistryVersion Version = "content_registry_task001_v1"

type ShopProductID string
type ShopProductType string
type GrantTargetKind string
type PriceCurrency string
type StockPolicyKind string

const (
	ShopProductTypeShip    ShopProductType = "ship"
	ShopProductTypeModule  ShopProductType = "module"
	ShopProductTypeItem    ShopProductType = "item"
	ShopProductTypePremium ShopProductType = "premium"
	ShopProductTypeLocked  ShopProductType = "locked"
	GrantTargetKindShip    GrantTargetKind = "ship"
	GrantTargetKindItem    GrantTargetKind = "item"
	GrantTargetKindModule  GrantTargetKind = "module"
	GrantTargetKindPremium GrantTargetKind = "premium"
	GrantTargetKindBlocker GrantTargetKind = "blocker"
	PriceCurrencyCredits   PriceCurrency   = "credits"
	PriceCurrencyPremium   PriceCurrency   = "premium_paid"
	StockPolicyUnlimited   StockPolicyKind = "unlimited"
	StockPolicyLimited     StockPolicyKind = "limited"
	StockPolicyUnavailable StockPolicyKind = "unavailable"
)

// ContentRegistry owns player-facing catalog metadata for the current playtest.
type ContentRegistry struct {
	Version      Version                 `json:"catalog_version"`
	Categories   []ContentCategory       `json:"categories"`
	ShopProducts []ShopProductDefinition `json:"shop_products"`
}

type ContentCategory struct {
	ID          string `json:"category_id"`
	DisplayName string `json:"display_name"`
	SortOrder   int    `json:"sort_order"`
}

type DisplayMetadata struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory,omitempty"`
	ArtKey      string `json:"art_key"`
	Rarity      string `json:"rarity,omitempty"`
	Tier        int    `json:"tier,omitempty"`
	SortOrder   int    `json:"sort_order"`
}

type GrantTarget struct {
	Kind     GrantTargetKind `json:"kind"`
	RefID    string          `json:"ref_id"`
	Quantity int64           `json:"quantity,omitempty"`
}

type PricePolicy struct {
	Currency PriceCurrency `json:"currency_type"`
	Amount   int64         `json:"amount"`
	Fixed    bool          `json:"fixed"`
}

type StockPolicy struct {
	Kind      StockPolicyKind `json:"kind"`
	Remaining int64           `json:"remaining,omitempty"`
	Total     int64           `json:"total,omitempty"`
}

type AvailabilityRule struct {
	Available    bool   `json:"available"`
	LockedReason string `json:"locked_reason,omitempty"`
	RequiredRank int    `json:"required_rank,omitempty"`
}

type ShopProductDefinition struct {
	ProductID    ShopProductID    `json:"product_id"`
	ProductType  ShopProductType  `json:"product_type"`
	Display      DisplayMetadata  `json:"display"`
	GrantTarget  GrantTarget      `json:"grant_target"`
	Price        PricePolicy      `json:"price_policy"`
	Stock        StockPolicy      `json:"stock_policy"`
	Availability AvailabilityRule `json:"availability"`
}

type ReferenceResolver struct {
	HasItem    func(string) bool
	HasModule  func(string) bool
	HasShip    func(string) bool
	HasPremium func(string) bool
}

func NewContentRegistry(version Version, categories []ContentCategory, products []ShopProductDefinition) (ContentRegistry, error) {
	registry := ContentRegistry{
		Version:      version,
		Categories:   cloneContentCategories(categories),
		ShopProducts: cloneShopProducts(products),
	}
	if err := registry.Validate(); err != nil {
		return ContentRegistry{}, err
	}
	return registry, nil
}

func (registry ContentRegistry) Validate() error {
	if err := registry.Version.Validate(); err != nil {
		return err
	}
	if len(registry.Categories) == 0 {
		return fmt.Errorf("categories: %w", ErrInvalidContentCategory)
	}
	categoryIDs := make(map[string]struct{}, len(registry.Categories))
	for _, category := range registry.Categories {
		if err := category.Validate(); err != nil {
			return err
		}
		if _, exists := categoryIDs[category.ID]; exists {
			return fmt.Errorf("category %q: %w", category.ID, ErrDuplicateContentCategory)
		}
		categoryIDs[category.ID] = struct{}{}
	}
	productIDs := make(map[ShopProductID]struct{}, len(registry.ShopProducts))
	for _, product := range registry.ShopProducts {
		if err := product.Validate(categoryIDs); err != nil {
			return err
		}
		if _, exists := productIDs[product.ProductID]; exists {
			return fmt.Errorf("shop product %q: %w", product.ProductID, ErrDuplicateShopProduct)
		}
		productIDs[product.ProductID] = struct{}{}
	}
	return nil
}

func (registry ContentRegistry) ValidateReferences(resolver ReferenceResolver) error {
	if err := registry.Validate(); err != nil {
		return err
	}
	for _, product := range registry.ShopProducts {
		if err := product.GrantTarget.ValidateReference(resolver); err != nil {
			return fmt.Errorf("shop product %q: %w", product.ProductID, err)
		}
	}
	return nil
}

func (registry ContentRegistry) SortedCategories() []ContentCategory {
	categories := cloneContentCategories(registry.Categories)
	sort.SliceStable(categories, func(i, j int) bool {
		if categories[i].SortOrder == categories[j].SortOrder {
			return categories[i].ID < categories[j].ID
		}
		return categories[i].SortOrder < categories[j].SortOrder
	})
	return categories
}

func (registry ContentRegistry) SortedShopProducts() []ShopProductDefinition {
	products := cloneShopProducts(registry.ShopProducts)
	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Display.SortOrder == products[j].Display.SortOrder {
			return products[i].ProductID < products[j].ProductID
		}
		return products[i].Display.SortOrder < products[j].Display.SortOrder
	})
	return products
}

func (category ContentCategory) Validate() error {
	if strings.TrimSpace(category.ID) == "" || strings.Contains(category.ID, " ") {
		return fmt.Errorf("category id %q: %w", category.ID, ErrInvalidContentCategory)
	}
	if strings.TrimSpace(category.DisplayName) == "" {
		return fmt.Errorf("category %q display name: %w", category.ID, ErrInvalidContentCategory)
	}
	if looksLikeRawID(category.DisplayName) {
		return fmt.Errorf("category %q display name %q: %w", category.ID, category.DisplayName, ErrInvalidDisplayMetadata)
	}
	return nil
}

func (product ShopProductDefinition) Validate(categoryIDs map[string]struct{}) error {
	if strings.TrimSpace(string(product.ProductID)) == "" {
		return fmt.Errorf("product id: %w", ErrInvalidShopProduct)
	}
	switch product.ProductType {
	case ShopProductTypeShip, ShopProductTypeModule, ShopProductTypeItem, ShopProductTypePremium, ShopProductTypeLocked:
	default:
		return fmt.Errorf("product %q type %q: %w", product.ProductID, product.ProductType, ErrInvalidShopProduct)
	}
	if err := product.Display.Validate(string(product.ProductID), categoryIDs); err != nil {
		return fmt.Errorf("product %q: %w", product.ProductID, err)
	}
	if err := product.GrantTarget.Validate(); err != nil {
		return fmt.Errorf("product %q: %w", product.ProductID, err)
	}
	if err := product.Price.Validate(); err != nil {
		return fmt.Errorf("product %q: %w", product.ProductID, err)
	}
	if err := product.Stock.Validate(); err != nil {
		return fmt.Errorf("product %q: %w", product.ProductID, err)
	}
	if err := product.Availability.Validate(); err != nil {
		return fmt.Errorf("product %q: %w", product.ProductID, err)
	}
	return nil
}

func (display DisplayMetadata) Validate(rawID string, categoryIDs map[string]struct{}) error {
	if strings.TrimSpace(display.DisplayName) == "" {
		return fmt.Errorf("display name: %w", ErrInvalidDisplayMetadata)
	}
	if display.DisplayName == rawID || looksLikeRawID(display.DisplayName) {
		return fmt.Errorf("display name %q: %w", display.DisplayName, ErrInvalidDisplayMetadata)
	}
	if strings.TrimSpace(display.Description) == "" {
		return fmt.Errorf("description: %w", ErrInvalidDisplayMetadata)
	}
	if strings.TrimSpace(display.Category) == "" {
		return fmt.Errorf("category: %w", ErrInvalidDisplayMetadata)
	}
	if _, ok := categoryIDs[display.Category]; !ok {
		return fmt.Errorf("category %q: %w", display.Category, ErrInvalidContentCategory)
	}
	if strings.TrimSpace(display.ArtKey) == "" || strings.Contains(display.ArtKey, " ") {
		return fmt.Errorf("art key %q: %w", display.ArtKey, ErrInvalidDisplayMetadata)
	}
	if display.Tier < 0 {
		return fmt.Errorf("tier %d: %w", display.Tier, ErrInvalidDisplayMetadata)
	}
	return nil
}

func (target GrantTarget) Validate() error {
	switch target.Kind {
	case GrantTargetKindShip, GrantTargetKindItem, GrantTargetKindModule, GrantTargetKindPremium, GrantTargetKindBlocker:
	default:
		return fmt.Errorf("grant kind %q: %w", target.Kind, ErrInvalidShopProduct)
	}
	if strings.TrimSpace(target.RefID) == "" {
		return fmt.Errorf("grant ref: %w", ErrInvalidShopProduct)
	}
	if target.Quantity < 0 {
		return fmt.Errorf("grant quantity %d: %w", target.Quantity, ErrInvalidShopProduct)
	}
	return nil
}

func (target GrantTarget) ValidateReference(resolver ReferenceResolver) error {
	if err := target.Validate(); err != nil {
		return err
	}
	switch target.Kind {
	case GrantTargetKindShip:
		if resolver.HasShip == nil || !resolver.HasShip(target.RefID) {
			return fmt.Errorf("ship %q: %w", target.RefID, ErrMissingContentReference)
		}
	case GrantTargetKindItem:
		if resolver.HasItem == nil || !resolver.HasItem(target.RefID) {
			return fmt.Errorf("item %q: %w", target.RefID, ErrMissingContentReference)
		}
	case GrantTargetKindModule:
		if resolver.HasModule == nil || !resolver.HasModule(target.RefID) {
			return fmt.Errorf("module %q: %w", target.RefID, ErrMissingContentReference)
		}
	case GrantTargetKindPremium:
		if resolver.HasPremium == nil || !resolver.HasPremium(target.RefID) {
			return fmt.Errorf("premium %q: %w", target.RefID, ErrMissingContentReference)
		}
	case GrantTargetKindBlocker:
		return nil
	}
	return nil
}

func (price PricePolicy) Validate() error {
	switch price.Currency {
	case PriceCurrencyCredits, PriceCurrencyPremium:
	default:
		return fmt.Errorf("price currency %q: %w", price.Currency, ErrInvalidShopProduct)
	}
	if price.Amount < 0 {
		return fmt.Errorf("price amount %d: %w", price.Amount, ErrInvalidShopProduct)
	}
	return nil
}

func (stock StockPolicy) Validate() error {
	switch stock.Kind {
	case StockPolicyUnlimited, StockPolicyLimited, StockPolicyUnavailable:
	default:
		return fmt.Errorf("stock kind %q: %w", stock.Kind, ErrInvalidShopProduct)
	}
	if stock.Remaining < 0 || stock.Total < 0 {
		return fmt.Errorf("stock %d/%d: %w", stock.Remaining, stock.Total, ErrInvalidShopProduct)
	}
	if stock.Kind == StockPolicyLimited && stock.Total > 0 && stock.Remaining > stock.Total {
		return fmt.Errorf("stock %d/%d: %w", stock.Remaining, stock.Total, ErrInvalidShopProduct)
	}
	return nil
}

func (availability AvailabilityRule) Validate() error {
	if availability.RequiredRank < 0 {
		return fmt.Errorf("required rank %d: %w", availability.RequiredRank, ErrInvalidShopProduct)
	}
	if availability.Available && strings.TrimSpace(availability.LockedReason) != "" {
		return fmt.Errorf("available locked reason %q: %w", availability.LockedReason, ErrInvalidShopProduct)
	}
	if !availability.Available && strings.TrimSpace(availability.LockedReason) == "" {
		return fmt.Errorf("locked reason: %w", ErrInvalidShopProduct)
	}
	return nil
}

func looksLikeRawID(value string) bool {
	if strings.Contains(value, "_") {
		return true
	}
	for _, char := range value {
		if unicode.IsLower(char) || unicode.IsDigit(char) || char == '-' {
			continue
		}
		return false
	}
	return true
}

func cloneContentCategories(categories []ContentCategory) []ContentCategory {
	return append([]ContentCategory(nil), categories...)
}

func cloneShopProducts(products []ShopProductDefinition) []ShopProductDefinition {
	return append([]ShopProductDefinition(nil), products...)
}

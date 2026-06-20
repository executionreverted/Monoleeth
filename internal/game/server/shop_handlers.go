package server

import (
	"encoding/json"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/realtime"
)

type shopCatalogRequestPayload struct {
	CategoryID string `json:"category_id,omitempty"`
}

type shopCatalogResponsePayload struct {
	Shop shopCatalogPayload `json:"shop"`
}

type shopCatalogPayload struct {
	CatalogVersion string                `json:"catalog_version"`
	Categories     []shopCategoryPayload `json:"categories"`
	Products       []shopProductPayload  `json:"products"`
}

type shopCategoryPayload struct {
	CategoryID  string `json:"category_id"`
	DisplayName string `json:"display_name"`
	SortOrder   int    `json:"sort_order"`
}

type shopProductPayload struct {
	ProductID    string                  `json:"product_id"`
	ProductType  string                  `json:"product_type"`
	DisplayName  string                  `json:"display_name"`
	Description  string                  `json:"description"`
	CategoryID   string                  `json:"category_id"`
	Subcategory  string                  `json:"subcategory,omitempty"`
	ArtKey       string                  `json:"art_key"`
	Rarity       string                  `json:"rarity,omitempty"`
	Tier         int                     `json:"tier,omitempty"`
	SortOrder    int                     `json:"sort_order"`
	GrantTarget  shopGrantTargetPayload  `json:"grant_target"`
	Price        shopPricePayload        `json:"price"`
	Stock        shopStockPayload        `json:"stock"`
	Availability shopAvailabilityPayload `json:"availability"`
}

type shopGrantTargetPayload struct {
	Kind     string `json:"kind"`
	RefID    string `json:"ref_id"`
	Quantity int64  `json:"quantity,omitempty"`
}

type shopPricePayload struct {
	Currency string `json:"currency_type"`
	Amount   int64  `json:"amount"`
	Fixed    bool   `json:"fixed"`
}

type shopStockPayload struct {
	Kind      string `json:"kind"`
	Remaining int64  `json:"stock_remaining,omitempty"`
	Total     int64  `json:"stock_total,omitempty"`
}

type shopAvailabilityPayload struct {
	Available    bool   `json:"available"`
	LockedReason string `json:"locked_reason,omitempty"`
	RequiredRank int    `json:"required_rank,omitempty"`
}

func (runtime *Runtime) handleShopCatalog(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var filter shopCatalogRequestPayload
	if err := decodeStrict(request.Payload, &filter); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return marshalPayload(shopCatalogResponsePayload{
		Shop: runtime.shopCatalogPayloadLocked(strings.TrimSpace(filter.CategoryID)),
	})
}

func (runtime *Runtime) shopCatalogPayloadLocked(categoryID string) shopCatalogPayload {
	categories := runtime.Content.SortedCategories()
	products := runtime.Content.SortedShopProducts()
	payload := shopCatalogPayload{
		CatalogVersion: runtime.Content.Version.String(),
		Categories:     make([]shopCategoryPayload, 0, len(categories)),
		Products:       make([]shopProductPayload, 0, len(products)),
	}
	for _, category := range categories {
		payload.Categories = append(payload.Categories, shopCategoryPayload{
			CategoryID:  category.ID,
			DisplayName: category.DisplayName,
			SortOrder:   category.SortOrder,
		})
	}
	for _, product := range products {
		if categoryID != "" && product.Display.Category != categoryID {
			continue
		}
		payload.Products = append(payload.Products, shopProductPayloadFromDefinition(product))
	}
	return payload
}

func shopProductPayloadFromDefinition(product catalog.ShopProductDefinition) shopProductPayload {
	return shopProductPayload{
		ProductID:   string(product.ProductID),
		ProductType: string(product.ProductType),
		DisplayName: product.Display.DisplayName,
		Description: product.Display.Description,
		CategoryID:  product.Display.Category,
		Subcategory: product.Display.Subcategory,
		ArtKey:      product.Display.ArtKey,
		Rarity:      product.Display.Rarity,
		Tier:        product.Display.Tier,
		SortOrder:   product.Display.SortOrder,
		GrantTarget: shopGrantTargetPayload{
			Kind:     string(product.GrantTarget.Kind),
			RefID:    product.GrantTarget.RefID,
			Quantity: product.GrantTarget.Quantity,
		},
		Price: shopPricePayload{
			Currency: string(product.Price.Currency),
			Amount:   product.Price.Amount,
			Fixed:    product.Price.Fixed,
		},
		Stock: shopStockPayload{
			Kind:      string(product.Stock.Kind),
			Remaining: product.Stock.Remaining,
			Total:     product.Stock.Total,
		},
		Availability: shopAvailabilityPayload{
			Available:    product.Availability.Available,
			LockedReason: product.Availability.LockedReason,
			RequiredRank: product.Availability.RequiredRank,
		},
	}
}

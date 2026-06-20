package server

import (
	"encoding/json"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/ships"
)

type shopCatalogRequestPayload struct {
	CategoryID string `json:"category_id,omitempty"`
}

type shopCatalogResponsePayload struct {
	Shop shopCatalogPayload `json:"shop"`
}

type shopBuyProductRequestPayload struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity,omitempty"`
}

type shopBuyProductResponsePayload struct {
	Accepted    bool                      `json:"accepted"`
	Duplicate   bool                      `json:"duplicate,omitempty"`
	Product     shopProductPayload        `json:"product"`
	Quantity    int64                     `json:"quantity"`
	ServerTotal int64                     `json:"server_total"`
	Wallet      walletSnapshotPayload     `json:"wallet"`
	Inventory   *inventorySnapshotPayload `json:"inventory,omitempty"`
	Hangar      *hangarSnapshotPayload    `json:"hangar,omitempty"`
	Shop        shopCatalogPayload        `json:"shop"`
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

type shopPurchaseRecord struct {
	ProductID catalog.ShopProductID
	Quantity  int64
	Total     int64
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

func (runtime *Runtime) handleShopBuyProduct(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload shopBuyProductRequestPayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	productID := catalog.ShopProductID(strings.TrimSpace(payload.ProductID))
	if productID == "" {
		return nil, invalidPayload("Shop product is invalid.", nil)
	}
	quantity := payload.Quantity
	if quantity == 0 {
		quantity = 1
	}
	if quantity < 0 {
		return nil, invalidPayload("Shop quantity is invalid.", nil)
	}
	referenceKey, err := foundation.ShopPurchaseIdempotencyKey(ctx.PlayerID, request.RequestID)
	if err != nil {
		return nil, invalidPayload("Shop purchase reference is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if previous, ok := runtime.shopPurchases[referenceKey]; ok {
		product, ok := runtime.Content.ShopProduct(previous.ProductID)
		if !ok {
			return nil, foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
		return marshalPayload(runtime.shopBuyProductResponseLocked(ctx.PlayerID, product, previous.Quantity, previous.Total, true))
	}
	product, ok := runtime.Content.ShopProduct(productID)
	if !ok {
		return nil, foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
	}
	unitQuantity, err := runtime.validateShopProductPurchaseLocked(ctx.PlayerID, product, quantity)
	if err != nil {
		return nil, err
	}
	grantQuantity := unitQuantity * quantity
	total, err := shopPurchaseTotal(product.Price.Amount, quantity)
	if err != nil {
		return nil, err
	}
	currency, err := shopCurrencyBucket(product.Price.Currency)
	if err != nil {
		return nil, err
	}
	debit, err := runtime.Wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     ctx.PlayerID,
		Currency:     currency,
		Amount:       total,
		Reason:       economy.LedgerReason("shop_purchase"),
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if err := runtime.grantShopProductLocked(ctx.PlayerID, product, grantQuantity, referenceKey); err != nil {
		return nil, err
	}
	if !debit.Duplicate {
		runtime.recordCurrencyLedgerMetric(debit.LedgerEntry)
	}
	runtime.shopPurchases[referenceKey] = shopPurchaseRecord{
		ProductID: product.ProductID,
		Quantity:  quantity,
		Total:     total,
	}
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	state := runtime.players[ctx.PlayerID]
	state.Wallet = wallet
	runtime.players[ctx.PlayerID] = state
	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventWalletSnapshot, wallet)
	switch product.GrantTarget.Kind {
	case catalog.GrantTargetKindItem, catalog.GrantTargetKindModule:
		runtime.queueEventLocked(sessionID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(ctx.PlayerID))
	case catalog.GrantTargetKindShip:
		hangar, err := runtime.hangarSnapshotLocked(ctx.PlayerID)
		if err != nil {
			return nil, domainErrorForRuntime(err)
		}
		runtime.queueEventLocked(sessionID, realtime.EventHangarSnapshot, hangar)
	}
	return marshalPayload(runtime.shopBuyProductResponseLocked(ctx.PlayerID, product, quantity, total, false))
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

func (runtime *Runtime) validateShopProductPurchaseLocked(playerID foundation.PlayerID, product catalog.ShopProductDefinition, quantity int64) (int64, error) {
	if !product.Availability.Available {
		reason := strings.TrimSpace(product.Availability.LockedReason)
		if reason == "" {
			reason = "Shop product is unavailable."
		}
		return 0, foundation.NewDomainError(foundation.CodeForbidden, reason)
	}
	if product.Stock.Kind == catalog.StockPolicyUnavailable {
		return 0, foundation.NewDomainError(foundation.CodeForbidden, "Shop product is unavailable.")
	}
	if product.Stock.Kind == catalog.StockPolicyLimited && product.Stock.Remaining < quantity {
		return 0, foundation.NewDomainError(foundation.CodeForbidden, "Shop stock is unavailable.")
	}
	if product.Price.Amount <= 0 {
		return 0, foundation.NewDomainError(foundation.CodeForbidden, "Shop product is unavailable.")
	}
	rank := runtime.players[playerID].Rank
	if product.Availability.RequiredRank > 0 && rank < product.Availability.RequiredRank {
		return 0, foundation.NewDomainError(foundation.CodeRankTooLow, "Pilot rank requirement is not met.")
	}
	unitQuantity := product.GrantTarget.Quantity
	if unitQuantity == 0 {
		unitQuantity = 1
	}
	if product.GrantTarget.Kind != catalog.GrantTargetKindItem && quantity != 1 {
		return 0, invalidPayload("Shop quantity is invalid.", nil)
	}
	if unitQuantity > 0 && quantity > 9223372036854775807/unitQuantity {
		return 0, invalidPayload("Shop quantity is invalid.", nil)
	}
	switch product.GrantTarget.Kind {
	case catalog.GrantTargetKindItem:
		itemID, err := foundation.ParseItemID(product.GrantTarget.RefID)
		if err != nil {
			return 0, invalidPayload("Shop product is invalid.", err)
		}
		if _, ok := runtime.itemCatalog[itemID]; !ok {
			return 0, foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
	case catalog.GrantTargetKindModule:
		itemID, err := foundation.ParseItemID(product.GrantTarget.RefID)
		if err != nil {
			return 0, invalidPayload("Shop product is invalid.", err)
		}
		if _, ok := runtime.itemCatalog[itemID]; !ok {
			return 0, foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
		if _, ok := runtime.ModuleCatalog.Lookup(itemID); !ok {
			return 0, foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
	case catalog.GrantTargetKindShip:
		shipID, err := foundation.ParseShipID(product.GrantTarget.RefID)
		if err != nil {
			return 0, invalidPayload("Shop product is invalid.", err)
		}
		if _, ok := runtime.HangarStore.PlayerShip(playerID, shipID); ok {
			return 0, foundation.NewDomainError(foundation.CodeForbidden, "Ship is already unlocked.")
		}
	case catalog.GrantTargetKindPremium, catalog.GrantTargetKindBlocker:
		return 0, foundation.NewDomainError(foundation.CodeForbidden, "Shop product is unavailable.")
	default:
		return 0, foundation.NewDomainError(foundation.CodeForbidden, "Shop product is unavailable.")
	}
	return unitQuantity, nil
}

func (runtime *Runtime) grantShopProductLocked(playerID foundation.PlayerID, product catalog.ShopProductDefinition, totalQuantity int64, referenceKey foundation.IdempotencyKey) error {
	switch product.GrantTarget.Kind {
	case catalog.GrantTargetKindItem:
		itemID, err := foundation.ParseItemID(product.GrantTarget.RefID)
		if err != nil {
			return invalidPayload("Shop product is invalid.", err)
		}
		definition, ok := runtime.itemCatalog[itemID]
		if !ok {
			return foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
		location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
		if err != nil {
			return invalidPayload("Shop inventory location is invalid.", err)
		}
		result, err := runtime.Inventory.AddItem(economy.AddItemInput{
			PlayerID:       playerID,
			ItemDefinition: definition,
			Quantity:       totalQuantity,
			Location:       location,
			Reason:         economy.LedgerReason("shop_purchase"),
			ReferenceKey:   referenceKey,
		})
		if err != nil {
			return domainErrorForEconomy(err)
		}
		if !result.Duplicate {
			runtime.recordItemLedgerMetrics([]economy.ItemLedgerEntry{result.LedgerEntry})
		}
	case catalog.GrantTargetKindModule:
		itemID, err := foundation.ParseItemID(product.GrantTarget.RefID)
		if err != nil {
			return invalidPayload("Shop product is invalid.", err)
		}
		definition, ok := runtime.itemCatalog[itemID]
		if !ok {
			return foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
		moduleDefinition, ok := runtime.ModuleCatalog.Lookup(itemID)
		if !ok {
			return foundation.NewDomainError(foundation.CodeNotFound, "Shop product was not found.")
		}
		location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
		if err != nil {
			return invalidPayload("Shop inventory location is invalid.", err)
		}
		result, err := runtime.Inventory.AddItem(economy.AddItemInput{
			PlayerID:       playerID,
			ItemDefinition: definition,
			Quantity:       totalQuantity,
			Location:       location,
			Reason:         economy.LedgerReason("shop_purchase"),
			ReferenceKey:   referenceKey,
		})
		if err != nil {
			return domainErrorForEconomy(err)
		}
		for _, item := range result.InstanceItems {
			updated, err := runtime.Inventory.SystemSetInstanceDurability(playerID, item.ItemInstanceID, moduleDefinition.Durability.Max)
			if err != nil {
				return domainErrorForEconomy(err)
			}
			if err := runtime.LoadoutStore.PutModuleItem(updated); err != nil {
				return domainErrorForRuntime(err)
			}
		}
		if !result.Duplicate {
			runtime.recordItemLedgerMetrics([]economy.ItemLedgerEntry{result.LedgerEntry})
		}
	case catalog.GrantTargetKindShip:
		shipID, err := foundation.ParseShipID(product.GrantTarget.RefID)
		if err != nil {
			return invalidPayload("Shop product is invalid.", err)
		}
		if _, err := runtime.Hangar.UnlockShip(ships.UnlockShipInput{
			PlayerID:    playerID,
			ShipID:      shipID,
			Source:      "shop",
			ReferenceID: referenceKey.String(),
		}); err != nil {
			return domainErrorForRuntime(err)
		}
	}
	return nil
}

func (runtime *Runtime) shopBuyProductResponseLocked(playerID foundation.PlayerID, product catalog.ShopProductDefinition, quantity int64, total int64, duplicate bool) shopBuyProductResponsePayload {
	response := shopBuyProductResponsePayload{
		Accepted:    true,
		Duplicate:   duplicate,
		Product:     shopProductPayloadFromDefinition(product),
		Quantity:    quantity,
		ServerTotal: total,
		Wallet:      runtime.walletSnapshotLocked(playerID),
		Shop:        runtime.shopCatalogPayloadLocked(""),
	}
	switch product.GrantTarget.Kind {
	case catalog.GrantTargetKindItem, catalog.GrantTargetKindModule:
		inventory := runtime.inventorySnapshotLocked(playerID)
		response.Inventory = &inventory
	case catalog.GrantTargetKindShip:
		hangar, err := runtime.hangarSnapshotLocked(playerID)
		if err == nil {
			response.Hangar = &hangar
		}
	}
	return response
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

func shopPurchaseTotal(unitPrice int64, quantity int64) (int64, error) {
	if unitPrice <= 0 || quantity <= 0 {
		return 0, invalidPayload("Shop quantity is invalid.", nil)
	}
	if quantity > 9223372036854775807/unitPrice {
		return 0, invalidPayload("Shop quantity is invalid.", nil)
	}
	return unitPrice * quantity, nil
}

func shopCurrencyBucket(currency catalog.PriceCurrency) (economy.CurrencyBucket, error) {
	switch currency {
	case catalog.PriceCurrencyCredits:
		return economy.CurrencyBucketCredits, nil
	case catalog.PriceCurrencyPremium:
		return economy.CurrencyBucketPremiumPaid, nil
	default:
		return "", invalidPayload("Shop currency is invalid.", nil)
	}
}

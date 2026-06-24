package contentdb

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
)

func mapShopProductRows(snapshot content.Snapshot) (catalog.ContentRegistry, error) {
	products := make([]catalog.ShopProductDefinition, 0, len(snapshot.ShopProducts))
	for _, row := range snapshot.ShopProducts {
		if !row.Enabled {
			continue
		}
		var product catalog.ShopProductDefinition
		if err := decodeSnapshotRow(content.ContentTypeShopProduct, row, &product); err != nil {
			return catalog.ContentRegistry{}, err
		}
		if err := requireRowID(content.ContentTypeShopProduct, row, string(product.ProductID)); err != nil {
			return catalog.ContentRegistry{}, err
		}
		products = append(products, product)
	}
	registry, err := catalog.NewContentRegistry(publishedVersion(snapshot), content.DefaultShopCategories(), products)
	if err != nil {
		return catalog.ContentRegistry{}, fmt.Errorf("shop: %w", err)
	}
	return registry, nil
}

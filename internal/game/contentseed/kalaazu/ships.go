package kalaazu

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
)

const kalaazuShipCatalogVersion catalog.Version = "kalaazu_ship_seed_v1"

func BuildStarterShipRows(filesystem fs.FS) ([]content.SnapshotRow, error) {
	itemRows, err := LoadDumpRows(filesystem, "testdata/items.sql")
	if err != nil {
		return nil, err
	}
	shipRows, err := LoadDumpRows(filesystem, "testdata/ships.sql")
	if err != nil {
		return nil, err
	}
	return mapStarterShipRows(itemRows, shipRows)
}

func mapStarterShipRows(itemRows []DumpRow, shipRows []DumpRow) ([]content.SnapshotRow, error) {
	itemsByKalaazuID := make(map[int]kalaazuItemSource)
	for _, row := range itemRows {
		source, err := decodeKalaazuItem(row)
		if err != nil {
			return nil, err
		}
		itemsByKalaazuID[source.KalaazuID] = source
	}

	sources := make([]kalaazuShipSource, 0, len(shipRows))
	for _, row := range shipRows {
		source, err := decodeKalaazuShip(row)
		if err != nil {
			return nil, err
		}
		item, ok := itemsByKalaazuID[source.ItemID]
		if !ok {
			return nil, fmt.Errorf("ship item %d: %w", source.ItemID, ErrMalformedDumpSQL)
		}
		source.Item = item
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].KalaazuID < sources[j].KalaazuID })

	rows := make([]content.SnapshotRow, 0, len(sources))
	for _, source := range sources {
		definition, err := shipDefinition(source)
		if err != nil {
			return nil, err
		}
		row, err := snapshotRow(definition.ShipID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

type kalaazuItemSource struct {
	KalaazuID int
	Name      string
	LootID    string
	Category  int
	Type      int
	Price     int64
	IsElite   bool
	IsEvent   bool
	IsBuyable bool
	Bonus     int
}

func decodeKalaazuItem(row DumpRow) (kalaazuItemSource, error) {
	id, err := row.Int("id")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	lootID, err := row.String("loot_id")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	category, err := row.Int("category")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	itemType, err := row.Int("type")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	price, err := row.Int("price")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	isElite, err := row.Bool("is_elite")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	isEvent, err := row.Bool("is_event")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	isBuyable, err := row.Bool("is_buyable")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	bonus, err := row.Int("bonus")
	if err != nil {
		return kalaazuItemSource{}, err
	}
	return kalaazuItemSource{
		KalaazuID: id,
		Name:      normalizeDisplayName(name),
		LootID:    normalizeIdentifier(lootID),
		Category:  category,
		Type:      itemType,
		Price:     int64(price),
		IsElite:   isElite,
		IsEvent:   isEvent,
		IsBuyable: isBuyable,
		Bonus:     bonus,
	}, nil
}

type kalaazuShipSource struct {
	KalaazuID  int
	ItemID     int
	Health     int
	Speed      int
	Cargo      int
	Batteries  int
	Rockets    int
	Lasers     int
	Hellstorms int
	Generators int
	Extras     int
	GFX        int
	Item       kalaazuItemSource
}

func decodeKalaazuShip(row DumpRow) (kalaazuShipSource, error) {
	id, err := row.Int("id")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	itemID, err := row.Int("items_id")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	health, err := row.Int("health")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	speed, err := row.Int("speed")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	cargo, err := row.Int("cargo")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	batteries, err := row.Int("batteries")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	rockets, err := row.Int("rockets")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	lasers, err := row.Int("lasers")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	hellstorms, err := row.Int("hellstorms")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	generators, err := row.Int("generators")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	extras, err := row.Int("extras")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	gfx, err := row.Int("gfx")
	if err != nil {
		return kalaazuShipSource{}, err
	}
	return kalaazuShipSource{
		KalaazuID:  id,
		ItemID:     itemID,
		Health:     health,
		Speed:      speed,
		Cargo:      cargo,
		Batteries:  batteries,
		Rockets:    rockets,
		Lasers:     lasers,
		Hellstorms: hellstorms,
		Generators: generators,
		Extras:     extras,
		GFX:        gfx,
	}, nil
}

func shipDefinition(source kalaazuShipSource) (ships.ShipDefinition, error) {
	shipID := foundation.ShipID(source.Item.LootID)
	sourceRow, err := catalog.NewVersionedDefinitionFromStrings(shipID.String(), kalaazuShipCatalogVersion.String())
	if err != nil {
		return ships.ShipDefinition{}, err
	}
	definition, err := ships.NewShipDefinition(
		sourceRow,
		shipID,
		source.Item.Name,
		shipTier(source),
		shipRole(source),
		shipRankRequirement(source),
		ships.ShipBaseStats{
			HP:            int64(source.Health),
			Shield:        int64(maxInt(1, source.Health/2)),
			Energy:        int64(maxInt(1, source.Batteries)),
			EnergyRegen:   int64(maxInt(1, source.Batteries/1000)),
			Speed:         int64(source.Speed),
			CargoCapacity: int64(source.Cargo),
			Radar:         420,
			Signature:     int64(maxInt(60, source.Health/2000)),
		},
		ships.SlotLayout{
			Offensive: source.Lasers,
			Defensive: source.Generators,
			Utility:   maxInt(1, source.Extras),
		},
	)
	if err != nil {
		return ships.ShipDefinition{}, err
	}
	if source.Item.IsElite {
		definition.PremiumPrice = source.Item.Price
	} else {
		definition.CreditPrice = source.Item.Price
	}
	definition.RepairCostMultiplierBps = 10_000
	if err := definition.Validate(); err != nil {
		return ships.ShipDefinition{}, err
	}
	return definition, nil
}

func shipTier(source kalaazuShipSource) int {
	switch {
	case source.Health >= 250000:
		return 4
	case source.Health >= 120000:
		return 3
	case source.Health >= 64000:
		return 2
	default:
		return 1
	}
}

func shipRankRequirement(source kalaazuShipSource) int {
	switch {
	case source.KalaazuID <= 2:
		return 1
	case source.KalaazuID <= 6:
		return 2
	case source.KalaazuID <= 10:
		return 3
	default:
		return 4
	}
}

func shipRole(source kalaazuShipSource) ships.ShipRole {
	name := strings.ToLower(source.Item.Name)
	switch {
	case strings.Contains(name, "aegis"):
		return ships.ShipRoleSupport
	case strings.Contains(name, "citadel"):
		return ships.ShipRoleIndustrial
	case source.Cargo >= 1500:
		return ships.ShipRoleHauler
	case source.Speed >= 370:
		return ships.ShipRoleScout
	case source.Lasers >= 5:
		return ships.ShipRoleFighter
	default:
		return ships.ShipRoleSupport
	}
}

func normalizeIdentifier(value string) string {
	normalized := npcTypePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func normalizeDisplayName(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

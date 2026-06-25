package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
)

type MarketListingStore struct {
	store *Store
}

var _ market.MarketListingRepository = (*MarketListingStore)(nil)
var _ market.MarketListingTransactionRepository = (*MarketListingStore)(nil)

type MarketStore = MarketListingStore

type MarketListingTx struct {
	tx *sql.Tx
}

var _ economy.IdempotencyStore = (*MarketListingTx)(nil)
var _ market.MarketListingTransaction = (*MarketListingTx)(nil)

type marketListingSQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type marketListingSQLQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewMarketListingStore(store *Store) (*MarketListingStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &MarketListingStore{store: store}, nil
}

func NewMarketStore(store *Store) (*MarketStore, error) {
	return NewMarketListingStore(store)
}

func (store *MarketListingStore) WithTransaction(ctx context.Context, fn func(*MarketListingTx) error) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if fn == nil {
		return errors.New("nil market listing transaction function")
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if err = fn(&MarketListingTx{tx: tx}); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (store *MarketListingStore) WithMarketListingTransaction(ctx context.Context, fn func(market.MarketListingTransaction) error) error {
	if fn == nil {
		return errors.New("nil market listing transaction function")
	}
	return store.WithTransaction(ctx, func(tx *MarketListingTx) error {
		return fn(tx)
	})
}

func (store *MarketListingStore) UpsertMarketListing(ctx context.Context, listing market.Listing) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	return upsertMarketListing(ctx, store.store.db, listing)
}

func (store *MarketListingStore) SaveMarketListing(ctx context.Context, listing market.Listing) error {
	return store.UpsertMarketListing(ctx, listing)
}

func (tx *MarketListingTx) UpsertMarketListing(ctx context.Context, listing market.Listing) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return upsertMarketListing(ctx, tx.tx, listing)
}

func (tx *MarketListingTx) SaveMarketListing(ctx context.Context, listing market.Listing) error {
	return tx.UpsertMarketListing(ctx, listing)
}

func (tx *MarketListingTx) CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return commitWalletMutation(ctx, tx.tx, commit)
}

func (tx *MarketListingTx) CommitInventoryMoveItem(ctx context.Context, commit economy.InventoryMoveItemCommit) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return commitInventoryMoveItem(ctx, tx.tx, commit)
}

func (tx *MarketListingTx) ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error) {
	if tx == nil || tx.tx == nil {
		return economy.IdempotencyClaimResult{}, ErrNilDatabase
	}
	return claimIdempotencyKey(ctx, tx.tx, row)
}

func (tx *MarketListingTx) CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error) {
	if tx == nil || tx.tx == nil {
		return economy.IdempotencyKeyRow{}, ErrNilDatabase
	}
	return completeIdempotencyKey(ctx, tx.tx, row)
}

func (tx *MarketListingTx) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return insertOutboxRow(ctx, tx.tx, row)
}

func (store *MarketListingStore) LoadMarketListing(ctx context.Context, listingID foundation.ListingID) (market.Listing, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return market.Listing{}, false, ErrNilDatabase
	}
	return loadMarketListing(ctx, store.store.db, listingID, false)
}

func (tx *MarketListingTx) LoadMarketListingForUpdate(ctx context.Context, listingID foundation.ListingID) (market.Listing, bool, error) {
	if tx == nil || tx.tx == nil {
		return market.Listing{}, false, ErrNilDatabase
	}
	return loadMarketListing(ctx, tx.tx, listingID, true)
}

func (store *MarketListingStore) LoadMarketListings(ctx context.Context) ([]market.Listing, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, marketListingSelectSQL()+`
		ORDER BY listing_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	listings := make([]market.Listing, 0)
	for rows.Next() {
		listing, err := scanMarketListing(rows)
		if err != nil {
			return nil, err
		}
		listings = append(listings, listing)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(listings, func(i, j int) bool {
		return listings[i].ListingID < listings[j].ListingID
	})
	return listings, nil
}

func upsertMarketListing(ctx context.Context, execer marketListingSQLExecer, listing market.Listing) error {
	if err := validateMarketListingRow(listing); err != nil {
		return err
	}
	definitionJSON, err := json.Marshal(snapshotMarketItemDefinition(listing.ItemDefinition))
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO market_listings(
			listing_id, seller_player_id, item_definition_json, item_instance_id,
			item_id, original_quantity, remaining_quantity, unit_price, currency_type,
			status, source_location_kind, source_location_id, escrow_location_kind,
			escrow_location_id, created_at, updated_at, expires_at, stale_at, stale_reason
		)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (listing_id) DO UPDATE
		SET seller_player_id = EXCLUDED.seller_player_id,
			item_definition_json = EXCLUDED.item_definition_json,
			item_instance_id = EXCLUDED.item_instance_id,
			item_id = EXCLUDED.item_id,
			original_quantity = EXCLUDED.original_quantity,
			remaining_quantity = EXCLUDED.remaining_quantity,
			unit_price = EXCLUDED.unit_price,
			currency_type = EXCLUDED.currency_type,
			status = EXCLUDED.status,
			source_location_kind = EXCLUDED.source_location_kind,
			source_location_id = EXCLUDED.source_location_id,
			escrow_location_kind = EXCLUDED.escrow_location_kind,
			escrow_location_id = EXCLUDED.escrow_location_id,
			updated_at = EXCLUDED.updated_at,
			expires_at = EXCLUDED.expires_at,
			stale_at = EXCLUDED.stale_at,
			stale_reason = EXCLUDED.stale_reason
	`, listing.ListingID.String(), listing.SellerPlayerID.String(), string(definitionJSON),
		listing.ItemInstanceID.String(), listing.ItemID.String(), listing.OriginalQuantity,
		listing.RemainingQuantity, listing.UnitPrice, listing.Currency.String(), listing.Status.String(),
		listing.SourceReturnLocation.Kind.String(), listing.SourceReturnLocation.ID.String(),
		listing.EscrowLocation.Kind.String(), listing.EscrowLocation.ID.String(), listing.CreatedAt.UTC(),
		listing.UpdatedAt.UTC(), zeroTimeAsNilPointer(listing.ExpiresAt), zeroTimeAsNilPointer(listing.StaleAt),
		listing.StaleReason)
	return err
}

func loadMarketListing(ctx context.Context, querier marketListingSQLQuerier, listingID foundation.ListingID, forUpdate bool) (market.Listing, bool, error) {
	if err := listingID.Validate(); err != nil {
		return market.Listing{}, false, err
	}
	query := marketListingSelectSQL() + `
		WHERE listing_id = $1
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	listing, err := scanMarketListing(querier.QueryRowContext(ctx, query, listingID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return market.Listing{}, false, nil
	}
	if err != nil {
		return market.Listing{}, false, err
	}
	return listing, true, nil
}

func scanMarketListing(scanner rowScanner) (market.Listing, error) {
	var listing market.Listing
	var listingID string
	var sellerPlayerID string
	var definitionJSON []byte
	var itemInstanceID string
	var itemID string
	var status string
	var currency string
	var sourceKind string
	var sourceID string
	var escrowKind string
	var escrowID string
	var expiresAt sql.NullTime
	var staleAt sql.NullTime
	if err := scanner.Scan(
		&listingID,
		&sellerPlayerID,
		&definitionJSON,
		&itemInstanceID,
		&itemID,
		&listing.OriginalQuantity,
		&listing.RemainingQuantity,
		&listing.UnitPrice,
		&currency,
		&status,
		&sourceKind,
		&sourceID,
		&escrowKind,
		&escrowID,
		&listing.CreatedAt,
		&listing.UpdatedAt,
		&expiresAt,
		&staleAt,
		&listing.StaleReason,
	); err != nil {
		return market.Listing{}, err
	}
	var definitionSnapshot marketItemDefinitionSnapshot
	if err := json.Unmarshal(definitionJSON, &definitionSnapshot); err != nil {
		return market.Listing{}, err
	}
	definition, err := definitionSnapshot.itemDefinition()
	if err != nil {
		return market.Listing{}, err
	}
	listing.ItemDefinition = definition
	listing.ListingID = foundation.ListingID(listingID)
	listing.SellerPlayerID = foundation.PlayerID(sellerPlayerID)
	listing.ItemInstanceID = foundation.ItemID(itemInstanceID)
	listing.ItemID = foundation.ItemID(itemID)
	listing.Currency = economy.CurrencyBucket(currency)
	listing.Status = market.ListingStatus(status)
	listing.SourceReturnLocation = economy.ItemLocation{
		Kind: economy.LocationKind(sourceKind),
		ID:   economy.LocationID(sourceID),
	}
	listing.EscrowLocation = economy.ItemLocation{
		Kind: economy.LocationKind(escrowKind),
		ID:   economy.LocationID(escrowID),
	}
	listing.CreatedAt = listing.CreatedAt.UTC()
	listing.UpdatedAt = listing.UpdatedAt.UTC()
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		listing.ExpiresAt = &value
	}
	if staleAt.Valid {
		value := staleAt.Time.UTC()
		listing.StaleAt = &value
	}
	if err := validateMarketListingRow(listing); err != nil {
		return market.Listing{}, err
	}
	return listing, nil
}

type marketItemDefinitionSnapshot struct {
	Source         catalog.VersionedDefinition `json:"source"`
	ItemID         foundation.ItemID           `json:"item_id"`
	Name           string                      `json:"name"`
	Type           economy.ItemType            `json:"item_type"`
	Rarity         economy.ItemRarity          `json:"rarity"`
	MaxStack       int64                       `json:"max_stack"`
	WeightUnits    int64                       `json:"weight_units"`
	TradeFlags     []economy.TradeFlag         `json:"trade_flags,omitempty"`
	BindRules      []economy.BindRule          `json:"bind_rules,omitempty"`
	MetadataSchema json.RawMessage             `json:"metadata_schema,omitempty"`
}

func snapshotMarketItemDefinition(definition economy.ItemDefinition) marketItemDefinitionSnapshot {
	metadata := append(json.RawMessage(nil), definition.MetadataSchema...)
	if len(metadata) == 0 {
		metadata = nil
	}
	return marketItemDefinitionSnapshot{
		Source:         definition.Source,
		ItemID:         definition.ItemID,
		Name:           definition.Name,
		Type:           definition.Type,
		Rarity:         definition.Rarity,
		MaxStack:       definition.MaxStack.Int64(),
		WeightUnits:    definition.WeightUnits.Int64(),
		TradeFlags:     append([]economy.TradeFlag(nil), definition.TradeFlags...),
		BindRules:      append([]economy.BindRule(nil), definition.BindRules...),
		MetadataSchema: metadata,
	}
}

func (snapshot marketItemDefinitionSnapshot) itemDefinition() (economy.ItemDefinition, error) {
	maxStack, err := foundation.NewQuantity(snapshot.MaxStack)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightUnits, err := foundation.NewQuantity(snapshot.WeightUnits)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		snapshot.Source,
		snapshot.ItemID,
		snapshot.Name,
		snapshot.Type,
		snapshot.Rarity,
		maxStack,
		weightUnits,
		snapshot.TradeFlags,
		snapshot.BindRules,
		snapshot.MetadataSchema,
	)
}

func validateMarketListingRow(listing market.Listing) error {
	if err := listing.ListingID.Validate(); err != nil {
		return err
	}
	if err := listing.SellerPlayerID.Validate(); err != nil {
		return err
	}
	if err := listing.ItemDefinition.Validate(); err != nil {
		return err
	}
	if !listing.ItemInstanceID.IsZero() {
		if err := listing.ItemInstanceID.Validate(); err != nil {
			return err
		}
	}
	if err := listing.ItemID.Validate(); err != nil {
		return err
	}
	if listing.ItemDefinition.ItemID != listing.ItemID {
		return fmt.Errorf("listing %q item %q definition %q: %w", listing.ListingID, listing.ItemID, listing.ItemDefinition.ItemID, economy.ErrItemSourceMismatch)
	}
	if _, err := foundation.NewQuantity(listing.OriginalQuantity); err != nil {
		return err
	}
	if listing.RemainingQuantity < 0 || listing.RemainingQuantity > listing.OriginalQuantity {
		return fmt.Errorf("listing %q remaining quantity %d original %d: %w", listing.ListingID, listing.RemainingQuantity, listing.OriginalQuantity, economy.ErrInsufficientItemQuantity)
	}
	if _, err := foundation.NewMoney(listing.UnitPrice); err != nil {
		return err
	}
	if err := listing.Currency.Validate(); err != nil {
		return err
	}
	if err := listing.Status.Validate(); err != nil {
		return err
	}
	if err := listing.SourceReturnLocation.Validate(); err != nil {
		return err
	}
	if err := listing.EscrowLocation.Validate(); err != nil {
		return err
	}
	if listing.EscrowLocation.Kind != economy.LocationKindMarketEscrow {
		return fmt.Errorf("listing %q escrow location %q: %w", listing.ListingID, listing.EscrowLocation.Kind, economy.ErrInvalidLocationKind)
	}
	if listing.CreatedAt.IsZero() || listing.UpdatedAt.IsZero() {
		return fmt.Errorf("listing %q timestamp missing", listing.ListingID)
	}
	if listing.Status == market.ListingStatusStale && listing.StaleAt == nil {
		return fmt.Errorf("listing %q stale timestamp missing", listing.ListingID)
	}
	return nil
}

func marketListingSelectSQL() string {
	return `
		SELECT
			listing_id,
			seller_player_id,
			item_definition_json,
			item_instance_id,
			item_id,
			original_quantity,
			remaining_quantity,
			unit_price,
			currency_type,
			status,
			source_location_kind,
			source_location_id,
			escrow_location_kind,
			escrow_location_id,
			created_at,
			updated_at,
			expires_at,
			stale_at,
			stale_reason
		FROM market_listings
	`
}

func zeroTimeAsNilPointer(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

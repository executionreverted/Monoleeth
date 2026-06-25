package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

type AuctionLotStore struct {
	store *Store
}

var _ auction.AuctionLotRepository = (*AuctionLotStore)(nil)
var _ auction.AuctionLotTransactionRepository = (*AuctionLotStore)(nil)

type AuctionStore = AuctionLotStore

type AuctionLotTx struct {
	tx *sql.Tx
}

var _ auction.AuctionLotTransaction = (*AuctionLotTx)(nil)

type auctionLotSQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type auctionLotSQLQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewAuctionLotStore(store *Store) (*AuctionLotStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &AuctionLotStore{store: store}, nil
}

func NewAuctionStore(store *Store) (*AuctionStore, error) {
	return NewAuctionLotStore(store)
}

func (store *AuctionLotStore) WithTransaction(ctx context.Context, fn func(*AuctionLotTx) error) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if fn == nil {
		return errors.New("nil auction lot transaction function")
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if err = fn(&AuctionLotTx{tx: tx}); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (store *AuctionLotStore) WithAuctionLotTransaction(ctx context.Context, fn func(auction.AuctionLotTransaction) error) error {
	if fn == nil {
		return errors.New("nil auction lot transaction function")
	}
	return store.WithTransaction(ctx, func(tx *AuctionLotTx) error {
		return fn(tx)
	})
}

func (store *AuctionLotStore) SaveAuctionLot(ctx context.Context, lot auction.Lot) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	return upsertAuctionLot(ctx, store.store.db, lot)
}

func (tx *AuctionLotTx) SaveAuctionLot(ctx context.Context, lot auction.Lot) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return upsertAuctionLot(ctx, tx.tx, lot)
}

func (tx *AuctionLotTx) CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return commitWalletMutation(ctx, tx.tx, commit)
}

func (tx *AuctionLotTx) ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error) {
	if tx == nil || tx.tx == nil {
		return economy.IdempotencyClaimResult{}, ErrNilDatabase
	}
	return claimIdempotencyKey(ctx, tx.tx, row)
}

func (tx *AuctionLotTx) CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error) {
	if tx == nil || tx.tx == nil {
		return economy.IdempotencyKeyRow{}, ErrNilDatabase
	}
	return completeIdempotencyKey(ctx, tx.tx, row)
}

func (tx *AuctionLotTx) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return insertOutboxRow(ctx, tx.tx, row)
}

func (store *AuctionLotStore) LoadAuctionLot(ctx context.Context, auctionID foundation.AuctionID) (auction.Lot, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return auction.Lot{}, false, ErrNilDatabase
	}
	return loadAuctionLot(ctx, store.store.db, auctionID, false)
}

func (tx *AuctionLotTx) LoadAuctionLotForUpdate(ctx context.Context, auctionID foundation.AuctionID) (auction.Lot, bool, error) {
	if tx == nil || tx.tx == nil {
		return auction.Lot{}, false, ErrNilDatabase
	}
	return loadAuctionLot(ctx, tx.tx, auctionID, true)
}

func (store *AuctionLotStore) LoadAuctionLots(ctx context.Context) ([]auction.Lot, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, auctionLotSelectSQL()+`
		ORDER BY auction_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lots := make([]auction.Lot, 0)
	for rows.Next() {
		lot, err := scanAuctionLot(rows)
		if err != nil {
			return nil, err
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(lots, func(i, j int) bool {
		return lots[i].AuctionID < lots[j].AuctionID
	})
	return lots, nil
}

func upsertAuctionLot(ctx context.Context, execer auctionLotSQLExecer, lot auction.Lot) error {
	if err := validateAuctionLotRow(lot); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(lot.Payload)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO auction_lots(
			auction_id, world_id, payload_json, currency_type, start_price,
			buy_now_price, current_bid, current_bidder_id, status, starts_at,
			ends_at, created_at, updated_at, closed_at, winning_player_id,
			close_reason
		)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (auction_id) DO UPDATE
		SET world_id = EXCLUDED.world_id,
			payload_json = EXCLUDED.payload_json,
			currency_type = EXCLUDED.currency_type,
			start_price = EXCLUDED.start_price,
			buy_now_price = EXCLUDED.buy_now_price,
			current_bid = EXCLUDED.current_bid,
			current_bidder_id = EXCLUDED.current_bidder_id,
			status = EXCLUDED.status,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			updated_at = EXCLUDED.updated_at,
			closed_at = EXCLUDED.closed_at,
			winning_player_id = EXCLUDED.winning_player_id,
			close_reason = EXCLUDED.close_reason
	`, lot.AuctionID.String(), lot.WorldID.String(), string(payloadJSON), lot.Currency.String(),
		lot.StartPrice, int64PointerAsNil(lot.BuyNowPrice), lot.CurrentBid, lot.CurrentBidderID.String(),
		lot.Status.String(), lot.StartsAt.UTC(), lot.EndsAt.UTC(), lot.CreatedAt.UTC(), lot.UpdatedAt.UTC(),
		zeroTimeAsNilPointer(lot.ClosedAt), lot.WinningPlayerID.String(), lot.CloseReason)
	return err
}

func loadAuctionLot(ctx context.Context, querier auctionLotSQLQuerier, auctionID foundation.AuctionID, forUpdate bool) (auction.Lot, bool, error) {
	if err := auctionID.Validate(); err != nil {
		return auction.Lot{}, false, err
	}
	query := auctionLotSelectSQL() + `
		WHERE auction_id = $1
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	lot, err := scanAuctionLot(querier.QueryRowContext(ctx, query, auctionID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return auction.Lot{}, false, nil
	}
	if err != nil {
		return auction.Lot{}, false, err
	}
	return lot, true, nil
}

func scanAuctionLot(scanner rowScanner) (auction.Lot, error) {
	var lot auction.Lot
	var auctionID string
	var worldID string
	var payloadJSON []byte
	var currency string
	var currentBidderID string
	var status string
	var buyNowPrice sql.NullInt64
	var closedAt sql.NullTime
	var winningPlayerID string
	var closeReason string
	if err := scanner.Scan(
		&auctionID,
		&worldID,
		&payloadJSON,
		&currency,
		&lot.StartPrice,
		&buyNowPrice,
		&lot.CurrentBid,
		&currentBidderID,
		&status,
		&lot.StartsAt,
		&lot.EndsAt,
		&lot.CreatedAt,
		&lot.UpdatedAt,
		&closedAt,
		&winningPlayerID,
		&closeReason,
	); err != nil {
		return auction.Lot{}, err
	}
	var payload auction.LotPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return auction.Lot{}, err
	}
	lot.AuctionID = foundation.AuctionID(auctionID)
	lot.WorldID = foundation.WorldID(worldID)
	lot.Payload = payload
	lot.Currency = economy.CurrencyBucket(currency)
	if buyNowPrice.Valid {
		value := buyNowPrice.Int64
		lot.BuyNowPrice = &value
	}
	lot.CurrentBidderID = foundation.PlayerID(currentBidderID)
	lot.Status = auction.LotStatus(status)
	lot.StartsAt = lot.StartsAt.UTC()
	lot.EndsAt = lot.EndsAt.UTC()
	lot.CreatedAt = lot.CreatedAt.UTC()
	lot.UpdatedAt = lot.UpdatedAt.UTC()
	if closedAt.Valid {
		value := closedAt.Time.UTC()
		lot.ClosedAt = &value
	}
	lot.WinningPlayerID = foundation.PlayerID(winningPlayerID)
	lot.CloseReason = auction.CloseReason(closeReason)
	if err := validateAuctionLotRow(lot); err != nil {
		return auction.Lot{}, err
	}
	return lot, nil
}

func validateAuctionLotRow(lot auction.Lot) error {
	if err := lot.AuctionID.Validate(); err != nil {
		return err
	}
	if err := lot.WorldID.Validate(); err != nil {
		return err
	}
	if err := validateAuctionPayload(lot.Payload); err != nil {
		return err
	}
	if err := lot.Currency.Validate(); err != nil {
		return err
	}
	if _, err := foundation.NewMoney(lot.StartPrice); err != nil {
		return err
	}
	if lot.BuyNowPrice != nil {
		if _, err := foundation.NewMoney(*lot.BuyNowPrice); err != nil {
			return err
		}
		if *lot.BuyNowPrice < lot.StartPrice {
			return fmt.Errorf("auction %q buy now %d start %d: %w", lot.AuctionID, *lot.BuyNowPrice, lot.StartPrice, auction.ErrBuyNowUnavailable)
		}
	}
	if lot.CurrentBid < 0 {
		return fmt.Errorf("auction %q current bid %d: %w", lot.AuctionID, lot.CurrentBid, foundation.ErrNonPositiveAmount)
	}
	if lot.CurrentBid > 0 {
		if _, err := foundation.NewMoney(lot.CurrentBid); err != nil {
			return err
		}
		if lot.CurrentBidderID.IsZero() {
			return fmt.Errorf("auction %q current bidder missing: %w", lot.AuctionID, auction.ErrInvalidLotStatus)
		}
	}
	if !lot.CurrentBidderID.IsZero() {
		if err := lot.CurrentBidderID.Validate(); err != nil {
			return err
		}
	}
	if err := lot.Status.Validate(); err != nil {
		return err
	}
	if lot.StartsAt.IsZero() || lot.EndsAt.IsZero() || !lot.EndsAt.After(lot.StartsAt) {
		return auction.ErrInvalidLotTiming
	}
	if lot.CreatedAt.IsZero() || lot.UpdatedAt.IsZero() {
		return fmt.Errorf("auction %q timestamp missing", lot.AuctionID)
	}
	if lot.ClosedAt != nil && lot.ClosedAt.IsZero() {
		return fmt.Errorf("auction %q closed timestamp missing", lot.AuctionID)
	}
	if !lot.WinningPlayerID.IsZero() {
		if err := lot.WinningPlayerID.Validate(); err != nil {
			return err
		}
	}
	if lot.CloseReason != "" {
		switch lot.CloseReason {
		case auction.CloseReasonEnded, auction.CloseReasonBuyNow, auction.CloseReasonNoBids:
		default:
			return fmt.Errorf("auction %q close reason %q: %w", lot.AuctionID, lot.CloseReason, auction.ErrInvalidLotStatus)
		}
	}
	return nil
}

func validateAuctionPayload(payload auction.LotPayload) error {
	if err := payload.Type.Validate(); err != nil {
		return err
	}
	if err := payload.Source.Validate(); err != nil {
		return err
	}
	if _, err := foundation.NewQuantity(payload.Quantity); err != nil {
		return fmt.Errorf("payload quantity: %w", err)
	}
	if len(payload.Metadata) > 0 && !json.Valid(payload.Metadata) {
		return fmt.Errorf("payload metadata: %w", auction.ErrInvalidLotPayload)
	}
	return nil
}

func auctionLotSelectSQL() string {
	return `
		SELECT
			auction_id,
			world_id,
			payload_json,
			currency_type,
			start_price,
			buy_now_price,
			current_bid,
			current_bidder_id,
			status,
			starts_at,
			ends_at,
			created_at,
			updated_at,
			closed_at,
			winning_player_id,
			close_reason
		FROM auction_lots
	`
}

func int64PointerAsNil(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

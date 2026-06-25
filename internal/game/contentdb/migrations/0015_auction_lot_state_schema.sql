CREATE TABLE IF NOT EXISTS auction_lots (
  auction_id text PRIMARY KEY CHECK (btrim(auction_id) <> ''),
  world_id text NOT NULL CHECK (btrim(world_id) <> ''),
  payload_json jsonb NOT NULL CHECK (jsonb_typeof(payload_json) = 'object'),
  currency_type text NOT NULL CHECK (btrim(currency_type) <> ''),
  start_price bigint NOT NULL CHECK (start_price > 0),
  buy_now_price bigint CHECK (buy_now_price IS NULL OR buy_now_price > 0),
  current_bid bigint NOT NULL DEFAULT 0 CHECK (current_bid >= 0),
  current_bidder_id text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('upcoming', 'active', 'closed', 'expired')),
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz,
  winning_player_id text NOT NULL DEFAULT '',
  close_reason text NOT NULL DEFAULT '',
  CHECK (ends_at > starts_at),
  CHECK (buy_now_price IS NULL OR buy_now_price >= start_price),
  CHECK ((current_bid = 0 AND current_bidder_id = '') OR (current_bid > 0 AND btrim(current_bidder_id) <> '')),
  CHECK (close_reason IN ('', 'ended', 'buy_now', 'no_bids'))
);

CREATE INDEX IF NOT EXISTS auction_lots_status_ends_idx
  ON auction_lots(status, ends_at);

CREATE INDEX IF NOT EXISTS auction_lots_world_status_idx
  ON auction_lots(world_id, status, updated_at);

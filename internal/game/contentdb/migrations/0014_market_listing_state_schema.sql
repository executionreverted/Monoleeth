CREATE TABLE IF NOT EXISTS market_listings (
  listing_id text PRIMARY KEY CHECK (btrim(listing_id) <> ''),
  seller_player_id text NOT NULL CHECK (btrim(seller_player_id) <> ''),
  item_definition_json jsonb NOT NULL CHECK (jsonb_typeof(item_definition_json) = 'object'),
  item_instance_id text NOT NULL DEFAULT '',
  item_id text NOT NULL CHECK (btrim(item_id) <> ''),
  original_quantity bigint NOT NULL CHECK (original_quantity > 0),
  remaining_quantity bigint NOT NULL CHECK (remaining_quantity >= 0),
  unit_price bigint NOT NULL CHECK (unit_price > 0),
  currency_type text NOT NULL CHECK (btrim(currency_type) <> ''),
  status text NOT NULL CHECK (status IN ('active', 'sold', 'cancelled', 'expired', 'stale', 'locked')),
  source_location_kind text NOT NULL CHECK (btrim(source_location_kind) <> ''),
  source_location_id text NOT NULL CHECK (btrim(source_location_id) <> ''),
  escrow_location_kind text NOT NULL CHECK (escrow_location_kind = 'market_escrow'),
  escrow_location_id text NOT NULL CHECK (btrim(escrow_location_id) <> ''),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz,
  stale_at timestamptz,
  stale_reason text NOT NULL DEFAULT '',
  CHECK (remaining_quantity <= original_quantity),
  CHECK (status <> 'stale' OR stale_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS market_listings_seller_status_idx
  ON market_listings(seller_player_id, status, updated_at);

CREATE INDEX IF NOT EXISTS market_listings_active_expiry_idx
  ON market_listings(expires_at)
  WHERE status = 'active' AND expires_at IS NOT NULL;

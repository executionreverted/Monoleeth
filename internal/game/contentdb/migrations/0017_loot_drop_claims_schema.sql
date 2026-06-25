CREATE TABLE IF NOT EXISTS loot_drop_claims (
  drop_id text PRIMARY KEY CHECK (btrim(drop_id) <> ''),
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  item_id text NOT NULL CHECK (btrim(item_id) <> ''),
  quantity bigint NOT NULL CHECK (quantity > 0),
  source_type text NOT NULL CHECK (btrim(source_type) <> ''),
  source_id text NOT NULL CHECK (btrim(source_id) <> ''),
  claimed_at timestamptz NOT NULL,
  payload_json jsonb NOT NULL
);

CREATE INDEX IF NOT EXISTS loot_drop_claims_player_claimed_idx
  ON loot_drop_claims(player_id, claimed_at);

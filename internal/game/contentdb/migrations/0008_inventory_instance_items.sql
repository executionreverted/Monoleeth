CREATE TABLE IF NOT EXISTS player_inventory_instance_items (
  item_instance_id text PRIMARY KEY CHECK (btrim(item_instance_id) <> ''),
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  item_id text NOT NULL CHECK (btrim(item_id) <> ''),
  location text NOT NULL CHECK (btrim(location) <> ''),
  location_kind text NOT NULL CHECK (btrim(location_kind) <> ''),
  location_id text NOT NULL CHECK (btrim(location_id) <> ''),
  quantity bigint NOT NULL DEFAULT 1 CHECK (quantity = 1),
  durability_current bigint NOT NULL DEFAULT 0 CHECK (durability_current >= 0),
  bound_state text NOT NULL DEFAULT 'unbound' CHECK (bound_state IN ('unbound', 'account_bound', 'soulbound')),
  source_definition_id text NOT NULL CHECK (btrim(source_definition_id) <> ''),
  source_version text NOT NULL CHECK (btrim(source_version) <> ''),
  metadata_json jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS player_inventory_instance_items_player_idx
  ON player_inventory_instance_items(player_id);

CREATE INDEX IF NOT EXISTS player_inventory_instance_items_location_idx
  ON player_inventory_instance_items(player_id, location_kind, location_id);

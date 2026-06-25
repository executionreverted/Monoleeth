CREATE TABLE IF NOT EXISTS player_loadouts (
  player_id text NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  loadout_id text NOT NULL CHECK (btrim(loadout_id) <> ''),
  ship_id text NOT NULL CHECK (btrim(ship_id) <> ''),
  name text NOT NULL CHECK (btrim(name) <> ''),
  slot_assignments_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(slot_assignments_json) = 'object'),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (player_id, loadout_id),
  FOREIGN KEY (player_id, ship_id) REFERENCES player_ships(player_id, ship_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS player_loadouts_player_ship_idx
  ON player_loadouts(player_id, ship_id);

CREATE TABLE IF NOT EXISTS player_equipped_modules (
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  ship_id text NOT NULL CHECK (btrim(ship_id) <> ''),
  slot_id text NOT NULL CHECK (btrim(slot_id) <> ''),
  item_instance_id text NOT NULL REFERENCES player_inventory_instance_items(item_instance_id) ON DELETE RESTRICT,
  equipped_at timestamptz NOT NULL,
  PRIMARY KEY (player_id, ship_id, slot_id),
  UNIQUE (item_instance_id),
  FOREIGN KEY (player_id, ship_id) REFERENCES player_ships(player_id, ship_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS player_equipped_modules_player_ship_idx
  ON player_equipped_modules(player_id, ship_id);

CREATE INDEX IF NOT EXISTS player_equipped_modules_item_idx
  ON player_equipped_modules(item_instance_id);

CREATE TABLE IF NOT EXISTS player_inventory_item_ledger (
  ledger_id text PRIMARY KEY CHECK (btrim(ledger_id) <> ''),
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  item_id text NOT NULL CHECK (btrim(item_id) <> ''),
  item_instance_id text NOT NULL DEFAULT '',
  quantity bigint NOT NULL CHECK (quantity > 0),
  action text NOT NULL CHECK (action IN ('increase', 'decrease')),
  balance_after bigint NOT NULL CHECK (balance_after >= 0),
  location text NOT NULL CHECK (btrim(location) <> ''),
  location_kind text NOT NULL CHECK (btrim(location_kind) <> ''),
  location_id text NOT NULL CHECK (btrim(location_id) <> ''),
  reason text NOT NULL CHECK (btrim(reason) <> ''),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS player_inventory_item_ledger_player_idx
  ON player_inventory_item_ledger(player_id, created_at);

CREATE INDEX IF NOT EXISTS player_inventory_item_ledger_reference_idx
  ON player_inventory_item_ledger(player_id, reference_key);

CREATE INDEX IF NOT EXISTS player_inventory_item_ledger_item_instance_idx
  ON player_inventory_item_ledger(item_instance_id)
  WHERE item_instance_id <> '';

CREATE TABLE IF NOT EXISTS player_inventory_add_item_references (
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  ledger_id text NOT NULL REFERENCES player_inventory_item_ledger(ledger_id) ON DELETE RESTRICT,
  item_instance_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(item_instance_ids) = 'array'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, reference_key)
);

CREATE INDEX IF NOT EXISTS player_inventory_add_item_references_ledger_idx
  ON player_inventory_add_item_references(ledger_id);

CREATE TABLE IF NOT EXISTS player_inventory_counters (
  counter_id text PRIMARY KEY CHECK (counter_id = 'inventory'),
  item_sequence bigint NOT NULL DEFAULT 0 CHECK (item_sequence >= 0),
  ledger_sequence bigint NOT NULL DEFAULT 0 CHECK (ledger_sequence >= 0),
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO player_inventory_counters(counter_id, item_sequence, ledger_sequence)
VALUES ('inventory', 0, 0)
ON CONFLICT (counter_id) DO NOTHING;

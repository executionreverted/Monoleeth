CREATE TABLE IF NOT EXISTS player_inventory_move_item_references (
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  primary_ledger_id text NOT NULL REFERENCES player_inventory_item_ledger(ledger_id) ON DELETE RESTRICT,
  ledger_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(ledger_ids) = 'array'),
  result_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, reference_key)
);

CREATE INDEX IF NOT EXISTS player_inventory_move_item_references_ledger_idx
  ON player_inventory_move_item_references(primary_ledger_id);

CREATE TABLE IF NOT EXISTS player_inventory_remove_item_references (
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  primary_ledger_id text NOT NULL REFERENCES player_inventory_item_ledger(ledger_id) ON DELETE RESTRICT,
  ledger_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(ledger_ids) = 'array'),
  result_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, reference_key)
);

CREATE INDEX IF NOT EXISTS player_inventory_remove_item_references_ledger_idx
  ON player_inventory_remove_item_references(primary_ledger_id);

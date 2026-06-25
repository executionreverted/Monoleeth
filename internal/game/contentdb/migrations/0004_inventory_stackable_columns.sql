ALTER TABLE player_inventory_items
  ADD COLUMN IF NOT EXISTS item_instance_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_definition_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_version text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS location_kind text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS location_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS metadata_json jsonb,
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

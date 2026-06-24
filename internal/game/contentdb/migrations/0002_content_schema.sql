CREATE TABLE IF NOT EXISTS content_versions (
  id uuid PRIMARY KEY,
  version text NOT NULL UNIQUE CHECK (btrim(version) <> ''),
  status text NOT NULL CHECK (status IN ('draft', 'published', 'archived', 'rolled_back')),
  is_current boolean NOT NULL DEFAULT false,
  idempotency_key text UNIQUE,
  snapshot_json jsonb NOT NULL CHECK (jsonb_typeof(snapshot_json) = 'object'),
  validation_report_json jsonb NOT NULL CHECK (jsonb_typeof(validation_report_json) = 'object'),
  notes text NOT NULL DEFAULT '',
  balance_tag text NOT NULL DEFAULT '',
  created_by text,
  created_at timestamptz NOT NULL DEFAULT now(),
  published_by text,
  published_at timestamptz,
  rolled_back_from uuid REFERENCES content_versions(id),
  CHECK (status <> 'published' OR published_at IS NOT NULL),
  CHECK (NOT is_current OR status = 'published')
);

CREATE UNIQUE INDEX IF NOT EXISTS content_versions_one_current
  ON content_versions (is_current)
  WHERE is_current;

CREATE INDEX IF NOT EXISTS content_versions_status_idx ON content_versions(status);
CREATE INDEX IF NOT EXISTS content_versions_created_at_idx ON content_versions(created_at);
CREATE INDEX IF NOT EXISTS content_versions_published_at_idx ON content_versions(published_at);

CREATE TABLE IF NOT EXISTS content_audit_log (
  id uuid PRIMARY KEY,
  content_version_id uuid REFERENCES content_versions(id),
  content_type text NOT NULL CHECK (btrim(content_type) <> ''),
  content_id text NOT NULL CHECK (btrim(content_id) <> ''),
  field_path text NOT NULL DEFAULT '',
  old_value_json jsonb,
  new_value_json jsonb,
  actor_account_id text,
  note text NOT NULL DEFAULT '',
  balance_tag text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS content_audit_log_version_idx ON content_audit_log(content_version_id);
CREATE INDEX IF NOT EXISTS content_audit_log_content_idx ON content_audit_log(content_type, content_id);
CREATE INDEX IF NOT EXISTS content_audit_log_created_at_idx ON content_audit_log(created_at);

CREATE TABLE IF NOT EXISTS content_items (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_modules (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_ships (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_shop_products (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_npc_templates (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_spawn_areas (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_enemy_pools (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_npc_drop_profiles (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_npc_aggro_profiles (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_npc_leash_profiles (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_npc_event_spawns (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_loot_tables (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_craft_recipes (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_production_buildings (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_quest_templates (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_quest_reward_tables (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE INDEX IF NOT EXISTS content_items_draft_version_idx ON content_items(draft_version);
CREATE INDEX IF NOT EXISTS content_modules_draft_version_idx ON content_modules(draft_version);
CREATE INDEX IF NOT EXISTS content_ships_draft_version_idx ON content_ships(draft_version);
CREATE INDEX IF NOT EXISTS content_shop_products_draft_version_idx ON content_shop_products(draft_version);
CREATE INDEX IF NOT EXISTS content_npc_templates_draft_version_idx ON content_npc_templates(draft_version);
CREATE INDEX IF NOT EXISTS content_spawn_areas_draft_version_idx ON content_spawn_areas(draft_version);
CREATE INDEX IF NOT EXISTS content_enemy_pools_draft_version_idx ON content_enemy_pools(draft_version);
CREATE INDEX IF NOT EXISTS content_npc_drop_profiles_draft_version_idx ON content_npc_drop_profiles(draft_version);
CREATE INDEX IF NOT EXISTS content_npc_aggro_profiles_draft_version_idx ON content_npc_aggro_profiles(draft_version);
CREATE INDEX IF NOT EXISTS content_npc_leash_profiles_draft_version_idx ON content_npc_leash_profiles(draft_version);
CREATE INDEX IF NOT EXISTS content_npc_event_spawns_draft_version_idx ON content_npc_event_spawns(draft_version);
CREATE INDEX IF NOT EXISTS content_loot_tables_draft_version_idx ON content_loot_tables(draft_version);
CREATE INDEX IF NOT EXISTS content_craft_recipes_draft_version_idx ON content_craft_recipes(draft_version);
CREATE INDEX IF NOT EXISTS content_production_buildings_draft_version_idx ON content_production_buildings(draft_version);
CREATE INDEX IF NOT EXISTS content_quest_templates_draft_version_idx ON content_quest_templates(draft_version);
CREATE INDEX IF NOT EXISTS content_quest_reward_tables_draft_version_idx ON content_quest_reward_tables(draft_version);

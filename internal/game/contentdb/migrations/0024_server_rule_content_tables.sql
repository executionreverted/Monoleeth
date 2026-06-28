CREATE TABLE IF NOT EXISTS content_scanner_configs (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_starter_configs (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_route_policies (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_production_rules (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_combat_rules (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE INDEX IF NOT EXISTS content_scanner_configs_draft_version_idx ON content_scanner_configs(draft_version);
CREATE INDEX IF NOT EXISTS content_starter_configs_draft_version_idx ON content_starter_configs(draft_version);
CREATE INDEX IF NOT EXISTS content_route_policies_draft_version_idx ON content_route_policies(draft_version);
CREATE INDEX IF NOT EXISTS content_production_rules_draft_version_idx ON content_production_rules(draft_version);
CREATE INDEX IF NOT EXISTS content_combat_rules_draft_version_idx ON content_combat_rules(draft_version);

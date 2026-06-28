CREATE TABLE IF NOT EXISTS content_maps (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE TABLE IF NOT EXISTS content_map_portals (
  content_id text PRIMARY KEY CHECK (btrim(content_id) <> ''),
  draft_version uuid REFERENCES content_versions(id),
  enabled boolean NOT NULL DEFAULT true,
  display_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(display_json) = 'object'),
  data_json jsonb NOT NULL CHECK (jsonb_typeof(data_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by text
);

CREATE INDEX IF NOT EXISTS content_maps_draft_version_idx ON content_maps(draft_version);
CREATE INDEX IF NOT EXISTS content_map_portals_draft_version_idx ON content_map_portals(draft_version);

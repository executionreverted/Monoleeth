-- P08: Durable automation route store (create/update/enable/disable lifecycle).
-- Two tables: one latest-record row per route_id (CAS revision), and one
-- dedup log row per reference_key. Apply checks reference dedup first, then
-- revision CAS on the route record, then commits both atomically.

CREATE TABLE IF NOT EXISTS automation_route_durable_records (
  route_id text PRIMARY KEY CHECK (btrim(route_id) <> ''),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  owner_player_id text NOT NULL CHECK (btrim(owner_player_id) <> ''),
  revision bigint NOT NULL CHECK (revision > 0),
  record_json jsonb NOT NULL CHECK (jsonb_typeof(record_json) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS automation_route_durable_records_owner_idx
  ON automation_route_durable_records(owner_player_id);

CREATE TABLE IF NOT EXISTS automation_route_durable_references (
  reference_key text PRIMARY KEY CHECK (btrim(reference_key) <> ''),
  route_id text NOT NULL CHECK (btrim(route_id) <> ''),
  revision bigint NOT NULL CHECK (revision > 0),
  record_json jsonb NOT NULL CHECK (jsonb_typeof(record_json) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT now()
);

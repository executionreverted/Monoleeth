CREATE TABLE IF NOT EXISTS idempotency_keys (
  scope text NOT NULL CHECK (btrim(scope) <> ''),
  idempotency_key text NOT NULL CHECK (btrim(idempotency_key) <> ''),
  operation text NOT NULL CHECK (btrim(operation) <> ''),
  player_id text NOT NULL DEFAULT '',
  request_hash text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed', 'failed')),
  result_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result_json) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  PRIMARY KEY (scope, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idempotency_keys_operation_player_idx
  ON idempotency_keys(operation, player_id);

CREATE INDEX IF NOT EXISTS idempotency_keys_status_updated_idx
  ON idempotency_keys(status, updated_at);

CREATE TABLE IF NOT EXISTS outbox (
  outbox_id text PRIMARY KEY CHECK (btrim(outbox_id) <> ''),
  topic text NOT NULL CHECK (btrim(topic) <> ''),
  event_type text NOT NULL CHECK (btrim(event_type) <> ''),
  aggregate_type text NOT NULL DEFAULT '',
  aggregate_id text NOT NULL DEFAULT '',
  idempotency_scope text NOT NULL DEFAULT '',
  idempotency_key text NOT NULL DEFAULT '',
  payload_json jsonb NOT NULL CHECK (jsonb_typeof(payload_json) = 'object'),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'leased', 'published', 'failed', 'dead')),
  available_at timestamptz NOT NULL DEFAULT now(),
  lease_owner text NOT NULL DEFAULT '',
  leased_until timestamptz,
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  max_attempts integer NOT NULL DEFAULT 20 CHECK (max_attempts > 0),
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  CHECK (status <> 'leased' OR (btrim(lease_owner) <> '' AND leased_until IS NOT NULL)),
  CHECK (status <> 'published' OR published_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS outbox_status_available_idx
  ON outbox(status, available_at, created_at)
  WHERE status IN ('pending', 'failed');

CREATE INDEX IF NOT EXISTS outbox_lease_idx
  ON outbox(leased_until)
  WHERE status = 'leased';

CREATE INDEX IF NOT EXISTS outbox_idempotency_idx
  ON outbox(idempotency_scope, idempotency_key)
  WHERE idempotency_key <> '';

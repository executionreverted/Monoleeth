CREATE TABLE IF NOT EXISTS premium_entitlements (
  entitlement_id text PRIMARY KEY CHECK (btrim(entitlement_id) <> ''),
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  entitlement_type text NOT NULL CHECK (entitlement_type IN ('premium_currency_pack', 'loadout_slot', 'weekly_x_core_purchase_right', 'cosmetic', 'badge')),
  state text NOT NULL CHECK (state IN ('pending', 'claimed', 'revoked')),
  provider_source text NOT NULL CHECK (btrim(provider_source) <> ''),
  provider_reference text NOT NULL CHECK (btrim(provider_reference) <> ''),
  payload_json jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  provider_confirmed_at timestamptz NOT NULL,
  claimed_at timestamptz,
  claim_request_ref text NOT NULL DEFAULT '',
  UNIQUE (provider_source, provider_reference),
  CHECK (provider_confirmed_at <= created_at),
  CHECK ((claimed_at IS NULL AND claim_request_ref = '') OR (claimed_at IS NOT NULL AND btrim(claim_request_ref) <> '')),
  CHECK (state <> 'claimed' OR claimed_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS premium_entitlements_player_state_idx
  ON premium_entitlements(player_id, state, created_at);

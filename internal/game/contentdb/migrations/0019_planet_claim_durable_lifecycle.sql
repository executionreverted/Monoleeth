-- P08: Durable planet claim lifecycle store.
-- One row per committed planet-claim lifecycle. The plan_json column carries
-- the validated ClaimDurableLifecyclePlan bundle (begin boundary, optional
-- production initialization, commit boundary/reference/event/outbox, optional
-- X Core consumption evidence) exactly as committed by the discovery domain.
-- claim_reference is the idempotency primary key: exact replays are duplicates,
-- conflicting plan reuse is rejected by the adapter before mutation.

CREATE TABLE IF NOT EXISTS claim_durable_lifecycles (
  claim_reference text PRIMARY KEY CHECK (btrim(claim_reference) <> ''),
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  planet_id text NOT NULL CHECK (btrim(planet_id) <> ''),
  plan_json jsonb NOT NULL CHECK (jsonb_typeof(plan_json) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS claim_durable_lifecycles_planet_idx
  ON claim_durable_lifecycles(planet_id);

CREATE INDEX IF NOT EXISTS claim_durable_lifecycles_player_idx
  ON claim_durable_lifecycles(player_id);

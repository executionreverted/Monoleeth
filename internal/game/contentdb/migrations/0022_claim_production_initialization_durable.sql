-- P08: Durable claim production initialization store.
-- Persists one validated production-initialization plan keyed by claim_reference.

CREATE TABLE IF NOT EXISTS claim_production_initialization_durable (
  claim_reference text PRIMARY KEY CHECK (btrim(claim_reference) <> ''),
  plan_json jsonb NOT NULL CHECK (jsonb_typeof(plan_json) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT now()
);

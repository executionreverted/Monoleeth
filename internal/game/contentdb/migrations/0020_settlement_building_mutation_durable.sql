-- P08: Durable production settlement and building mutation commit stores.
-- Each table persists one validated durable commit plan as opaque JSON keyed by
-- its idempotency reference. Exact replays are duplicates; conflicting plan
-- reuse for the same reference is rejected by the adapter before mutation.

CREATE TABLE IF NOT EXISTS building_mutation_durable_commits (
  reference_key text PRIMARY KEY CHECK (btrim(reference_key) <> ''),
  plan_json jsonb NOT NULL CHECK (jsonb_typeof(plan_json) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS settlement_durable_commits (
  reference_key text PRIMARY KEY CHECK (btrim(reference_key) <> ''),
  plan_json jsonb NOT NULL CHECK (jsonb_typeof(plan_json) = 'object'),
  committed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS player_wallet_ledger (
  ledger_id text PRIMARY KEY CHECK (btrim(ledger_id) <> ''),
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  currency_type text NOT NULL CHECK (btrim(currency_type) <> ''),
  amount bigint NOT NULL CHECK (amount > 0),
  action text NOT NULL CHECK (action IN ('increase', 'decrease')),
  balance_after bigint NOT NULL CHECK (balance_after >= 0),
  reason text NOT NULL CHECK (btrim(reason) <> ''),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS player_wallet_ledger_player_idx
  ON player_wallet_ledger(player_id, created_at);

CREATE INDEX IF NOT EXISTS player_wallet_ledger_reference_idx
  ON player_wallet_ledger(player_id, reference_key);

CREATE TABLE IF NOT EXISTS player_wallet_references (
  player_id text NOT NULL CHECK (btrim(player_id) <> ''),
  operation text NOT NULL CHECK (operation IN ('credit_wallet', 'debit_wallet', 'transfer_currency')),
  reference_key text NOT NULL CHECK (btrim(reference_key) <> ''),
  primary_ledger_id text NOT NULL REFERENCES player_wallet_ledger(ledger_id) ON DELETE RESTRICT,
  ledger_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(ledger_ids) = 'array'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, operation, reference_key)
);

CREATE INDEX IF NOT EXISTS player_wallet_references_primary_ledger_idx
  ON player_wallet_references(primary_ledger_id);

CREATE TABLE IF NOT EXISTS player_wallet_counters (
  counter_id text PRIMARY KEY CHECK (counter_id = 'wallet'),
  ledger_sequence bigint NOT NULL DEFAULT 0 CHECK (ledger_sequence >= 0),
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO player_wallet_counters(counter_id, ledger_sequence)
VALUES ('wallet', 0)
ON CONFLICT (counter_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS accounts (
  id text PRIMARY KEY CHECK (btrim(id) <> ''),
  email text NOT NULL CHECK (btrim(email) <> ''),
  password_hash text NOT NULL CHECK (btrim(password_hash) <> ''),
  roles text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS players (
  id text PRIMARY KEY CHECK (btrim(id) <> ''),
  account_id text NOT NULL REFERENCES accounts(id),
  callsign text NOT NULL CHECK (btrim(callsign) <> ''),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id)
);

CREATE INDEX IF NOT EXISTS players_account_idx ON players(account_id);

CREATE TABLE IF NOT EXISTS auth_sessions (
  id text PRIMARY KEY CHECK (btrim(id) <> ''),
  account_id text NOT NULL REFERENCES accounts(id),
  player_id text NOT NULL REFERENCES players(id),
  token_hash text NOT NULL CHECK (btrim(token_hash) <> ''),
  roles text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS auth_sessions_account_idx ON auth_sessions(account_id);
CREATE INDEX IF NOT EXISTS auth_sessions_player_idx ON auth_sessions(player_id);
CREATE INDEX IF NOT EXISTS auth_sessions_expires_at_idx ON auth_sessions(expires_at);

CREATE TABLE IF NOT EXISTS player_wallets (
  player_id text NOT NULL REFERENCES players(id),
  currency_type text NOT NULL CHECK (btrim(currency_type) <> ''),
  balance bigint NOT NULL DEFAULT 0 CHECK (balance >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, currency_type)
);

CREATE TABLE IF NOT EXISTS player_inventory_items (
  player_id text NOT NULL REFERENCES players(id),
  location text NOT NULL CHECK (btrim(location) <> ''),
  item_id text NOT NULL CHECK (btrim(item_id) <> ''),
  quantity bigint NOT NULL DEFAULT 0 CHECK (quantity >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, location, item_id)
);

CREATE INDEX IF NOT EXISTS player_inventory_items_player_idx ON player_inventory_items(player_id);

CREATE TABLE IF NOT EXISTS player_progression (
  player_id text PRIMARY KEY REFERENCES players(id),
  main_xp bigint NOT NULL DEFAULT 0 CHECK (main_xp >= 0),
  main_level integer NOT NULL DEFAULT 1 CHECK (main_level >= 1),
  rank text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS player_role_levels (
  player_id text NOT NULL REFERENCES players(id),
  role_type text NOT NULL CHECK (btrim(role_type) <> ''),
  level integer NOT NULL DEFAULT 0 CHECK (level >= 0),
  xp bigint NOT NULL DEFAULT 0 CHECK (xp >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, role_type)
);

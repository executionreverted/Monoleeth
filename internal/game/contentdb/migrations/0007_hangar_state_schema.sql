CREATE TABLE IF NOT EXISTS player_ships (
  player_id text NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ship_id text NOT NULL CHECK (btrim(ship_id) <> ''),
  unlocked_at timestamptz NOT NULL DEFAULT now(),
  state text NOT NULL CHECK (state IN ('available', 'active', 'disabled', 'repairing', 'locked')),
  disabled_reason text NOT NULL DEFAULT '',
  disabled_at timestamptz,
  last_repaired_at timestamptz,
  metadata_json jsonb,
  PRIMARY KEY (player_id, ship_id)
);

CREATE INDEX IF NOT EXISTS player_ships_player_idx ON player_ships(player_id);
CREATE INDEX IF NOT EXISTS player_ships_state_idx ON player_ships(state);

CREATE TABLE IF NOT EXISTS player_active_ships (
  player_id text PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
  ship_id text NOT NULL CHECK (btrim(ship_id) <> ''),
  activated_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  FOREIGN KEY (player_id, ship_id) REFERENCES player_ships(player_id, ship_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS player_active_ships_ship_idx ON player_active_ships(ship_id);

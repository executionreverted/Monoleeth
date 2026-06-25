ALTER TABLE player_progression
  ADD COLUMN IF NOT EXISTS rank_level integer NOT NULL DEFAULT 1 CHECK (rank_level >= 1);

CREATE TABLE IF NOT EXISTS player_skill_points (
  player_id text PRIMARY KEY REFERENCES players(id),
  total_points integer NOT NULL DEFAULT 0 CHECK (total_points >= 0),
  spent_points integer NOT NULL DEFAULT 0 CHECK (spent_points >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (spent_points <= total_points)
);

CREATE TABLE IF NOT EXISTS player_unlocked_skill_nodes (
  player_id text NOT NULL REFERENCES players(id),
  node_id text NOT NULL CHECK (btrim(node_id) <> ''),
  unlocked_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (player_id, node_id)
);

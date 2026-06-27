CREATE TABLE IF NOT EXISTS social_clans (
    clan_id TEXT PRIMARY KEY CHECK (btrim(clan_id) <> ''),
    name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 3 AND 32),
    tag TEXT NOT NULL UNIQUE CHECK (char_length(btrim(tag)) BETWEEN 3 AND 5),
    owner_player_id TEXT NOT NULL CHECK (btrim(owner_player_id) <> ''),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS social_clan_memberships (
    clan_id TEXT NOT NULL REFERENCES social_clans(clan_id) ON DELETE CASCADE,
    player_id TEXT PRIMARY KEY CHECK (btrim(player_id) <> ''),
    rank TEXT NOT NULL CHECK (rank IN ('owner', 'officer', 'member')),
    joined_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_social_clan_memberships_clan_joined
    ON social_clan_memberships(clan_id, joined_at);

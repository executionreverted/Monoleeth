package contentdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/social"
)

type SocialClanStore struct {
	store *Store
}

var _ social.ClanStore = (*SocialClanStore)(nil)

func NewSocialClanStore(store *Store) (*SocialClanStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &SocialClanStore{store: store}, nil
}

func (store *SocialClanStore) CreateClan(clan social.Clan, ownerMembership social.ClanMembership) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	ctx := context.Background()
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)

	if exists, err := socialClanRowExists(ctx, tx, `SELECT 1 FROM social_clan_memberships WHERE player_id = $1`, ownerMembership.PlayerID.String()); err != nil {
		return err
	} else if exists {
		return social.ErrAlreadyInClan
	}
	if exists, err := socialClanRowExists(ctx, tx, `SELECT 1 FROM social_clans WHERE tag = $1`, string(clan.Tag)); err != nil {
		return err
	} else if exists {
		return social.ErrClanAlreadyExists
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO social_clans(clan_id, name, tag, owner_player_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, string(clan.ClanID), clan.Name, string(clan.Tag), clan.OwnerID.String(), clan.CreatedAt.UTC()); err != nil {
		return mapSocialClanWriteError(err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO social_clan_memberships(clan_id, player_id, rank, joined_at)
		VALUES ($1, $2, $3, $4)
	`, string(ownerMembership.ClanID), ownerMembership.PlayerID.String(), string(ownerMembership.Rank), ownerMembership.JoinedAt.UTC()); err != nil {
		return mapSocialClanWriteError(err)
	}
	err = tx.Commit()
	return err
}

func (store *SocialClanStore) Clan(clanID social.ClanID) (social.Clan, bool, error) {
	return store.clanByLookup(`WHERE clan_id = $1`, string(clanID))
}

func (store *SocialClanStore) ClanByTag(tag social.ClanTag) (social.Clan, bool, error) {
	return store.clanByLookup(`WHERE tag = $1`, string(tag))
}

func (store *SocialClanStore) Memberships(clanID social.ClanID) ([]social.ClanMembership, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(context.Background(), `
		SELECT clan_id, player_id, rank, joined_at
		FROM social_clan_memberships
		WHERE clan_id = $1
		ORDER BY joined_at ASC, player_id ASC
	`, string(clanID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memberships []social.ClanMembership
	for rows.Next() {
		membership, err := scanSocialClanMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memberships, nil
}

func (store *SocialClanStore) Membership(playerID foundation.PlayerID) (social.ClanMembership, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return social.ClanMembership{}, false, ErrNilDatabase
	}
	row := store.store.db.QueryRowContext(context.Background(), `
		SELECT clan_id, player_id, rank, joined_at
		FROM social_clan_memberships
		WHERE player_id = $1
	`, playerID.String())
	membership, err := scanSocialClanMembership(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return social.ClanMembership{}, false, nil
		}
		return social.ClanMembership{}, false, err
	}
	return membership, true, nil
}

func (store *SocialClanStore) AddMembership(membership social.ClanMembership) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	ctx := context.Background()
	existing, ok, err := store.Membership(membership.PlayerID)
	if err != nil {
		return err
	}
	if ok && existing.ClanID != membership.ClanID {
		return social.ErrAlreadyInClan
	}
	if ok {
		_, err := store.store.db.ExecContext(ctx, `
			UPDATE social_clan_memberships
			SET rank = $3,
			    joined_at = $4
			WHERE clan_id = $1 AND player_id = $2
		`, string(membership.ClanID), membership.PlayerID.String(), string(membership.Rank), membership.JoinedAt.UTC())
		return err
	}
	_, err = store.store.db.ExecContext(ctx, `
		INSERT INTO social_clan_memberships(clan_id, player_id, rank, joined_at)
		VALUES ($1, $2, $3, $4)
	`, string(membership.ClanID), membership.PlayerID.String(), string(membership.Rank), membership.JoinedAt.UTC())
	return mapSocialClanWriteError(err)
}

func (store *SocialClanStore) RemoveMembership(playerID foundation.PlayerID) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	_, err := store.store.db.ExecContext(context.Background(), `
		DELETE FROM social_clan_memberships
		WHERE player_id = $1
	`, playerID.String())
	return err
}

func (store *SocialClanStore) SetOwner(clanID social.ClanID, ownerID foundation.PlayerID) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	result, err := store.store.db.ExecContext(context.Background(), `
		UPDATE social_clans
		SET owner_player_id = $2
		WHERE clan_id = $1
	`, string(clanID), ownerID.String())
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return social.ErrClanNotFound
	}
	return nil
}

func (store *SocialClanStore) clanByLookup(where string, arg any) (social.Clan, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return social.Clan{}, false, ErrNilDatabase
	}
	row := store.store.db.QueryRowContext(context.Background(), `
		SELECT clan_id, name, tag, owner_player_id, created_at
		FROM social_clans
		`+where+`
	`, arg)
	var clan social.Clan
	var clanID string
	var tag string
	var ownerID string
	if err := row.Scan(&clanID, &clan.Name, &tag, &ownerID, &clan.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return social.Clan{}, false, nil
		}
		return social.Clan{}, false, err
	}
	clan.ClanID = social.ClanID(clanID)
	clan.Tag = social.ClanTag(tag)
	clan.OwnerID = foundation.PlayerID(ownerID)
	clan.CreatedAt = clan.CreatedAt.UTC()
	return clan, true, nil
}

type socialClanMembershipScanner interface {
	Scan(dest ...any) error
}

func scanSocialClanMembership(scanner socialClanMembershipScanner) (social.ClanMembership, error) {
	var membership social.ClanMembership
	var clanID string
	var playerID string
	var rank string
	if err := scanner.Scan(&clanID, &playerID, &rank, &membership.JoinedAt); err != nil {
		return social.ClanMembership{}, err
	}
	membership.ClanID = social.ClanID(clanID)
	membership.PlayerID = foundation.PlayerID(playerID)
	membership.Rank = social.ClanRank(rank)
	membership.JoinedAt = membership.JoinedAt.UTC()
	if err := social.ValidateClanRank(membership.Rank); err != nil {
		return social.ClanMembership{}, fmt.Errorf("stored clan rank: %w", err)
	}
	return membership, nil
}

func socialClanRowExists(ctx context.Context, tx *sql.Tx, query string, arg any) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, query, arg).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func mapSocialClanWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "social_clans_tag_key":
			return social.ErrClanAlreadyExists
		case "social_clan_memberships_pkey":
			return social.ErrAlreadyInClan
		}
	}
	return err
}

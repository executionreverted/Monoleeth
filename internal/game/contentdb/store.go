package contentdb

import (
	"context"
	"database/sql"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}
	return &Store{db: db}, nil
}

func (store *Store) Migrate(ctx context.Context, mode MigrationMode) error {
	if store == nil || store.db == nil {
		return ErrNilDatabase
	}
	return RunMigrations(ctx, store.db, mode)
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

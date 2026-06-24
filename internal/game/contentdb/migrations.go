package contentdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version  string
	SQL      string
	Checksum string
}

type MigrationStore interface {
	AppliedMigrations(ctx context.Context) (map[string]string, error)
	ApplyMigration(ctx context.Context, migration Migration) error
}

func EmbeddedMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		sqlBytes, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		version := strings.TrimSuffix(entry.Name(), ".sql")
		body := string(sqlBytes)
		migrations = append(migrations, Migration{
			Version:  version,
			SQL:      body,
			Checksum: checksum(body),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func ApplyMigrations(ctx context.Context, store MigrationStore, migrations []Migration, mode MigrationMode) error {
	if mode == "" {
		mode = MigrationModeAuto
	}
	if mode == MigrationModeOff {
		return nil
	}
	if store == nil {
		return ErrNilDatabase
	}
	applied, err := store.AppliedMigrations(ctx)
	if err != nil {
		return err
	}
	pending, err := PendingMigrations(applied, migrations)
	if err != nil {
		return err
	}
	if len(pending) > 0 && mode == MigrationModeVerify {
		return fmt.Errorf("%w: %d pending", ErrPendingMigrations, len(pending))
	}
	for _, migration := range pending {
		if err := store.ApplyMigration(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func PendingMigrations(applied map[string]string, migrations []Migration) ([]Migration, error) {
	applied = copyApplied(applied)
	pending := make([]Migration, 0, len(migrations))
	for _, migration := range migrations {
		if migration.Version == "" || migration.Checksum == "" {
			return nil, fmt.Errorf("%w: %s", ErrMissingMigrationChecksum, migration.Version)
		}
		if existing, ok := applied[migration.Version]; ok {
			if existing != migration.Checksum {
				return nil, fmt.Errorf("%w: %s", ErrMigrationChecksumMismatch, migration.Version)
			}
			continue
		}
		pending = append(pending, migration)
	}
	return pending, nil
}

type SQLMigrationStore struct {
	db *sql.DB
}

func NewSQLMigrationStore(db *sql.DB) (*SQLMigrationStore, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}
	return &SQLMigrationStore{db: db}, nil
}

func RunMigrations(ctx context.Context, db *sql.DB, mode MigrationMode) error {
	if mode == "" {
		mode = MigrationModeAuto
	}
	if mode == MigrationModeOff {
		return nil
	}
	store, err := NewSQLMigrationStore(db)
	if err != nil {
		return err
	}
	if err := store.ensureTable(ctx); err != nil {
		return err
	}
	migrations, err := EmbeddedMigrations()
	if err != nil {
		return err
	}
	return ApplyMigrations(ctx, store, migrations, mode)
}

func (store *SQLMigrationStore) AppliedMigrations(ctx context.Context) (map[string]string, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := make(map[string]string)
	for rows.Next() {
		var version string
		var sum string
		if err := rows.Scan(&version, &sum); err != nil {
			return nil, err
		}
		applied[version] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

func (store *SQLMigrationStore) ApplyMigration(ctx context.Context, migration Migration) error {
	if migration.Version == "" || migration.Checksum == "" {
		return fmt.Errorf("%w: %s", ErrMissingMigrationChecksum, migration.Version)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if _, err = tx.ExecContext(ctx, migration.SQL); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO schema_migrations(version, checksum)
		VALUES ($1, $2)
		ON CONFLICT (version) DO NOTHING
	`, migration.Version, migration.Checksum); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (store *SQLMigrationStore) ensureTable(ctx context.Context) error {
	migrations, err := EmbeddedMigrations()
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return nil
	}
	_, err = store.db.ExecContext(ctx, migrations[0].SQL)
	return err
}

func rollbackUnlessCommitted(tx *sql.Tx, errp *error) {
	if *errp == nil {
		return
	}
	if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		*errp = fmt.Errorf("%w; rollback: %v", *errp, rollbackErr)
	}
}

func copyApplied(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func checksum(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

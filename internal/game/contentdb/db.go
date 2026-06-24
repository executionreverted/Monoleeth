package contentdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const postgresDriverName = "pgx"

func Open(ctx context.Context, config Config) (*sql.DB, error) {
	config = config.WithDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled() {
		return nil, ErrContentDatabaseDisabled
	}
	db, err := sql.Open(postgresDriverName, config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open content db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping content db: %w", err)
	}
	return db, nil
}

func IsDisabled(err error) bool {
	return errors.Is(err, ErrContentDatabaseDisabled)
}

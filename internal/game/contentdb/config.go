package contentdb

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	EnvDatabaseURL = "GAME_CONTENT_DATABASE_URL"
	EnvMode        = "GAME_CONTENT_MODE"
	EnvMigrations  = "GAME_CONTENT_MIGRATIONS"
)

type ContentMode string

const (
	ContentModeOff         ContentMode = "off"
	ContentModeRequired    ContentMode = "required"
	ContentModeDevFallback ContentMode = "dev_fallback"
)

type MigrationMode string

const (
	MigrationModeOff    MigrationMode = "off"
	MigrationModeAuto   MigrationMode = "auto"
	MigrationModeVerify MigrationMode = "verify"
)

var (
	ErrMissingDatabaseURL        = errors.New("missing content database url")
	ErrInvalidContentMode        = errors.New("invalid content mode")
	ErrInvalidMigrationMode      = errors.New("invalid content migration mode")
	ErrContentDatabaseDisabled   = errors.New("content database disabled")
	ErrNilDatabase               = errors.New("nil content database")
	ErrMissingMigrationChecksum  = errors.New("missing migration checksum")
	ErrMigrationChecksumMismatch = errors.New("migration checksum mismatch")
	ErrPendingMigrations         = errors.New("pending content migrations")
	ErrCurrentContentNotFound    = errors.New("current published content not found")
	ErrContentPublishConflict    = errors.New("content publish conflict")
	ErrUnknownContentType        = errors.New("unknown content type")
	ErrUnknownAuditAction        = errors.New("unknown content audit action")
)

type Config struct {
	DatabaseURL string
	Mode        ContentMode
	Migrations  MigrationMode
}

func FromEnv() Config {
	return Config{
		DatabaseURL: strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		Mode:        ContentMode(strings.TrimSpace(os.Getenv(EnvMode))),
		Migrations:  MigrationMode(strings.TrimSpace(os.Getenv(EnvMigrations))),
	}.WithDefaults()
}

func (config Config) WithDefaults() Config {
	if config.Mode == "" {
		if strings.TrimSpace(config.DatabaseURL) == "" {
			config.Mode = ContentModeOff
		} else {
			config.Mode = ContentModeRequired
		}
	}
	if config.Migrations == "" {
		if config.Mode == ContentModeOff {
			config.Migrations = MigrationModeOff
		} else {
			config.Migrations = MigrationModeAuto
		}
	}
	config.DatabaseURL = strings.TrimSpace(config.DatabaseURL)
	return config
}

func (config Config) Enabled() bool {
	config = config.WithDefaults()
	return config.Mode != ContentModeOff && config.DatabaseURL != ""
}

func (config Config) Validate() error {
	config = config.WithDefaults()
	switch config.Mode {
	case ContentModeOff, ContentModeRequired, ContentModeDevFallback:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidContentMode, config.Mode)
	}
	switch config.Migrations {
	case MigrationModeOff, MigrationModeAuto, MigrationModeVerify:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMigrationMode, config.Migrations)
	}
	if config.Mode == ContentModeRequired && config.DatabaseURL == "" {
		return fmt.Errorf("%w: %s is required when %s=%s", ErrMissingDatabaseURL, EnvDatabaseURL, EnvMode, ContentModeRequired)
	}
	return nil
}

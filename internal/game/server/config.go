package server

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world/worker"
)

const (
	EnvServerAddr          = "GAME_SERVER_ADDR"
	EnvAllowedOrigins      = "GAME_ALLOWED_ORIGINS"
	EnvAllowMissingOrigin  = "GAME_ALLOW_MISSING_ORIGIN"
	EnvCookieSecure        = "GAME_COOKIE_SECURE"
	EnvDevMode             = "GAME_DEV_MODE"
	defaultServerAddr      = ":8080"
	defaultSocketReadLimit = 64 * 1024
)

// Config controls the concrete single-process game server.
type Config struct {
	Addr               string
	AllowedOrigins     []string
	AllowMissingOrigin bool
	CookieSecure       bool
	DevMode            bool
	SessionTTL         time.Duration
	SocketReadTimeout  time.Duration
	SocketWriteTimeout time.Duration
	SocketReadLimit    int64
	TickDelta          time.Duration
	WorldID            foundation.WorldID
	ZoneID             foundation.ZoneID
	AdminSeed          auth.AdminSeedInput
	PasswordHasher     auth.PasswordHasher
	Clock              foundation.Clock
}

// DefaultConfig returns local-safe server defaults.
func DefaultConfig() Config {
	return Config{
		Addr:               defaultServerAddr,
		AllowedOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		SessionTTL:         24 * time.Hour,
		SocketReadTimeout:  30 * time.Second,
		SocketWriteTimeout: 10 * time.Second,
		SocketReadLimit:    defaultSocketReadLimit,
		TickDelta:          worker.DefaultTickDelta,
		WorldID:            "world-1",
		ZoneID:             "zone-1",
	}
}

// ConfigFromEnv returns Config with GAME_* environment overrides.
func ConfigFromEnv() Config {
	config := DefaultConfig()
	if value := strings.TrimSpace(os.Getenv(EnvServerAddr)); value != "" {
		config.Addr = value
	}
	if value := strings.TrimSpace(os.Getenv(EnvAllowedOrigins)); value != "" {
		config.AllowedOrigins = splitCSV(value)
	}
	config.AllowMissingOrigin = envBool(EnvAllowMissingOrigin, config.AllowMissingOrigin)
	config.CookieSecure = envBool(EnvCookieSecure, config.CookieSecure)
	config.DevMode = envBool(EnvDevMode, config.DevMode)
	config.AdminSeed = auth.AdminSeedInput{
		Enabled:  os.Getenv(auth.EnvAdminEmail) != "" || os.Getenv(auth.EnvAdminPassword) != "",
		Email:    os.Getenv(auth.EnvAdminEmail),
		Password: os.Getenv(auth.EnvAdminPassword),
		Callsign: os.Getenv(auth.EnvAdminCallsign),
	}
	return config
}

func (config Config) originPolicy() auth.OriginPolicy {
	return auth.OriginPolicy{
		AllowedOrigins:     append([]string(nil), config.AllowedOrigins...),
		AllowMissingOrigin: config.AllowMissingOrigin,
	}
}

func (config Config) withDefaults() Config {
	defaults := DefaultConfig()
	if config.Addr == "" {
		config.Addr = defaults.Addr
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = defaults.SessionTTL
	}
	if config.SocketReadTimeout <= 0 {
		config.SocketReadTimeout = defaults.SocketReadTimeout
	}
	if config.SocketWriteTimeout <= 0 {
		config.SocketWriteTimeout = defaults.SocketWriteTimeout
	}
	if config.SocketReadLimit <= 0 {
		config.SocketReadLimit = defaults.SocketReadLimit
	}
	if config.TickDelta <= 0 {
		config.TickDelta = defaults.TickDelta
	}
	if config.WorldID == "" {
		config.WorldID = defaults.WorldID
	}
	if config.ZoneID == "" {
		config.ZoneID = defaults.ZoneID
	}
	return config
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

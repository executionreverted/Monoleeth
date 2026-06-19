package auth

import (
	"context"
	"errors"
	"os"

	"gameproject/internal/game/foundation"
)

const (
	EnvAdminEmail    = "GAME_ADMIN_EMAIL"
	EnvAdminPassword = "GAME_ADMIN_PASSWORD"
	EnvAdminCallsign = "GAME_ADMIN_CALLSIGN"
)

// AdminSeedInput configures the reproducible local admin seed path.
type AdminSeedInput struct {
	Enabled  bool
	Email    string
	Password string
	Callsign string
}

// AdminSeedResult reports whether the seed created or updated an admin account.
type AdminSeedResult struct {
	Applied   bool
	Created   bool
	AccountID foundation.AccountID
	PlayerID  foundation.PlayerID
	Email     Email
}

// SeedAdminFromEnv applies the admin seed when GAME_ADMIN_* values request it.
func (service *Service) SeedAdminFromEnv(ctx context.Context) (AdminSeedResult, error) {
	return service.SeedAdmin(ctx, AdminSeedInput{
		Enabled:  os.Getenv(EnvAdminEmail) != "" || os.Getenv(EnvAdminPassword) != "",
		Email:    os.Getenv(EnvAdminEmail),
		Password: os.Getenv(EnvAdminPassword),
		Callsign: os.Getenv(EnvAdminCallsign),
	})
}

// SeedAdmin creates or updates an admin account from explicit inputs.
func (service *Service) SeedAdmin(ctx context.Context, input AdminSeedInput) (AdminSeedResult, error) {
	if service == nil {
		return AdminSeedResult{}, ErrNilAuthService
	}
	if !input.Enabled {
		return AdminSeedResult{}, nil
	}
	if input.Email == "" || input.Password == "" {
		return AdminSeedResult{}, ErrMissingAdminSeedInput
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return AdminSeedResult{}, invalidAuthPayload("Admin email is invalid.", err)
	}
	callsign := input.Callsign
	if callsign == "" {
		callsign = "Admin"
	}
	normalizedCallsign, err := ValidateCallsign(callsign)
	if err != nil {
		return AdminSeedResult{}, invalidAuthPayload("Admin callsign is invalid.", err)
	}
	passwordHash, err := service.passwords.HashPassword(input.Password)
	if err != nil {
		return AdminSeedResult{}, invalidAuthPayload("Admin password is invalid.", err)
	}
	now := service.now()
	account, player, err := service.store.AccountByEmail(ctx, email)
	if err == nil {
		account.PasswordHash = passwordHash
		account.Roles = mergeRoles(account.Roles, RoleAdmin)
		account.UpdatedAt = now
		if err := service.store.UpdateAccount(ctx, account); err != nil {
			return AdminSeedResult{}, err
		}
		return AdminSeedResult{
			Applied:   true,
			Created:   false,
			AccountID: account.ID,
			PlayerID:  player.ID,
			Email:     account.Email,
		}, nil
	}
	if !errors.Is(err, ErrAccountNotFound) {
		return AdminSeedResult{}, err
	}
	accountID, playerID, err := service.newAccountIDs()
	if err != nil {
		return AdminSeedResult{}, err
	}
	account = Account{
		ID:           accountID,
		Email:        email,
		PasswordHash: passwordHash,
		Roles:        []Role{RoleAdmin},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	player = PlayerProfile{
		ID:        playerID,
		AccountID: accountID,
		Callsign:  normalizedCallsign,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.InsertAccount(ctx, account, player); err != nil {
		return AdminSeedResult{}, err
	}
	return AdminSeedResult{
		Applied:   true,
		Created:   true,
		AccountID: account.ID,
		PlayerID:  player.ID,
		Email:     account.Email,
	}, nil
}

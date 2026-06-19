package auth

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordHashAlgorithm = "pbkdf2-sha256"
	passwordHashVersion   = "v1"
	defaultPBKDF2Iter     = 210_000
	defaultSaltBytes      = 16
	defaultKeyBytes       = 32
	minPasswordBytes      = 8
)

// PasswordHash stores a versioned password verifier. It is not safe to expose
// to the browser.
type PasswordHash string

// PasswordHasher hashes and verifies plaintext passwords.
type PasswordHasher interface {
	HashPassword(password string) (PasswordHash, error)
	VerifyPassword(password string, encoded PasswordHash) (bool, error)
}

// PBKDF2PasswordHasher is the dependency-free MVP password hasher.
type PBKDF2PasswordHasher struct {
	Iterations int
	SaltBytes  int
	KeyBytes   int
}

// HashPassword returns a salted PBKDF2-SHA256 password hash.
func (hasher PBKDF2PasswordHasher) HashPassword(password string) (PasswordHash, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	params := hasher.params()
	salt, err := randomBytes(params.SaltBytes)
	if err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, params.Iterations, params.KeyBytes)
	if err != nil {
		return "", fmt.Errorf("derive password hash: %w", err)
	}
	return PasswordHash(strings.Join([]string{
		passwordHashAlgorithm,
		passwordHashVersion,
		strconv.Itoa(params.Iterations),
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	}, "$")), nil
}

// VerifyPassword checks password against encoded using constant-time key
// comparison once the verifier has been parsed.
func (hasher PBKDF2PasswordHasher) VerifyPassword(password string, encoded PasswordHash) (bool, error) {
	if err := validatePassword(password); err != nil {
		return false, err
	}
	parts := strings.Split(string(encoded), "$")
	if len(parts) != 5 || parts[0] != passwordHashAlgorithm || parts[1] != passwordHashVersion {
		return false, ErrInvalidPasswordHash
	}
	iterations, err := strconv.Atoi(parts[2])
	if err != nil || iterations <= 0 {
		return false, ErrInvalidPasswordHash
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(salt) == 0 {
		return false, ErrInvalidPasswordHash
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || len(want) == 0 {
		return false, ErrInvalidPasswordHash
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false, fmt.Errorf("derive password hash: %w", err)
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func (hasher PBKDF2PasswordHasher) params() PBKDF2PasswordHasher {
	if hasher.Iterations <= 0 {
		hasher.Iterations = defaultPBKDF2Iter
	}
	if hasher.SaltBytes <= 0 {
		hasher.SaltBytes = defaultSaltBytes
	}
	if hasher.KeyBytes <= 0 {
		hasher.KeyBytes = defaultKeyBytes
	}
	return hasher
}

func validatePassword(password string) error {
	if len(password) < minPasswordBytes {
		return ErrInvalidPassword
	}
	if strings.ContainsAny(password, "\x00\r\n") {
		return ErrInvalidPassword
	}
	return nil
}

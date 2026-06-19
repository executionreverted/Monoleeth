package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	defaultTokenBytes = 32
	defaultIDBytes    = 12
)

// TokenGenerator creates unguessable ids and session tokens.
type TokenGenerator interface {
	NewSessionToken() (string, error)
	NewID(prefix string) (string, error)
}

// RandomTokenGenerator uses crypto/rand for opaque auth tokens and ids.
type RandomTokenGenerator struct {
	TokenBytes int
	IDBytes    int
}

// NewSessionToken returns an opaque raw token suitable for an HttpOnly cookie.
func (generator RandomTokenGenerator) NewSessionToken() (string, error) {
	size := generator.TokenBytes
	if size <= 0 {
		size = defaultTokenBytes
	}
	token, err := randomBase64(size)
	if err != nil {
		return "", err
	}
	return token, nil
}

// NewID returns a stable prefixed id for server-owned auth records.
func (generator RandomTokenGenerator) NewID(prefix string) (string, error) {
	size := generator.IDBytes
	if size <= 0 {
		size = defaultIDBytes
	}
	value, err := randomBase64(size)
	if err != nil {
		return "", err
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return value, nil
	}
	return prefix + "_" + value, nil
}

func tokenHash(rawToken string) (string, error) {
	if strings.TrimSpace(rawToken) == "" || strings.ContainsAny(rawToken, " \t\r\n") {
		return "", ErrInvalidSessionToken
	}
	sum := sha256.Sum256([]byte(rawToken))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func randomBase64(size int) (string, error) {
	bytes, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomBytes(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("random bytes size %d: %w", size, ErrInvalidSessionToken)
	}
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("read secure random: %w", err)
	}
	return bytes, nil
}

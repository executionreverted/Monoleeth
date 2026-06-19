package auth

import "testing"

func TestPasswordHashVerifiesCorrectPasswordAndRejectsWrongPassword(t *testing.T) {
	hasher := PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16}

	hash, err := hasher.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v, want nil", err)
	}
	if hash == "" || string(hash) == "correct-password" {
		t.Fatalf("hash = %q, want encoded verifier without plaintext", hash)
	}
	ok, err := hasher.VerifyPassword("correct-password", hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(correct) = %v, %v; want true, nil", ok, err)
	}
	ok, err = hasher.VerifyPassword("wrong-password", hash)
	if err != nil || ok {
		t.Fatalf("VerifyPassword(wrong) = %v, %v; want false, nil", ok, err)
	}
}

func TestPasswordHashRejectsInvalidPasswordAndMalformedHash(t *testing.T) {
	hasher := PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16}

	if _, err := hasher.HashPassword("short"); err != ErrInvalidPassword {
		t.Fatalf("HashPassword(short) error = %v, want ErrInvalidPassword", err)
	}
	if ok, err := hasher.VerifyPassword("correct-password", "not-a-valid-hash"); err != ErrInvalidPasswordHash || ok {
		t.Fatalf("VerifyPassword(malformed) = %v, %v; want false, ErrInvalidPasswordHash", ok, err)
	}
}

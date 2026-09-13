package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password with bcrypt (DefaultCost),
// suitable for storing in a users table. Compare with ComparePassword.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("auth: password must not be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
}

// ComparePassword reports whether password matches the bcrypt hash.
// A wrong password (or malformed hash) returns an error — never a bool
// that callers might ignore — so login handlers fail closed:
//
//	if err := auth.ComparePassword(u.passwordHash, in.Password); err != nil {
//	    return grove.Unauthorized("invalid credentials")
//	}
func ComparePassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("auth: invalid credentials")
	}
	return nil
}

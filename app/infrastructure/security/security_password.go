package security

import (
	"crypto/sha256"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type PasswordHasher struct{}

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{}
}

func (h *PasswordHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(prehash(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (h *PasswordHasher) Verify(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), prehash(password))
}

// prehash applies SHA-256 before bcrypt to safely handle passwords of any length.
// bcrypt truncates input at 72 bytes; prehashing ensures the full password is always considered.
func prehash(password string) []byte {
	h := sha256.Sum256([]byte(password))
	return h[:]
}

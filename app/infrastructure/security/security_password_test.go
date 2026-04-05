package security

import (
	"strings"
	"testing"
)

func TestHash_ReturnsValidBcryptHash(t *testing.T) {
	h := NewPasswordHasher()
	hash, err := h.Hash("secret123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(hash, "$2a$") {
		t.Fatalf("expected bcrypt hash prefix, got: %s", hash[:10])
	}
}

func TestVerify_CorrectPassword(t *testing.T) {
	h := NewPasswordHasher()
	hash, _ := h.Hash("correct-password")
	if err := h.Verify("correct-password", hash); err != nil {
		t.Fatalf("expected nil error for correct password, got: %v", err)
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	h := NewPasswordHasher()
	hash, _ := h.Hash("correct-password")
	if err := h.Verify("wrong-password", hash); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestVerify_EmptyPassword(t *testing.T) {
	h := NewPasswordHasher()
	hash, _ := h.Hash("something")
	if err := h.Verify("", hash); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerify_TamperedHash(t *testing.T) {
	h := NewPasswordHasher()
	hash, _ := h.Hash("password")
	tampered := hash[:len(hash)-1] + "X"
	if err := h.Verify("password", tampered); err == nil {
		t.Fatal("expected error for tampered hash")
	}
}

func TestVerify_InvalidHashFormat(t *testing.T) {
	h := NewPasswordHasher()
	if err := h.Verify("password", "not-a-bcrypt-hash"); err == nil {
		t.Fatal("expected error for invalid hash format")
	}
}

func TestHash_DifferentHashesForSameInput(t *testing.T) {
	h := NewPasswordHasher()
	h1, _ := h.Hash("same-password")
	h2, _ := h.Hash("same-password")
	if h1 == h2 {
		t.Fatal("expected different hashes due to salt")
	}
}

func TestHash_LongPassword(t *testing.T) {
	h := NewPasswordHasher()
	// bcrypt truncates at 72 bytes - both should verify against their own hash
	long := strings.Repeat("a", 100)
	hash, err := h.Hash(long)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := h.Verify(long, hash); err != nil {
		t.Fatalf("expected verify to succeed for long password: %v", err)
	}
}

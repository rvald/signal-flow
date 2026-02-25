package security_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/rvald/signal-flow/internal/security"
)

func TestEncryptionCycle(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	km, err := security.NewLocalKeyManager(key)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	plaintext := []byte("bluesky-refresh-token-abc123")
	ctx := context.Background()

	ciphertext, err := km.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := km.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip mismatch:\n  want: %q\n  got:  %q", plaintext, decrypted)
	}
}

func TestEncryptUniqueCiphertexts(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	km, err := security.NewLocalKeyManager(key)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	plaintext := []byte("same-token-every-time")
	ctx := context.Background()

	ct1, err := km.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt #1: %v", err)
	}

	ct2, err := km.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of the same plaintext produced identical ciphertext; nonce must be random")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	km, err := security.NewLocalKeyManager(key)
	if err != nil {
		t.Fatalf("NewLocalKeyManager: %v", err)
	}

	plaintext := []byte("sensitive-refresh-token")
	ctx := context.Background()

	ciphertext, err := km.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip the last byte of the ciphertext to simulate tampering.
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = km.Decrypt(ctx, tampered)
	if err == nil {
		t.Error("Decrypt should fail on tampered ciphertext, but returned nil error")
	}
}

func TestNewLocalKeyManagerInvalidKey(t *testing.T) {
	tests := []struct {
		name   string
		keyLen int
	}{
		{"empty key", 0},
		{"16-byte key", 16},
		{"24-byte key", 24},
		{"31-byte key", 31},
		{"33-byte key", 33},
		{"64-byte key", 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keyLen)
			_, err := security.NewLocalKeyManager(key)
			if err == nil {
				t.Errorf("NewLocalKeyManager accepted a %d-byte key; only 32-byte keys should be valid", tt.keyLen)
			}
		})
	}
}

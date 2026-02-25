// Package security provides encryption primitives and key management for Signal-Flow.
package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// KeyManager abstracts encryption/decryption of sensitive data.
// Implementations range from a local AES key (dev) to cloud KMS envelope encryption (prod).
type KeyManager interface {
	// Encrypt wraps plaintext using the master key logic. Returns nonce-prepended ciphertext.
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	// Decrypt unwraps nonce-prepended ciphertext.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// LocalKeyManager implements KeyManager using a 32-byte AES-256-GCM key loaded at startup.
type LocalKeyManager struct {
	gcm cipher.AEAD
}

// NewLocalKeyManager creates a LocalKeyManager from a raw 32-byte key.
// Returns an error if the key length is not exactly 32 bytes.
func NewLocalKeyManager(key []byte) (*LocalKeyManager, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("security: key must be exactly 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("security: aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("security: cipher.NewGCM: %w", err)
	}

	return &LocalKeyManager{gcm: gcm}, nil
}

// Encrypt seals plaintext with AES-256-GCM. The returned ciphertext is formatted as:
//
//	[12-byte nonce][GCM ciphertext + tag]
func (km *LocalKeyManager) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, km.gcm.NonceSize()) // 12 bytes for GCM
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("security: generate nonce: %w", err)
	}

	// Seal appends the ciphertext to the nonce slice, giving us nonce || ciphertext.
	return km.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens a nonce-prepended AES-256-GCM ciphertext.
func (km *LocalKeyManager) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	nonceSize := km.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("security: ciphertext too short (%d bytes)", len(ciphertext))
	}

	nonce := ciphertext[:nonceSize]
	encrypted := ciphertext[nonceSize:]

	plaintext, err := km.gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("security: decrypt failed: %w", err)
	}

	return plaintext, nil
}

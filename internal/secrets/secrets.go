package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// Encrypter wraps an AES-256-GCM AEAD for encrypting small secrets (API keys,
// connector tokens). The ciphertext format is: [12-byte nonce][ciphertext+tag].
type Encrypter struct {
	gcm cipher.AEAD
}

// New constructs an Encrypter from a 32-byte key.
func New(key []byte) (*Encrypter, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Encrypter{gcm: gcm}, nil
}

// NewFromEnv reads a base64-encoded 32-byte key from the given env var.
// Returns a helpful error if missing or malformed.
func NewFromEnv(varName string) (*Encrypter, error) {
	raw := os.Getenv(varName)
	if raw == "" {
		return nil, fmt.Errorf("%s is required (base64-encoded 32-byte key, e.g. `openssl rand -base64 32`)", varName)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded: %w", varName, err)
	}
	return New(key)
}

// Encrypt seals plaintext with a random nonce.
func (e *Encrypter) Encrypt(plaintext []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("encrypter not configured")
	}
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := e.gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, out...), nil
}

// Decrypt opens a ciphertext produced by Encrypt.
func (e *Encrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("encrypter not configured")
	}
	ns := e.gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, body := ciphertext[:ns], ciphertext[ns:]
	return e.gcm.Open(nil, nonce, body, nil)
}

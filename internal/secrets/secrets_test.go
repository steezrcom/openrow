package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return b
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	e, err := New(mustKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pt := []byte("the quick brown fox")
	ct, err := e.Encrypt(pt)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !bytes.Equal(ct[:magicLen], magic) {
		t.Fatalf("encrypt did not produce versioned format; first bytes=%x", ct[:magicLen])
	}
	if ct[magicLen] != 1 {
		t.Fatalf("write version=%d, want 1", ct[magicLen])
	}
	got, err := e.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("decrypt mismatch: got %q want %q", got, pt)
	}
}

func TestLegacyCiphertextDecrypts(t *testing.T) {
	key := mustKey(t)
	e, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Manually build a pre-versioning ciphertext: 12-byte nonce || seal.
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	pt := []byte("legacy row from before rotation")
	sealed := gcm.Seal(nil, nonce, pt, nil)
	legacy := append(append([]byte(nil), nonce...), sealed...)

	got, err := e.Decrypt(legacy)
	if err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("legacy decrypt mismatch: got %q want %q", got, pt)
	}
}

func TestMultiKeyRotation(t *testing.T) {
	k1 := mustKey(t)
	k2 := mustKey(t)

	// Old encrypter (V1 only).
	e1, err := NewMulti(map[byte][]byte{1: k1}, 1, k1)
	if err != nil {
		t.Fatalf("e1 NewMulti: %v", err)
	}
	ct1, err := e1.Encrypt([]byte("written under v1"))
	if err != nil {
		t.Fatalf("e1.Encrypt: %v", err)
	}
	if ct1[magicLen] != 1 {
		t.Fatalf("ct1 version=%d, want 1", ct1[magicLen])
	}

	// New encrypter: V1 + V2, write under V2.
	e2, err := NewMulti(map[byte][]byte{1: k1, 2: k2}, 2, k1)
	if err != nil {
		t.Fatalf("e2 NewMulti: %v", err)
	}
	// Reads old V1 ciphertext.
	if pt, err := e2.Decrypt(ct1); err != nil || string(pt) != "written under v1" {
		t.Fatalf("e2.Decrypt(ct1): pt=%q err=%v", pt, err)
	}
	// Writes under V2.
	ct2, err := e2.Encrypt([]byte("written under v2"))
	if err != nil {
		t.Fatalf("e2.Encrypt: %v", err)
	}
	if ct2[magicLen] != 2 {
		t.Fatalf("ct2 version=%d, want 2", ct2[magicLen])
	}
	if pt, err := e2.Decrypt(ct2); err != nil || string(pt) != "written under v2" {
		t.Fatalf("e2.Decrypt(ct2): pt=%q err=%v", pt, err)
	}
}

func TestDecryptUnknownVersion(t *testing.T) {
	e, _ := New(mustKey(t))
	bad := append([]byte(nil), magic...)
	bad = append(bad, 99)                                  // version not in keyring
	bad = append(bad, make([]byte, nonceLen+16)...)        // nonce + sealed tag (16 bytes is the GCM tag size)
	if _, err := e.Decrypt(bad); err == nil {
		t.Fatalf("expected error on unknown version")
	}
}

func TestEncryptIsRandom(t *testing.T) {
	e, _ := New(mustKey(t))
	pt := []byte("hello")
	a, _ := e.Encrypt(pt)
	b, _ := e.Encrypt(pt)
	if bytes.Equal(a, b) {
		t.Fatalf("nonces collided: ciphertext is deterministic")
	}
}

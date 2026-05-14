// Package secrets wraps AES-256-GCM for encrypting small per-tenant
// secrets (connector credentials, LLM API keys, flow webhook signing
// secrets). It supports multiple keys at once so operators can rotate
// the underlying key material without re-encrypting every row in
// lockstep with the binary swap.
//
// Ciphertext formats:
//
//	versioned (current writes):  "ORv" || version || nonce(12) || ct+tag
//	legacy   (older rows):       nonce(12) || ct+tag
//
// The 3-byte magic prefix lets the decrypter tell the two apart. A
// legacy ciphertext whose first three random nonce bytes collide with
// the magic (~1 / 16 M) would mis-route to the versioned path and fail
// to decrypt; the caller will see an error and can re-enter the secret.
// New deployments only ever produce versioned ciphertexts.
//
// Key configuration:
//
//	OPENROW_SECRET_KEY            base64(32 bytes) — single key, legacy
//	                              behaviour. Treated as version 1.
//	OPENROW_SECRET_KEY_V<n>       base64(32 bytes), n = 1..255. Multiple
//	                              can be set; the highest version is the
//	                              write key by default.
//	OPENROW_WRITE_KEY_VERSION     override the write key version (the
//	                              version used for new encryptions).
package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
)

// magic identifies a versioned ciphertext. Three bytes ("ORv") chosen to
// be unlikely to occur at the start of a random 12-byte nonce.
var magic = []byte{'O', 'R', 'v'}

const (
	magicLen = 3
	verLen   = 1
	nonceLen = 12 // GCM standard
)

// Encrypter holds the keyring and the version to write under.
type Encrypter struct {
	gcms       map[byte]cipher.AEAD // key version -> AEAD
	writeVer   byte                 // version used to seal new ciphertexts
	legacyGCM  cipher.AEAD          // used to open un-versioned rows
}

// New constructs an Encrypter from a single 32-byte key. The key is
// installed as both the legacy decryptor and version 1; writes use the
// versioned format with version=1.
func New(key []byte) (*Encrypter, error) {
	gcm, err := buildGCM(key)
	if err != nil {
		return nil, err
	}
	return &Encrypter{
		gcms:      map[byte]cipher.AEAD{1: gcm},
		writeVer:  1,
		legacyGCM: gcm,
	}, nil
}

// NewMulti builds an Encrypter from a versioned keyring. keys[v] must
// be 32 bytes for every v. writeVer must be present in keys. legacyKey
// is the key used to open unversioned (pre-versioning) ciphertexts; it
// can be the same as one of the versioned keys (typical: legacyKey ==
// keys[1] during transition).
func NewMulti(keys map[byte][]byte, writeVer byte, legacyKey []byte) (*Encrypter, error) {
	if len(keys) == 0 {
		return nil, errors.New("at least one key is required")
	}
	gcms := make(map[byte]cipher.AEAD, len(keys))
	for v, k := range keys {
		gcm, err := buildGCM(k)
		if err != nil {
			return nil, fmt.Errorf("key v%d: %w", v, err)
		}
		gcms[v] = gcm
	}
	if _, ok := gcms[writeVer]; !ok {
		return nil, fmt.Errorf("write version %d not in keyring", writeVer)
	}
	var legacyGCM cipher.AEAD
	if len(legacyKey) > 0 {
		g, err := buildGCM(legacyKey)
		if err != nil {
			return nil, fmt.Errorf("legacy key: %w", err)
		}
		legacyGCM = g
	}
	return &Encrypter{gcms: gcms, writeVer: writeVer, legacyGCM: legacyGCM}, nil
}

// NewFromEnv reads OPENROW_SECRET_KEY (single key) and/or any
// OPENROW_SECRET_KEY_V<n> entries from the environment and returns an
// Encrypter wired with all of them. The write key is the highest
// numbered V<n> by default; OPENROW_WRITE_KEY_VERSION overrides.
//
// `varName` is the legacy single-key env var; pass "OPENROW_SECRET_KEY"
// for backward compatibility. Older deployments that only set this one
// variable continue to work exactly as before, and any data written
// before this change remains decryptable via the legacy path.
func NewFromEnv(varName string) (*Encrypter, error) {
	versioned, err := readVersionedKeys()
	if err != nil {
		return nil, err
	}
	legacyRaw := os.Getenv(varName)

	// Path 1: no versioned keys → behave like the old `New` did, but
	// install the legacy key under version 1 as well so future writes
	// use the versioned format.
	if len(versioned) == 0 {
		if legacyRaw == "" {
			return nil, fmt.Errorf("%s is required (base64-encoded 32-byte key, e.g. `openssl rand -base64 32`)", varName)
		}
		k, err := decodeBase64(legacyRaw, varName)
		if err != nil {
			return nil, err
		}
		return New(k)
	}

	// Path 2: versioned keys present. Pick the write version (env
	// override or the highest available), and use the legacy env var
	// for opening pre-versioned ciphertext if it's set.
	writeVer, err := pickWriteVersion(versioned)
	if err != nil {
		return nil, err
	}
	var legacyKey []byte
	if legacyRaw != "" {
		legacyKey, err = decodeBase64(legacyRaw, varName)
		if err != nil {
			return nil, err
		}
	} else {
		// No legacy var: synthesise legacy from V1 if present, so old
		// rows encrypted with what used to be `OPENROW_SECRET_KEY`
		// (now reused as V1) remain decryptable.
		if k, ok := versioned[1]; ok {
			legacyKey = k
		}
	}
	return NewMulti(versioned, writeVer, legacyKey)
}

func buildGCM(key []byte) (cipher.AEAD, error) {
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
	return gcm, nil
}

func decodeBase64(raw, varName string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded: %w", varName, err)
	}
	return b, nil
}

var versionedKeyRe = regexp.MustCompile(`^OPENROW_SECRET_KEY_V(\d{1,3})$`)

func readVersionedKeys() (map[byte][]byte, error) {
	out := make(map[byte][]byte)
	for _, kv := range os.Environ() {
		eq := -1
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				eq = i
				break
			}
		}
		if eq < 0 {
			continue
		}
		name := kv[:eq]
		m := versionedKeyRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || n > 255 {
			return nil, fmt.Errorf("%s: version must be 1..255", name)
		}
		raw := kv[eq+1:]
		if raw == "" {
			continue
		}
		k, err := decodeBase64(raw, name)
		if err != nil {
			return nil, err
		}
		if len(k) != 32 {
			return nil, fmt.Errorf("%s: must be 32 bytes when base64-decoded", name)
		}
		out[byte(n)] = k
	}
	return out, nil
}

func pickWriteVersion(keys map[byte][]byte) (byte, error) {
	if v := os.Getenv("OPENROW_WRITE_KEY_VERSION"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 255 {
			return 0, fmt.Errorf("OPENROW_WRITE_KEY_VERSION: must be 1..255, got %q", v)
		}
		if _, ok := keys[byte(n)]; !ok {
			return 0, fmt.Errorf("OPENROW_WRITE_KEY_VERSION=%d but OPENROW_SECRET_KEY_V%d is not set", n, n)
		}
		return byte(n), nil
	}
	// Default: highest version available.
	versions := make([]int, 0, len(keys))
	for v := range keys {
		versions = append(versions, int(v))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	return byte(versions[0]), nil
}

// Encrypt seals plaintext with a random nonce, in the versioned format.
func (e *Encrypter) Encrypt(plaintext []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("encrypter not configured")
	}
	gcm, ok := e.gcms[e.writeVer]
	if !ok {
		return nil, fmt.Errorf("write key version %d missing", e.writeVer)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	header := append(append([]byte(nil), magic...), e.writeVer)
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	// Concatenate: magic || version || nonce || sealed
	out := make([]byte, 0, magicLen+verLen+len(nonce)+len(sealed))
	out = append(out, header...)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Decrypt opens a ciphertext produced by this package, in either the
// versioned or legacy format.
func (e *Encrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	if e == nil {
		return nil, errors.New("encrypter not configured")
	}
	// Versioned path: magic + version + nonce + ct.
	if len(ciphertext) >= magicLen+verLen+nonceLen && bytes.Equal(ciphertext[:magicLen], magic) {
		ver := ciphertext[magicLen]
		gcm, ok := e.gcms[ver]
		if !ok {
			return nil, fmt.Errorf("unknown key version %d", ver)
		}
		body := ciphertext[magicLen+verLen:]
		nonce, sealed := body[:nonceLen], body[nonceLen:]
		return gcm.Open(nil, nonce, sealed, nil)
	}
	// Legacy path: nonce + ct, decrypted with the legacy key. Used for
	// rows written before the versioning format landed.
	if e.legacyGCM == nil {
		return nil, errors.New("legacy ciphertext but no legacy key configured")
	}
	if len(ciphertext) < e.legacyGCM.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	ns := e.legacyGCM.NonceSize()
	nonce, body := ciphertext[:ns], ciphertext[ns:]
	return e.legacyGCM.Open(nil, nonce, body, nil)
}

// WriteVersion returns the version under which new ciphertexts are
// sealed. Exposed for diagnostics + operator UIs.
func (e *Encrypter) WriteVersion() byte { return e.writeVer }

// Package providers owns the runtime-mutable identity-provider registry
// (`instance_providers` table). Decision decision-auth-model-loopback-
// and-providers § 3 puts the active IdP list on env; this package
// extends that envelope so credential rotation and IdP toggling no
// longer require a restart.
//
// Encryption model: client_id is plaintext (it appears in OAuth
// metadata anyway), client_secret is AES-256-GCM ciphertext keyed by
// PINDOC_INSTANCE_KEY (32-byte base64-encoded env). Daemon refuses to
// start when an encrypted row exists but the key is unset — fail loud
// rather than silently lose decryption capability.
package providers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInstanceKeyMissing is returned when an encryption / decryption is
// attempted without a configured master key. Surfaced to handlers so
// the operator sees INSTANCE_KEY_MISSING rather than a generic crypto
// error.
var ErrInstanceKeyMissing = errors.New("providers: PINDOC_INSTANCE_KEY is required for credential storage")

// ErrInstanceKeyInvalid is returned when PINDOC_INSTANCE_KEY is set
// but cannot be decoded as 32-byte base64. Boot fails fast on this so
// operators don't discover the typo at first credential write.
var ErrInstanceKeyInvalid = errors.New("providers: PINDOC_INSTANCE_KEY must be 32 bytes (base64 encoded)")

// ErrCiphertextCorrupt covers nonce-too-short / GCM-auth-failed cases.
// Surfaced as INSTANCE_KEY_INVALID at the API since the most likely
// cause is "operator rotated the key without re-encrypting rows".
var ErrCiphertextCorrupt = errors.New("providers: encrypted credential is unreadable with the current PINDOC_INSTANCE_KEY")

// instanceKeyBytes is the AES-256 key length.
const instanceKeyBytes = 32

// gcmNonceBytes is the nonce length AES-GCM uses by default. Storing
// nonce inline so a future key rotation can re-encrypt without a
// schema bump.
const gcmNonceBytes = 12

// Cipher wraps a configured AES-GCM AEAD. Empty Cipher signals "no
// master key configured" — Encrypt / Decrypt return ErrInstanceKey
// Missing in that state.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipherFromBase64 builds a Cipher from a base64-encoded 32-byte
// key. Empty input returns an empty (no-op) Cipher; invalid input
// returns ErrInstanceKeyInvalid.
func NewCipherFromBase64(raw string) (*Cipher, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &Cipher{}, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInstanceKeyInvalid, err)
		}
	}
	if len(key) != instanceKeyBytes {
		return nil, fmt.Errorf("%w: got %d bytes, want %d", ErrInstanceKeyInvalid, len(key), instanceKeyBytes)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceKeyInvalid, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceKeyInvalid, err)
	}
	return &Cipher{aead: aead}, nil
}

// Configured reports whether a master key has been wired. Used by boot
// to decide between "key required" and "key optional" code paths.
func (c *Cipher) Configured() bool {
	return c != nil && c.aead != nil
}

// Encrypt produces nonce || ciphertext || tag for the given plaintext.
// Returns ErrInstanceKeyMissing when the cipher is unconfigured. Empty
// plaintext encrypts to empty bytes so storage rows with no secret
// (passkey / WebAuthn future) can serialise as zero-length.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	if !c.Configured() {
		return nil, ErrInstanceKeyMissing
	}
	nonce := make([]byte, gcmNonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("providers: nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Decrypt reverses Encrypt. Empty input maps to empty plaintext so
// rows with no secret round-trip cleanly. ErrInstanceKeyMissing
// surfaces when the cipher is unconfigured but a non-empty ciphertext
// is present — the daemon refuses to silently degrade in that case.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	if !c.Configured() {
		return nil, ErrInstanceKeyMissing
	}
	if len(ciphertext) < gcmNonceBytes+1 {
		return nil, ErrCiphertextCorrupt
	}
	nonce, body := ciphertext[:gcmNonceBytes], ciphertext[gcmNonceBytes:]
	plaintext, err := c.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCiphertextCorrupt, err)
	}
	return plaintext, nil
}

package providers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func TestCipher_RoundTrip(t *testing.T) {
	key := freshKey(t)
	c, err := NewCipherFromBase64(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewCipherFromBase64: %v", err)
	}
	if !c.Configured() {
		t.Fatal("Configured = false; want true")
	}
	plaintext := []byte("github-client-secret")
	ct, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ct) <= gcmNonceBytes {
		t.Fatalf("ciphertext too short: %d", len(ct))
	}
	pt, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(pt) != string(plaintext) {
		t.Fatalf("plaintext = %q, want %q", pt, plaintext)
	}
}

func TestCipher_EmptyKeyBecomesUnconfigured(t *testing.T) {
	c, err := NewCipherFromBase64("")
	if err != nil {
		t.Fatalf("empty key: %v", err)
	}
	if c.Configured() {
		t.Fatal("Configured = true on empty key")
	}
	if _, err := c.Encrypt([]byte("x")); !errors.Is(err, ErrInstanceKeyMissing) {
		t.Fatalf("Encrypt err = %v, want ErrInstanceKeyMissing", err)
	}
	// Empty ciphertext decrypts to empty without a key — rows with no
	// secret stay readable.
	if _, err := c.Decrypt(nil); err != nil {
		t.Fatalf("Decrypt(nil): %v", err)
	}
	if _, err := c.Decrypt([]byte{1, 2, 3}); !errors.Is(err, ErrInstanceKeyMissing) {
		t.Fatalf("Decrypt non-empty without key err = %v, want ErrInstanceKeyMissing", err)
	}
}

func TestCipher_RejectsWrongKeyLength(t *testing.T) {
	short := make([]byte, 16)
	_, err := NewCipherFromBase64(base64.StdEncoding.EncodeToString(short))
	if !errors.Is(err, ErrInstanceKeyInvalid) {
		t.Fatalf("err = %v, want ErrInstanceKeyInvalid for 16-byte key", err)
	}
}

func TestCipher_RejectsCorruptedCiphertext(t *testing.T) {
	c, _ := NewCipherFromBase64(base64.StdEncoding.EncodeToString(freshKey(t)))
	ct, _ := c.Encrypt([]byte("payload"))
	ct[len(ct)-1] ^= 0xff
	if _, err := c.Decrypt(ct); !errors.Is(err, ErrCiphertextCorrupt) {
		t.Fatalf("err = %v, want ErrCiphertextCorrupt", err)
	}
}

func TestCipher_AcceptsRawBase64(t *testing.T) {
	key := freshKey(t)
	raw := base64.RawStdEncoding.EncodeToString(key)
	if _, err := NewCipherFromBase64(raw); err != nil {
		t.Fatalf("raw base64 encoding rejected: %v", err)
	}
}

func freshKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, instanceKeyBytes)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return key
}

// Package security provides crypto primitives: AES-GCM secret encryption and bcrypt hashing.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/bcrypt"
)

// Crypto provides AES-GCM symmetric encryption for secrets at rest
// (cookie / apikey / passkey / downloader passwords).
type Crypto struct {
	key []byte // 32 bytes derived from the secret seed via SHA-256
}

// NewCrypto derives a 32-byte AES key from the given seed via SHA-256.
func NewCrypto(seed string) *Crypto {
	sum := sha256.Sum256([]byte(seed))
	return &Crypto{key: sum[:]}
}

// Encrypt encrypts plaintext with AES-GCM and returns a base64 string (nonce||ciphertext).
// Empty input yields empty output (no record stored).
func (c *Crypto) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	out := append(nonce, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt decrypts a base64 AES-GCM ciphertext produced by Encrypt.
func (c *Crypto) Decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns+1 {
		return "", errors.New("ciphertext too short")
	}
	pt, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// HashPassword bcrypt-hashes a password.
func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword reports whether the password matches the bcrypt hash.
func VerifyPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}

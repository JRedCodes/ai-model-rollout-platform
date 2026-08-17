// Package token generates and hashes random bearer tokens (tenant API
// keys, session tokens) with one shared convention: a 128-bit random
// value, hex-encoded for the plaintext form, SHA-256-hashed for storage.
// Plaintext is only ever handed to the caller once, at creation time.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Generate returns a random 128-bit token, hex-encoded.
func Generate() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// Hash returns the SHA-256 hash of a token, hex-encoded. Only the hash is
// ever persisted -- the plaintext token is never stored anywhere after
// it's generated and returned to the caller.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Package apikey generates and hashes API keys.
//
// A key is returned to the caller exactly once. Only its SHA-256 hash is
// stored, so a leaked database cannot be used to recover client keys.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// KeyBytes is the amount of randomness in a generated key.
const KeyBytes = 32

// Generate returns a new random key and the SHA-256 hash to store for it.
func Generate() (key, hash string, err error) {
	buf := make([]byte, KeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate API key: %w", err)
	}

	key = base64.RawURLEncoding.EncodeToString(buf)
	return key, Hash(key), nil
}

// Hash returns the lowercase hex SHA-256 hash of key, which is the value
// stored in api_keys.key_hash.
func Hash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

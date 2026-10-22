package apikey

import (
	"encoding/base64"
	"testing"
)

func TestGenerate(t *testing.T) {
	key, hash, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if key == "" {
		t.Fatal("Generate() returned an empty key")
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 hex characters", len(hash))
	}
	if got := Hash(key); got != hash {
		t.Errorf("Hash(key) = %q, want %q", got, hash)
	}

	raw, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("key is not base64url: %v", err)
	}
	if len(raw) != KeyBytes {
		t.Errorf("key entropy = %d bytes, want %d", len(raw), KeyBytes)
	}

	second, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if key == second {
		t.Error("two generated keys are identical")
	}
}

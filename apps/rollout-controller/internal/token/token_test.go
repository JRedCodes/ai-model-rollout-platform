package token

import "testing"

// TestGenerate covers the primitive session tokens (and tenant API keys)
// are built from: a random, hex-encoded, sufficiently long value that
// differs on every call.
func TestGenerate(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if a == b {
		t.Fatal("two calls to Generate produced the same token")
	}
	if len(a) != 32 { // 16 random bytes, hex-encoded
		t.Fatalf("expected a 32-character hex token, got length %d (%q)", len(a), a)
	}
}

func TestHash(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"typical token", "tk_deadbeefdeadbeefdeadbeefdeadbeef"},
		{"empty token", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h1 := Hash(c.token)
			h2 := Hash(c.token)

			if h1 != h2 {
				t.Fatal("Hash must be deterministic for the same input")
			}
			if h1 == c.token {
				t.Fatal("Hash must not return the plaintext unchanged")
			}
			if len(h1) != 64 { // SHA-256, hex-encoded
				t.Fatalf("expected a 64-character hex hash, got length %d (%q)", len(h1), h1)
			}
		})
	}
}

func TestHashDiffersForDifferentInput(t *testing.T) {
	if Hash("token-a") == Hash("token-b") {
		t.Fatal("different tokens hashed to the same value")
	}
}

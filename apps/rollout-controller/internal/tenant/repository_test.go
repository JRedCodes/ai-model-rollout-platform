package tenant

import "testing"

// generateAPIKey/hashAPIKey are the only pure, DB-free logic in this
// package -- everything else (Create, CreateTx, RegenerateAPIKey,
// ListAuth, GetIDByAPIKey) needs a live Postgres pool or transaction, and
// gets real coverage in the test/integration-coverage branch instead of a
// mocked unit test here (same call made for auth.UserRepository/
// SessionRepository in feat/auth). The underlying random-token and hash
// primitives are already covered by internal/token's tests; what's unique
// to this package is the "tk_" prefix convention layered on top.
func TestGenerateAPIKey(t *testing.T) {
	a, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey: %v", err)
	}
	b, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey: %v", err)
	}

	const prefix = "tk_"
	for _, key := range []string{a, b} {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			t.Fatalf("expected key to start with %q, got %q", prefix, key)
		}
	}
	if a == b {
		t.Fatal("two calls to generateAPIKey produced the same key")
	}
}

func TestHashAPIKeyIsDeterministicAndNotReversible(t *testing.T) {
	key := "tk_deadbeefdeadbeefdeadbeefdeadbeef"

	h1 := hashAPIKey(key)
	h2 := hashAPIKey(key)

	if h1 != h2 {
		t.Fatal("hashAPIKey must be deterministic for the same input")
	}
	if h1 == key {
		t.Fatal("hashAPIKey must not return the plaintext key unchanged")
	}
}

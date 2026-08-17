package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{"typical password", "correct-horse-battery-staple"},
		{"minimum length", "12345678"},
		{"unicode password", "pässwörd🔐123"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hash, err := HashPassword(c.password)
			if err != nil {
				t.Fatalf("HashPassword: %v", err)
			}
			if hash == c.password {
				t.Fatal("hash must not equal the plaintext password")
			}
			if !VerifyPassword(hash, c.password) {
				t.Fatal("VerifyPassword should accept the correct password")
			}
			if VerifyPassword(hash, c.password+"x") {
				t.Fatal("VerifyPassword should reject an incorrect password")
			}
		})
	}
}

func TestHashPasswordProducesDistinctHashes(t *testing.T) {
	// bcrypt salts each hash, so hashing the same password twice must
	// never produce the same output.
	a, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Fatal("hashing the same password twice produced identical hashes")
	}
}

func TestVerifyPasswordRejectsGarbageHash(t *testing.T) {
	if VerifyPassword("not-a-real-bcrypt-hash", "anything") {
		t.Fatal("VerifyPassword should reject a malformed hash rather than panic or falsely accept")
	}
}

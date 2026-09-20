package auth

import (
	"bytes"
	"testing"
)

func TestEncryptedToken(t *testing.T) {
	plain := "dbag_synthetic-test-token"
	sealed, err := SealToken("test-key", "token-1", "user-1", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(plain)) {
		t.Fatal("plaintext stored")
	}
	got, err := OpenToken("test-key", "token-1", "user-1", sealed)
	if err != nil || got != plain {
		t.Fatal("round trip failed")
	}
	for _, tc := range []struct{ key, token, user string }{
		{"wrong-key", "token-1", "user-1"}, {"test-key", "other-token", "user-1"}, {"test-key", "token-1", "other-user"},
	} {
		if _, err := OpenToken(tc.key, tc.token, tc.user, sealed); err == nil {
			t.Fatal("wrong identity or key accepted")
		}
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := OpenToken("test-key", "token-1", "user-1", sealed); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := OpenToken("test-key", "token-1", "user-1", nil); err == nil {
		t.Fatal("empty ciphertext accepted")
	}
}

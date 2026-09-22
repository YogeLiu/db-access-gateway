package auth

import (
	"bytes"
	"testing"
)

func TestEncryptedResourcePassword(t *testing.T) {
	plain := "target-password-with-specials-%&"
	sealed, err := SealResourcePassword("test-key", "resource-1", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(plain)) {
		t.Fatal("plaintext stored")
	}

	got, err := OpenResourcePassword("test-key", "resource-1", sealed)
	if err != nil || got != plain {
		t.Fatalf("round trip failed: got=%q err=%v", got, err)
	}
	for _, tc := range []struct {
		pepper, resourceID string
	}{
		{"wrong-key", "resource-1"},
		{"test-key", "resource-2"},
	} {
		if _, err := OpenResourcePassword(tc.pepper, tc.resourceID, sealed); err == nil {
			t.Fatal("wrong key or resource accepted")
		}
	}

	sealed[len(sealed)-1] ^= 1
	if _, err := OpenResourcePassword("test-key", "resource-1", sealed); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := OpenResourcePassword("test-key", "resource-1", nil); err == nil {
		t.Fatal("empty ciphertext accepted")
	}
}

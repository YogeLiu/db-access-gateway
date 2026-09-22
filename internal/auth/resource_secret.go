package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

func resourcePasswordCipher(pepper string) (cipher.AEAD, error) {
	key := Digest(pepper, "dbag/resource-password-encryption/v1")
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealResourcePassword encrypts a target database password with authenticated
// encryption. The resource ID is part of the associated data so ciphertext
// cannot be moved to a different resource and accepted.
func SealResourcePassword(pepper, resourceID, plain string) ([]byte, error) {
	aead, err := resourcePasswordCipher(pepper)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, []byte(plain), []byte("resource:"+resourceID)), nil
}

// OpenResourcePassword decrypts and authenticates a target database password.
func OpenResourcePassword(pepper, resourceID string, sealed []byte) (string, error) {
	aead, err := resourcePasswordCipher(pepper)
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("invalid encrypted resource password")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("resource:"+resourceID))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

// Domain separation keeps the encryption key distinct from token digests.
func tokenCipher(pepper string) (cipher.AEAD, error) {
	key := Digest(pepper, "dbag/token-encryption/v1")
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func SealToken(pepper, tokenID, principalID, plain string) ([]byte, error) {
	aead, err := tokenCipher(pepper)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, []byte(plain), []byte(tokenID+":"+principalID)), nil
}

func OpenToken(pepper, tokenID, principalID string, sealed []byte) (string, error) {
	aead, err := tokenCipher(pepper)
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("invalid encrypted token")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(tokenID+":"+principalID))
	return string(plain), err
}

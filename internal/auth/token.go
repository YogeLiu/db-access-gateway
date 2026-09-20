package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

const tokenPrefix = "dbag_"

func NewToken() (plain, prefix string, err error) {
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw[:])
	plain = tokenPrefix + secret
	prefix = plain
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	return plain, prefix, nil
}

func Digest(pepper, token string) [32]byte {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(token))
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func Bearer(header string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("missing bearer token")
	}
	return strings.TrimSpace(parts[1]), nil
}

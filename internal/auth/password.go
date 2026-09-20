package auth

import "golang.org/x/crypto/bcrypt"

const (
	minPasswordLength = 8
	maxPasswordBytes  = 72 // bcrypt's input limit; avoid silent truncation.
)

func ValidatePassword(password string) bool {
	return len([]rune(password)) >= minPasswordLength && len([]byte(password)) <= maxPasswordBytes
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

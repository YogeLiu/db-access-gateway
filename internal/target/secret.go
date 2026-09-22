package target

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
)

func resolveSecret(ref string) (string, error) {
	// Legacy compatibility for resources created before passwords were stored
	// encrypted in the control database.
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("credential reference is empty")
	}
	var b strings.Builder
	for _, ch := range ref {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			b.WriteRune(unicode.ToUpper(ch))
		} else {
			b.WriteByte('_')
		}
	}
	key := "DB_SECRET_" + b.String()
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return "", fmt.Errorf("credential reference %q is not available", ref)
	}
	return value, nil
}

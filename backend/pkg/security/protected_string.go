package security

import (
	"encoding/base64"
	"errors"
	"strings"
)

const protectedStringPrefix = "xenc:v1:"

// ProtectString stores provider-controlled URL-like values as authenticated
// ciphertext while keeping legacy SQLite column types intact.
func ProtectString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, protectedStringPrefix) {
		if _, err := RevealString(value); err != nil {
			return "", err
		}
		return value, nil
	}
	ciphertext, err := EncryptSecret([]byte(value))
	if err != nil {
		return "", err
	}
	return protectedStringPrefix + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// RevealString accepts plaintext only for migration/test compatibility.
// Production startup rewrites all populated provider values before serving.
func RevealString(value string) (string, error) {
	if !strings.HasPrefix(value, protectedStringPrefix) {
		return value, nil
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, protectedStringPrefix))
	if err != nil {
		return "", errors.New("protected provider value is invalid")
	}
	plaintext, err := DecryptSecret(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

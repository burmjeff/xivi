package queries

import (
	"errors"
	"xivi/backend/pkg/security"
)

const encryptedProviderURLPlaceholder = "encrypted"

func protectProviderURL(value string) (string, []byte, error) {
	if value == "" {
		return "", nil, errors.New("provider URL is empty")
	}
	ciphertext, err := security.EncryptSecret([]byte(value))
	if err != nil {
		return "", nil, err
	}
	return encryptedProviderURLPlaceholder, ciphertext, nil
}

func revealProviderURL(ciphertext []byte, fallback string) (string, error) {
	if len(ciphertext) == 0 {
		// This compatibility branch permits a safe, transactional startup
		// migration and keeps older test fixtures readable. Production startup
		// encrypts all populated rows before serving requests.
		return fallback, nil
	}
	plaintext, err := security.DecryptSecret(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

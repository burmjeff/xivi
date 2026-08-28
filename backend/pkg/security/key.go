package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"xivi/backend/platform/settings"
)

var (
	keyMu     sync.RWMutex
	masterKey []byte
)

func AuthKeyPath() string {
	if value := strings.TrimSpace(os.Getenv("XIVI_AUTH_KEY_FILE")); value != "" {
		return value
	}
	return filepath.Join(settings.CONFIG_PATH, "auth.key")
}

func GenerateKeyFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("authentication key path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("authentication key already exists at %s: %w", path, os.ErrExist)
		}
		return err
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		_ = file.Close()
		_ = os.Remove(path)
	}()
	if _, err := file.WriteString(base64.RawURLEncoding.EncodeToString(key) + "\n"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func InitializeKey() error {
	production := strings.EqualFold(strings.TrimSpace(os.Getenv("XIVI_PRODUCTION")), "true")
	path := AuthKeyPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if generateErr := GenerateKeyFile(path); generateErr != nil && !errors.Is(generateErr, os.ErrExist) {
			return fmt.Errorf("create authentication key at %s: %w", path, generateErr)
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return fmt.Errorf("authentication key unavailable at %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect authentication key at %s: %w", path, err)
	}
	if production && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("authentication key permissions at %s are too broad; use mode 0400 or 0600", path)
	}
	key, err := decodeKey(data)
	if err != nil {
		return fmt.Errorf("invalid authentication key: %w", err)
	}
	keyMu.Lock()
	masterKey = key
	keyMu.Unlock()
	return nil
}

func decodeKey(data []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(data))
	if decoded, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && len(decoded) >= 32 {
		return append([]byte(nil), decoded[:32]...), nil
	}
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) >= 32 {
		return append([]byte(nil), decoded[:32]...), nil
	}
	if len(data) >= 32 {
		return append([]byte(nil), data[:32]...), nil
	}
	return nil, errors.New("key must contain at least 256 random bits")
}

func derivedKey(label string) ([]byte, error) {
	keyMu.RLock()
	key := append([]byte(nil), masterKey...)
	keyMu.RUnlock()
	if len(key) != 32 {
		return nil, errors.New("authentication key is not initialized")
	}
	hash := hmac.New(sha256.New, key)
	_, _ = hash.Write([]byte("xivi:" + label))
	return hash.Sum(nil), nil
}

func HashToken(kind, token string) ([]byte, error) {
	key, err := derivedKey("token:" + kind)
	if err != nil {
		return nil, err
	}
	hash := hmac.New(sha256.New, key)
	_, _ = hash.Write([]byte(token))
	return hash.Sum(nil), nil
}

func CSRFToken(sessionToken string) (string, error) {
	hash, err := HashToken("csrf", sessionToken)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(hash), nil
}

func RandomToken(bytes int) (string, error) {
	if bytes < 16 {
		bytes = 16
	}
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func EncryptSecret(plaintext []byte) ([]byte, error) {
	key, err := derivedKey("secret-encryption:v1")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, []byte("xivi-secret-v1")), nil
}

func DecryptSecret(ciphertext []byte) ([]byte, error) {
	key, err := derivedKey("secret-encryption:v1")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("encrypted secret is truncated")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	return gcm.Open(nil, nonce, ciphertext[gcm.NonceSize():], []byte("xivi-secret-v1"))
}

func VirtualTunerToken(lineupID int64, version int64) (string, error) {
	payload := fmt.Sprintf("%d.%d", lineupID, version)
	signature, err := HashToken("virtual-tuner", payload)
	if err != nil {
		return "", err
	}
	return "xvt_" + payload + "." + base64.RawURLEncoding.EncodeToString(signature[:24]), nil
}

func ValidateVirtualTunerToken(token string) (int64, int64, bool) {
	var lineupID, version int64
	var encoded string
	if _, err := fmt.Sscanf(token, "xvt_%d.%d.%s", &lineupID, &version, &encoded); err != nil {
		return 0, 0, false
	}
	want, err := VirtualTunerToken(lineupID, version)
	return lineupID, version, err == nil && hmac.Equal([]byte(want), []byte(token))
}

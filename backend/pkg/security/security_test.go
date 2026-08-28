package security

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initializeTestKey(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_AUTH_KEY_FILE", path)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := GenerateKeyFile(path); err != nil {
		t.Fatal(err)
	}
	if err := InitializeKey(); err != nil {
		t.Fatal(err)
	}
}

func TestProductionRequiresExplicitAuthKeyFile(t *testing.T) {
	t.Setenv("XIVI_PRODUCTION", "true")
	t.Setenv("XIVI_AUTH_KEY_FILE", "")
	if err := InitializeKey(); err == nil || !strings.Contains(err.Error(), "XIVI_AUTH_KEY_FILE") {
		t.Fatalf("got %v, want explicit production key-file requirement", err)
	}
}

func TestProductionRejectsBroadAuthKeyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_PRODUCTION", "false")
	t.Setenv("XIVI_AUTH_KEY_FILE", path)
	if err := GenerateKeyFile(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XIVI_PRODUCTION", "true")
	if err := InitializeKey(); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("got %v, want restrictive key permission failure", err)
	}
}

func TestPasswordHashAndBounds(t *testing.T) {
	if err := ValidatePassword("orbit-42", "viewer"); err != nil {
		t.Fatalf("eight-character password rejected: %v", err)
	}
	if err := ValidatePassword("orbit-4", "viewer"); err == nil {
		t.Fatal("seven-character password accepted")
	}
	password := "a long unicode passphrase 🛰️"
	if err := ValidatePassword(password, "viewer"); err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(password, hash); err != nil || !ok {
		t.Fatalf("password did not verify: %v", err)
	}
	if ok, _ := VerifyPassword(password+"x", hash); ok {
		t.Fatal("wrong password verified")
	}
	if err := ValidatePassword("short", "viewer"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := ValidatePassword("viewer has a very long password", "viewer"); err == nil {
		t.Fatal("username-derived password accepted")
	}
	for _, common := range []string{"Password-Password-2026", "aaaaaaaaaaaaaaaa", "1234567890123456"} {
		if err := ValidatePassword(common, "viewer"); err == nil {
			t.Fatalf("common password %q accepted", common)
		}
	}
}

func TestPasswordVerificationHasBoundedMemoryConcurrency(t *testing.T) {
	for index := 0; index < cap(passwordVerifierSlots); index++ {
		passwordVerifierSlots <- struct{}{}
	}
	defer func() {
		for index := 0; index < cap(passwordVerifierSlots); index++ {
			<-passwordVerifierSlots
		}
	}()
	if _, err := VerifyPassword("irrelevant", DummyPasswordHash()); !errors.Is(err, ErrPasswordVerifierBusy) {
		t.Fatalf("got %v, want busy verifier", err)
	}
}

func TestTOTPReplayIsRejected(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0)
	counter := now.Unix() / 30
	code := totpCode(secret, counter)
	used, ok := ValidateTOTP(secret, code, now, -1)
	if !ok || used != counter {
		t.Fatal("fresh TOTP was rejected")
	}
	if _, ok := ValidateTOTP(secret, code, now, used); ok {
		t.Fatal("replayed TOTP was accepted")
	}
}

func TestSecretEncryptionAndURLRedaction(t *testing.T) {
	initializeTestKey(t)
	plaintext := []byte("https://user:password@example.com/private/list?token=secret")
	ciphertext, err := EncryptSecret(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	decoded, err := DecryptSecret(ciphertext)
	if err != nil || !bytes.Equal(decoded, plaintext) {
		t.Fatalf("secret did not round trip: %v", err)
	}
	redacted := RedactProviderURL(string(plaintext))
	for _, forbidden := range []string{"user", "password", "private", "token", "secret"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted URL contains %q: %s", forbidden, redacted)
		}
	}
}

func TestProtectedProviderStringRoundTrip(t *testing.T) {
	initializeTestKey(t)
	plaintext := "https://logos.example/private/image.png?token=secret"
	protected, err := ProtectString(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if protected == plaintext || strings.Contains(protected, "secret") || strings.Contains(protected, "logos.example") {
		t.Fatalf("provider string was not protected: %q", protected)
	}
	revealed, err := RevealString(protected)
	if err != nil || revealed != plaintext {
		t.Fatalf("provider string did not round trip: %q, %v", revealed, err)
	}
}

func TestSensitiveTextRedaction(t *testing.T) {
	value := RedactSensitiveText("access_token=xmk_prefix.secret password=hunter2 xvt_1.2.signature")
	for _, forbidden := range []string{"xmk_prefix.secret", "hunter2", "xvt_1.2.signature"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("redacted diagnostic contains %q: %s", forbidden, value)
		}
	}
}

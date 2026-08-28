package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPageCursorIsSignedAndBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.key")
	if err := os.Setenv("XIVI_AUTH_KEY_FILE", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("XIVI_AUTH_KEY_FILE") })
	if err := InitializeKey(); err != nil {
		t.Fatal(err)
	}

	value, err := EncodePageCursor(250, "user:7|/channels|filter")
	if err != nil {
		t.Fatal(err)
	}
	offset, err := ParsePageCursor(value, "user:7|/channels|filter")
	if err != nil || offset != 250 {
		t.Fatalf("cursor did not round trip: offset=%d err=%v", offset, err)
	}
	if _, err := ParsePageCursor(value, "user:8|/channels|filter"); err == nil {
		t.Fatal("cursor was accepted for another identity")
	}
	tamperedBytes := []byte(value)
	tamperedBytes[len(tamperedBytes)/2] = 'A'
	if tamperedBytes[len(tamperedBytes)/2] == value[len(value)/2] {
		tamperedBytes[len(tamperedBytes)/2] = 'B'
	}
	tampered := string(tamperedBytes)
	if _, err := ParsePageCursor(tampered, "user:7|/channels|filter"); err == nil {
		t.Fatal("tampered cursor was accepted")
	}
}

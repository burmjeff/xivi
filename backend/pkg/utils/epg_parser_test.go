package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"xivi/backend/app/models"
)

func TestFetchAndParseEPGRejectsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := fetchAndParseEPG(context.Background(), &models.Epg{URL: server.URL}, server.Client())
	if err == nil {
		t.Fatal("expected a non-success HTTP response to fail")
	}
	if !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("expected the HTTP status in the error, got %q", err)
	}
}

func TestParseEpgReturnsSourceError(t *testing.T) {
	missingSource := filepath.Join(t.TempDir(), "missing.xml")

	err := ParseEpg(&models.Epg{URL: missingSource})
	if err == nil {
		t.Fatal("expected an unreadable guide source to fail")
	}
	if !strings.Contains(err.Error(), "could not load guide data") {
		t.Fatalf("expected a useful guide error, got %q", err)
	}
}

package utils

import (
	"context"
	"fmt"
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

func TestParseXMLReportsIndeterminateProgrammeCount(t *testing.T) {
	var fixture strings.Builder
	fixture.WriteString("<tv>")
	for index := 0; index < 5000; index++ {
		fixture.WriteString(fmt.Sprintf(`<programme start="20260824120000 +0000" stop="20260824123000 +0000" channel="channel-1"><title>Programme %d</title></programme>`, index))
	}
	fixture.WriteString("</tv>")

	progress := -1
	message := ""
	item, err := parseXMLWithProgress(strings.NewReader(fixture.String()), func(next int, nextMessage string) {
		progress = next
		message = nextMessage
	})
	if err != nil {
		t.Fatalf("parse XMLTV fixture: %v", err)
	}
	if len(item.Programmes) != 5000 {
		t.Fatalf("expected 5000 programmes, got %d", len(item.Programmes))
	}
	if progress != 0 || !strings.Contains(message, "5000 programmes") {
		t.Fatalf("expected an indeterminate parse count, got progress=%d message=%q", progress, message)
	}
}

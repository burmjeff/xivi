package controllers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

func TestUpdateSettingsReturnsFieldValidationForMissingTrustedProxy(t *testing.T) {
	t.Setenv("XIVI_PRODUCTION", "true")
	next, err := settings.SetDefaults()
	if err != nil {
		t.Fatal(err)
	}
	next.Security.PublicBaseURL = "https://tv.example.com"
	next.Security.PublicMediaBaseURL = ""
	next.Security.LocalBaseURL = "http://192.168.1.10:4772"
	next.Security.TrustedProxyCIDRs = nil

	body, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Put("/api/settings", UpdateSettings)
	request := httptest.NewRequest("PUT", "/api/settings", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("invalid security settings returned %d, want 400", response.StatusCode)
	}
	var payload struct {
		Code        string            `json:"code"`
		Message     string            `json:"message"`
		FieldErrors map[string]string `json:"field_errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "invalid_security_settings" || payload.Message == "" || payload.FieldErrors["trusted_proxy_hosts"] == "" {
		t.Fatalf("unexpected security validation response: %#v", payload)
	}
}

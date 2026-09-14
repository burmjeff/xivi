package controllers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
)

// Real controller/middleware/SQLite boundary test. The browser fixture represents
// an already password/MFA-authenticated session; existing auth tests exercise login.
func TestTVPairingHTTPBoundaryAndViewerConfinement(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_AUTH_KEY_FILE", keyFile)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.GenerateKeyFile(keyFile); err != nil {
		t.Fatal(err)
	}
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}
	originalPolicy := settings.Current().Security
	settings.Current().Security.TrustedProxyCIDRs = []string{"0.0.0.0/0", "::/0"}
	settings.Current().Security.PublicBaseURL = "https://tv.example"
	t.Cleanup(func() { settings.Current().Security = originalPolicy })
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE app_user(id INTEGER PRIMARY KEY,username TEXT,password_hash TEXT,display_name TEXT,
		role TEXT,must_change_password BOOLEAN,initial_password BOOLEAN,auth_version INTEGER,disabled_at TIMESTAMP,created_at TIMESTAMP,updated_at TIMESTAMP)`)
	db.MustExec(`CREATE TABLE user_mfa(user_id INTEGER PRIMARY KEY)`)
	db.MustExec(`CREATE TABLE user_lineup(user_id INTEGER,lineup_id INTEGER)`)
	db.MustExec(`CREATE TABLE security_audit_event(id INTEGER PRIMARY KEY,actor_user_id INTEGER,actor_username TEXT,actor_display_name TEXT,
		target_user_id INTEGER,target_username TEXT,target_display_name TEXT,action TEXT,outcome TEXT,resource_type TEXT,resource_id TEXT,client_ip TEXT,detail TEXT,created_at TIMESTAMP)`)
	migration, err := os.ReadFile("../../platform/database/migrations/000031_add_tv_devices_and_preferences.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec(string(migration))
	db.MustExec(`INSERT INTO app_user VALUES(1,'admin','unused','Administrator','admin',FALSE,FALSE,1,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	originalDB := database.Db
	database.Db = &database.Queries{SecurityQueries: queries.NewSecurityQueries(db)}
	t.Cleanup(func() { database.Db = originalDB })

	app := fiber.New()
	app.Use(middleware.AuthenticateSession)
	app.Use(func(c *fiber.Ctx) error {
		if c.Get("Test-Browser") == "yes" {
			c.Locals("xivi_principal", models.SessionPrincipal{UserID: 1, Username: "admin", Role: models.RoleAdmin, MustChangePassword: c.Get("Test-Password-Change") == "yes"})
			c.Locals("xivi_session", &models.AuthSession{ID: 1, UserID: 1, TransportScope: "https"})
			c.Locals("xivi_session_token", "fixture-browser-session")
		}
		return c.Next()
	})
	app.Post("/api/v2/tv/auth/device", V2TVDevice)
	app.Post("/api/v2/tv/auth/token", V2TVToken)
	app.Post("/api/v2/tv/auth/decision", middleware.RequireAuthenticated(), middleware.RequirePasswordChanged(), middleware.CSRFProtected(), V2TVDecision)
	app.Get("/api/v2/watch/preferences", middleware.RequireAuthenticated(), V2WatchPreferences)
	app.Get("/api/v2/studio/users", middleware.RequireAdmin(), func(c *fiber.Ctx) error { return c.SendStatus(204) })
	app.Post("/api/v2/tv/auth/logout", middleware.RequireAuthenticated(), middleware.CSRFProtected(), V2TVLogout)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nonce := ""
	proof := func(key *ecdsa.PrivateKey, method, path, access string) string {
		b64 := base64.RawURLEncoding.EncodeToString
		header, _ := json.Marshal(map[string]any{"typ": "dpop+jwt", "alg": "ES256", "jwk": map[string]string{
			"crv": "P-256", "kty": "EC", "x": b64(key.X.FillBytes(make([]byte, 32))), "y": b64(key.Y.FillBytes(make([]byte, 32)))}})
		jti, err := security.RandomToken(24)
		if err != nil {
			t.Fatal(err)
		}
		claims := map[string]any{"jti": jti, "htm": method, "htu": "https://tv.example" + path, "iat": time.Now().Unix(), "nonce": nonce}
		if access != "" {
			digest := sha256.Sum256([]byte(access))
			claims["ath"] = b64(digest[:])
		}
		body, _ := json.Marshal(claims)
		value := b64(header) + "." + b64(body)
		digest := sha256.Sum256([]byte(value))
		r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return value + "." + b64(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
	}
	request := func(method, path string, body map[string]any, headers map[string]string, want int) map[string]any {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "http://tv.example"+path, bytes.NewReader(encoded))
		req.RequestURI = req.URL.RequestURI()
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("Origin", "https://tv.example")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		response, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload := map[string]any{}
		_ = json.NewDecoder(response.Body).Decode(&payload)
		if response.StatusCode != want {
			t.Fatalf("%s %s status=%d want=%d payload=%v", method, path, response.StatusCode, want, payload)
		}
		if next := response.Header.Get("DPoP-Nonce"); next != "" {
			nonce = next
		}
		return payload
	}
	devicePath, tokenPath := "/api/v2/tv/auth/device", "/api/v2/tv/auth/token"
	deviceBody := map[string]any{"device_name": "Living room"}
	request("POST", devicePath, deviceBody, map[string]string{"DPoP": proof(key, "POST", devicePath, "")}, 400)
	if nonce == "" {
		t.Fatal("Missing server nonce challenge")
	}
	pair := request("POST", devicePath, deviceBody, map[string]string{"DPoP": proof(key, "POST", devicePath, "")}, 200)
	if strings.Contains(pair["verification_uri_complete"].(string), pair["device_code"].(string)) {
		t.Fatal("QR exposed device secret")
	}
	decision := map[string]any{"user_code": pair["user_code"], "approve": true}
	request("POST", "/api/v2/tv/auth/decision", decision, nil, 401)
	request("POST", "/api/v2/tv/auth/decision", decision, map[string]string{"Test-Browser": "yes"}, 403)
	csrf, err := security.CSRFToken("fixture-browser-session")
	if err != nil {
		t.Fatal(err)
	}
	request("POST", "/api/v2/tv/auth/decision", decision, map[string]string{"Test-Browser": "yes", "X-CSRF-Token": csrf, "Origin": "https://attacker.example"}, 403)
	request("POST", "/api/v2/tv/auth/decision", decision, map[string]string{"Test-Browser": "yes", "X-CSRF-Token": csrf, "Test-Password-Change": "yes"}, 403)
	request("POST", "/api/v2/tv/auth/decision", decision, map[string]string{"Test-Browser": "yes", "X-CSRF-Token": csrf}, 204)
	exchange := map[string]any{"grant_type": "urn:ietf:params:oauth:grant-type:device_code", "device_code": pair["device_code"]}
	tokens := request("POST", tokenPath, exchange, map[string]string{"DPoP": proof(key, "POST", tokenPath, "")}, 200)
	access, refresh := tokens["access_token"].(string), tokens["refresh_token"].(string)
	db.MustExec(`UPDATE tv_pairing SET last_polled_at=?`, time.Now().Add(-6*time.Second))
	retry := request("POST", tokenPath, exchange, map[string]string{"DPoP": proof(key, "POST", tokenPath, "")}, 200)
	if retry["refresh_token"] != refresh || retry["device_id"] != tokens["device_id"] {
		t.Fatal("Pairing retry changed durable grant")
	}
	path := "/api/v2/watch/preferences"
	signed := proof(key, "GET", path, access)
	request("GET", path, nil, map[string]string{"Authorization": "DPoP " + access, "DPoP": signed}, 200)
	request("GET", path, nil, map[string]string{"Authorization": "DPoP " + access, "DPoP": signed}, 401)
	request("GET", path, nil, map[string]string{"Authorization": "DPoP " + access}, 401)
	request("GET", path, nil, map[string]string{"Authorization": "Bearer " + access}, 401)
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	request("GET", path, nil, map[string]string{"Authorization": "DPoP " + access, "DPoP": proof(other, "GET", path, access)}, 401)
	request("GET", path, nil, map[string]string{"Authorization": "DPoP " + access, "DPoP": proof(key, "GET", path, access), "X-Forwarded-Proto": "http"}, 403)
	request("GET", "/api/v2/studio/users", nil, map[string]string{"Authorization": "DPoP " + access, "DPoP": proof(key, "GET", "/api/v2/studio/users", access)}, 403)
	renew := map[string]any{"grant_type": "refresh_token", "refresh_token": refresh}
	newTokens := request("POST", tokenPath, renew, map[string]string{"DPoP": proof(key, "POST", tokenPath, "")}, 200)
	if newTokens["refresh_token"] != refresh {
		t.Fatal("Renewal rotated durable credential")
	}
	logoutPath := "/api/v2/tv/auth/logout"
	request("POST", logoutPath, map[string]any{}, map[string]string{"Authorization": "DPoP " + access, "DPoP": proof(key, "POST", logoutPath, access)}, 204)
	request("POST", tokenPath, renew, map[string]string{"DPoP": proof(key, "POST", tokenPath, "")}, 400)
}

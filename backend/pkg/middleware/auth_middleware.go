package middleware

import (
	"crypto/hmac"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

const (
	PublicSessionCookie = "__Host-xivi_session"
	LANSessionCookie    = "xivi_lan_session"
	passiveCheckHeader  = "X-Xivi-Session-Check"
	principalLocal      = "xivi_principal"
	sessionLocal        = "xivi_session"
	sessionTokenLocal   = "xivi_session_token"
	mediaKeyLocal       = "xivi_media_key"
)

type MediaCredential struct {
	Key             *models.MediaAccessKey
	VirtualLineupID int64
	Token           string
}

func Principal(c *fiber.Ctx) (models.SessionPrincipal, bool) {
	principal, ok := c.Locals(principalLocal).(models.SessionPrincipal)
	return principal, ok
}

func CurrentSession(c *fiber.Ctx) (*models.AuthSession, bool) {
	session, ok := c.Locals(sessionLocal).(*models.AuthSession)
	return session, ok
}

func CurrentSessionToken(c *fiber.Ctx) string {
	value, _ := c.Locals(sessionTokenLocal).(string)
	return value
}

func CurrentMediaCredential(c *fiber.Ctx) (MediaCredential, bool) {
	credential, ok := c.Locals(mediaKeyLocal).(MediaCredential)
	return credential, ok
}

func SessionCookieName(scope string) string {
	if scope == "https" {
		return PublicSessionCookie
	}
	return LANSessionCookie
}

func SetSessionCookie(c *fiber.Ctx, scope, token string, expires time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName(scope),
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		Secure:   scope == "https",
		SameSite: "Strict",
		Expires:  expires,
	})
}

func ClearSessionCookie(c *fiber.Ctx, scope string) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName(scope),
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   scope == "https",
		SameSite: "Strict",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
	})
}

func AuthenticateSession(c *fiber.Ctx) error {
	scope, allowed := security.TransportScope(c)
	if !allowed {
		return c.Next()
	}
	token := c.Cookies(SessionCookieName(scope))
	if token == "" {
		return c.Next()
	}
	hash, err := security.HashToken("session", token)
	if err != nil {
		return err
	}
	session, err := database.Db.GetSession(c.UserContext(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ClearSessionCookie(c, scope)
			return c.Next()
		}
		return err
	}
	now := time.Now().UTC()
	invalid := session.RevokedAt != nil || session.UserDisabledAt != nil || session.TransportScope != scope ||
		session.AuthVersion <= 0 || (session.MFAEnabled && !session.MFAVerified) ||
		!now.Before(session.IdleExpiresAt) || !now.Before(session.AbsoluteExpiresAt)
	if !invalid {
		user, userErr := database.Db.GetUserByID(c.UserContext(), session.UserID)
		invalid = userErr != nil || user.AuthVersion != session.AuthVersion
	}
	if invalid {
		_ = database.Db.RevokeSessionByHash(c.UserContext(), hash)
		ClearSessionCookie(c, scope)
		return c.Next()
	}
	lineupIDs := []int64{}
	if session.Role != models.RoleAdmin {
		lineupIDs, _ = database.Db.GetUserLineupIDs(c.UserContext(), session.UserID)
	}
	csrf, _ := security.CSRFToken(token)
	principal := models.SessionPrincipal{
		UserID: session.UserID, Username: session.Username, Role: session.Role,
		MustChangePassword: session.MustChangePassword, MFAEnabled: session.MFAEnabled,
		MFARequired: session.MFAEnabled && !session.MFAVerified, LineupIDs: lineupIDs, CSRFToken: csrf,
	}
	c.Locals(principalLocal, principal)
	c.Locals(sessionLocal, session)
	c.Locals(sessionTokenLocal, token)
	if !strings.EqualFold(c.Get(passiveCheckHeader), "passive") && now.Sub(session.LastSeenAt) >= time.Minute {
		idleExpiry := now.Add(sessionIdleDuration(session.Role, scope))
		if idleExpiry.After(session.AbsoluteExpiresAt) {
			idleExpiry = session.AbsoluteExpiresAt
		}
		_ = database.Db.TouchSession(c.UserContext(), session.ID, now, idleExpiry)
	}
	return c.Next()
}

func sessionIdleDuration(role, scope string) time.Duration {
	if scope == "lan_http" {
		if role == models.RoleAdmin {
			return 15 * time.Minute
		}
		return 2 * time.Hour
	}
	if role == models.RoleAdmin {
		return 30 * time.Minute
	}
	return 24 * time.Hour
}

func SessionDurations(role, scope string) (time.Duration, time.Duration) {
	idle := sessionIdleDuration(role, scope)
	if scope == "lan_http" {
		if role == models.RoleAdmin {
			return idle, 4 * time.Hour
		}
		return idle, 24 * time.Hour
	}
	if role == models.RoleAdmin {
		return idle, 12 * time.Hour
	}
	return idle, 30 * 24 * time.Hour
}

func RequireAuthenticated() fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, ok := Principal(c)
		if !ok {
			return securityError(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.")
		}
		if principal.MFARequired {
			return securityError(c, fiber.StatusUnauthorized, "mfa_required", "Sign in again with a verification code.")
		}
		return c.Next()
	}
}

func RequirePasswordChanged() fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, ok := Principal(c)
		if !ok {
			return securityError(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.")
		}
		if principal.MustChangePassword {
			return securityError(c, fiber.StatusForbidden, "password_change_required", "Change the temporary password before continuing.")
		}
		return c.Next()
	}
}

func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, ok := Principal(c)
		if !ok {
			return securityError(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.")
		}
		if !principal.IsAdmin() {
			return securityError(c, fiber.StatusForbidden, "administrator_required", "Administrator access is required.")
		}
		return c.Next()
	}
}

func RequireRecentReauthentication(maxAge time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		session, ok := CurrentSession(c)
		if !ok {
			return securityError(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.")
		}
		if time.Since(session.ReauthenticatedAt) > maxAge {
			return securityError(c, fiber.StatusForbidden, "reauthentication_required", "Confirm your password before continuing.")
		}
		return c.Next()
	}
}

func CSRFProtected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead || c.Method() == fiber.MethodOptions {
			return c.Next()
		}
		if _, ok := Principal(c); !ok {
			return securityError(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.")
		}
		token := CurrentSessionToken(c)
		want, err := security.CSRFToken(token)
		if err != nil || !hmac.Equal([]byte(want), []byte(c.Get("X-CSRF-Token"))) {
			return securityError(c, fiber.StatusForbidden, "csrf_rejected", "The request security token was invalid.")
		}
		if !ValidRequestOrigin(c) {
			return securityError(c, fiber.StatusForbidden, "origin_rejected", "The request origin was not allowed.")
		}
		return c.Next()
	}
}

func ValidRequestOrigin(c *fiber.Ctx) bool {
	if site := strings.ToLower(c.Get("Sec-Fetch-Site")); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	expected := security.BaseURLForRequest(c)
	want, err := url.Parse(expected)
	if err != nil || want.Host == "" {
		return false
	}
	value := c.Get("Origin")
	if value == "" {
		value = c.Get("Referer")
	}
	got, err := url.Parse(value)
	return err == nil && strings.EqualFold(got.Scheme, want.Scheme) && strings.EqualFold(got.Host, want.Host)
}

func authenticateMediaToken(c *fiber.Ctx) (MediaCredential, error) {
	if security.InitialSetupRequired() {
		return MediaCredential{}, sql.ErrNoRows
	}
	token := strings.TrimSpace(c.Query("access_token"))
	if token == "" || len(token) > 256 {
		return MediaCredential{}, sql.ErrNoRows
	}
	if lineupID, version, ok := security.ValidateVirtualTunerToken(token); ok {
		if !security.RequestNetworkInfo(c).DirectTrustedLAN {
			return MediaCredential{}, sql.ErrNoRows
		}
		valid, err := database.Db.VirtualTunerCredentialValid(c.UserContext(), lineupID, version)
		if err != nil || !valid {
			return MediaCredential{}, sql.ErrNoRows
		}
		return MediaCredential{VirtualLineupID: lineupID, Token: token}, nil
	}
	if !strings.HasPrefix(token, "xmk_") {
		return MediaCredential{}, sql.ErrNoRows
	}
	separator := strings.IndexByte(token, '.')
	if separator < 5 {
		return MediaCredential{}, sql.ErrNoRows
	}
	prefix := token[:separator]
	key, err := database.Db.GetMediaKeyByPrefix(c.UserContext(), prefix)
	if err != nil || key.RevokedAt != nil || (key.ExpiresAt != nil && time.Now().UTC().After(*key.ExpiresAt)) {
		return MediaCredential{}, sql.ErrNoRows
	}
	hash, err := security.HashToken("media", token)
	if err != nil || !hmac.Equal(hash, key.TokenHash) {
		return MediaCredential{}, sql.ErrNoRows
	}
	if !mediaKeyNetworkAllowed(key.NetworkScope, security.RequestNetworkInfo(c)) {
		return MediaCredential{}, sql.ErrNoRows
	}
	if key.LastUsedAt == nil || time.Since(*key.LastUsedAt) >= 5*time.Minute {
		_ = database.Db.TouchMediaKey(c.UserContext(), key.ID, security.RequestNetworkInfo(c).IP.String())
	}
	return MediaCredential{Key: key, Token: token}, nil
}

func mediaKeyNetworkAllowed(scope string, network security.RequestNetwork) bool {
	trustedLANTransport := network.DirectTrustedLAN &&
		(network.Scheme == "https" || settings.APP_SETTINGS.Security.AllowLANHTTP)
	switch scope {
	case "lan":
		return trustedLANTransport
	case "public":
		// Public keys are Internet-capable, not cleartext-capable. They may
		// travel over HTTPS or through the explicit trusted-LAN compatibility
		// exception, but never over an unapproved cleartext listener.
		return network.Scheme == "https" || trustedLANTransport
	default:
		return false
	}
}

// RequireImageAccess keeps logo delivery inside the same lineup boundary as
// guide and playback data. Administrators may use the unscoped logo library;
// viewers and compatibility clients must name an authorized lineup.
func RequireImageAccess() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if principal, ok := Principal(c); ok {
			if principal.MustChangePassword {
				return securityError(c, fiber.StatusForbidden, "password_change_required", "Change the temporary password before continuing.")
			}
			if principal.IsAdmin() {
				return c.Next()
			}
		}
		lineupID, valid := security.ParsePositiveID(c.Query("lineup_id"))
		if !valid {
			return securityError(c, fiber.StatusNotFound, "image_not_found", "The image was not found.")
		}
		if principal, ok := Principal(c); ok {
			allowed, err := database.Db.UserCanAccessLineup(c.UserContext(), principal.UserID, principal.Role, lineupID)
			assetAllowed := false
			if err == nil && allowed {
				assetAllowed, err = database.Db.LineupContainsLogo(c.UserContext(), lineupID, c.Params("asset"))
			}
			if err == nil && assetAllowed {
				return c.Next()
			}
			return securityError(c, fiber.StatusNotFound, "image_not_found", "The image was not found.")
		}
		credential, err := authenticateMediaToken(c)
		if err != nil {
			return rejectInvalidMediaCredential(c)
		}
		allowed := credential.VirtualLineupID == lineupID
		if credential.Key != nil {
			allowed, err = database.Db.MediaKeyAllowsLineup(c.UserContext(), credential.Key.ID, lineupID)
		}
		if err == nil && allowed {
			allowed, err = database.Db.LineupContainsLogo(c.UserContext(), lineupID, c.Params("asset"))
		}
		if err != nil || !allowed {
			return securityError(c, fiber.StatusNotFound, "image_not_found", "The image was not found.")
		}
		c.Locals(mediaKeyLocal, credential)
		return c.Next()
	}
}

func RequireLineupAccess(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		lineupID, ok := security.ParsePositiveID(c.Params(param))
		if !ok {
			return securityError(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.")
		}
		if principal, authenticated := Principal(c); authenticated {
			allowed, err := database.Db.UserCanAccessLineup(c.UserContext(), principal.UserID, principal.Role, lineupID)
			if err == nil && allowed {
				return c.Next()
			}
			return securityError(c, fiber.StatusNotFound, "lineup_not_found", "The lineup was not found.")
		}
		credential, err := authenticateMediaToken(c)
		if err != nil {
			return rejectInvalidMediaCredential(c)
		}
		allowed := credential.VirtualLineupID == lineupID
		if credential.Key != nil {
			allowed, err = database.Db.MediaKeyAllowsLineup(c.UserContext(), credential.Key.ID, lineupID)
		}
		if err != nil || !allowed {
			return securityError(c, fiber.StatusNotFound, "lineup_not_found", "The lineup was not found.")
		}
		c.Locals(mediaKeyLocal, credential)
		return c.Next()
	}
}

func RequireLineupMediaAccess(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		lineupID, ok := security.ParsePositiveID(c.Params(param))
		if !ok {
			return securityError(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.")
		}
		credential, err := authenticateMediaToken(c)
		if err != nil {
			return rejectInvalidMediaCredential(c)
		}
		allowed := credential.VirtualLineupID == lineupID
		if credential.Key != nil {
			allowed, err = database.Db.MediaKeyAllowsLineup(c.UserContext(), credential.Key.ID, lineupID)
		}
		if err != nil || !allowed {
			return securityError(c, fiber.StatusNotFound, "lineup_not_found", "The lineup was not found.")
		}
		c.Locals(mediaKeyLocal, credential)
		return c.Next()
	}
}

func RequirePlayback() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uuid := strings.TrimSpace(c.Params("stream_id"))
		if uuid == "" {
			return securityError(c, fiber.StatusBadRequest, "invalid_stream", "The stream id is invalid.")
		}
		if principal, authenticated := Principal(c); authenticated {
			if principal.MustChangePassword {
				return securityError(c, fiber.StatusForbidden, "password_change_required", "Change the temporary password before continuing.")
			}
			allowed, err := database.Db.UserCanAccessChannel(c.UserContext(), principal.UserID, principal.Role, uuid)
			if err == nil && allowed {
				return c.Next()
			}
			return securityError(c, fiber.StatusNotFound, "stream_not_found", "The stream was not found.")
		}
		credential, err := authenticateMediaToken(c)
		if err != nil {
			return rejectInvalidMediaCredential(c)
		}
		var allowed bool
		if credential.Key != nil {
			allowed, err = database.Db.MediaKeyAllowsChannel(c.UserContext(), credential.Key.ID, uuid)
		} else {
			_, version, _ := security.ValidateVirtualTunerToken(credential.Token)
			allowed, err = database.Db.VirtualTunerAllowsChannel(c.UserContext(), credential.VirtualLineupID, version, uuid)
		}
		if err != nil || !allowed {
			return securityError(c, fiber.StatusNotFound, "stream_not_found", "The stream was not found.")
		}
		c.Locals(mediaKeyLocal, credential)
		return c.Next()
	}
}

func RequireTrustedLAN() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !security.RequestNetworkInfo(c).DirectTrustedLAN {
			return securityError(c, fiber.StatusNotFound, "not_found", "The resource was not found.")
		}
		return c.Next()
	}
}

func securityError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(models.APIError{Code: code, Message: message, Retryable: false})
}

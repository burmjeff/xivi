package middleware

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

const tvDeviceLocal = "xivi_tv_device"

func CurrentTVDevice(c *fiber.Ctx) (*models.TVDevice, bool) {
	d, ok := c.Locals(tvDeviceLocal).(*models.TVDevice)
	return d, ok
}

// Deliberately narrow: an administrator's TV cannot administer the server.
func tvPathAllowed(method, path string) bool {
	if method == fiber.MethodGet || method == fiber.MethodHead {
		return strings.HasPrefix(path, "/api/v2/watch/") || strings.HasPrefix(path, "/stream/hls/") || strings.HasPrefix(path, "/images/")
	}
	return (method == fiber.MethodPatch && path == "/api/v2/watch/preferences") ||
		(method == fiber.MethodPost && (path == "/api/v2/watch/playback/release" || path == "/api/v2/stream/telemetry" || path == "/api/v2/tv/auth/logout"))
}

func authenticateTVDevice(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("WWW-Authenticate", `DPoP algs="ES256"`)
	if !tvPathAllowed(c.Method(), c.Path()) {
		return securityError(c, 403, "tv_viewer_only", "TV devices can only use viewer features.")
	}
	if security.RequestNetworkInfo(c).Scheme != "https" {
		return securityError(c, 403, "https_required", "TV devices require HTTPS.")
	}
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.Get("Authorization")), "DPoP "))
	if !strings.HasPrefix(token, "xta_") || len(token) > 256 {
		return securityError(c, 401, "invalid_token", "The TV access token is invalid.")
	}
	hash, err := security.HashToken("tv-access", token)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	d, err := database.Db.GetTVDeviceByAccess(c.UserContext(), hash, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return securityError(c, 401, "invalid_token", "Renew the TV access token.")
		}
		return err
	}
	proof, err := security.VerifyDPoP(c.Get("DPoP"), c.Method(), security.DPoPRequestURL(c), d.KeyThumbprint, token, now)
	if err != nil {
		return securityError(c, 401, "invalid_dpop_proof", "Check the TV clock and connection.")
	}
	used, err := database.Db.ConsumeTVProof(c.UserContext(), proof.Thumbprint, proof.JTI, now)
	if err != nil {
		return err
	}
	if !used {
		return securityError(c, 401, "invalid_dpop_proof", "The proof was already used.")
	}
	user, err := database.Db.GetUserByID(c.UserContext(), d.UserID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return securityError(c, 401, "pairing_revoked", "Pair this TV again.")
	}
	if d.RevokedAt != nil || user.DisabledAt != nil || user.MustChangePassword || user.AuthVersion != d.AuthVersion {
		return securityError(c, 401, "pairing_revoked", "This TV pairing has been revoked.")
	}
	c.Locals(tvDeviceLocal, d)
	c.Locals(principalLocal, models.SessionPrincipal{UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role, MFAEnabled: user.MFAEnabled, LineupIDs: user.LineupIDs})
	c.Response().Header.Del("WWW-Authenticate")
	return c.Next()
}

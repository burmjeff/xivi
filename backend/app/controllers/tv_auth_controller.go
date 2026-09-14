package controllers

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

func tvProof(c *fiber.Ctx, thumb string) (security.DPoPProof, bool, error) {
	c.Set("Cache-Control", "no-store")
	if security.RequestNetworkInfo(c).Scheme != "https" {
		return security.DPoPProof{}, false, v2Error(c, 403, "https_required", "TV pairing requires HTTPS.", false)
	}
	now := time.Now().UTC()
	proof, err := security.VerifyDPoP(c.Get("DPoP"), c.Method(), security.DPoPRequestURL(c), thumb, "", now)
	if err != nil {
		return proof, false, v2Error(c, 400, "invalid_dpop_proof", "Check the TV clock and connection.", true)
	}
	if !security.ValidDPoPNonce(proof.Nonce, proof.Thumbprint, now) {
		nonce, err := security.NewDPoPNonce(proof.Thumbprint, now)
		if err != nil {
			return proof, false, err
		}
		c.Set("DPoP-Nonce", nonce)
		return proof, false, v2Error(c, 400, "use_dpop_nonce", "Retry using the server nonce.", true)
	}
	used, err := database.Db.ConsumeTVProof(c.UserContext(), proof.Thumbprint, proof.JTI, now)
	if err != nil {
		return proof, false, err
	}
	if !used {
		return proof, false, v2Error(c, 400, "invalid_dpop_proof", "The proof was already used.", false)
	}
	return proof, true, nil
}

func V2TVDevice(c *fiber.Ctx) error {
	proof, ok, err := tvProof(c, "")
	if !ok {
		return err
	}
	var req struct {
		DeviceName string `json:"device_name"`
	}
	if decodeStrict(c, &req) != nil {
		return v2Error(c, 400, "invalid_device", "Enter a TV name.", false)
	}
	name, valid := normalizeDeviceName(req.DeviceName)
	if !valid {
		return v2Error(c, 400, "invalid_device", "Enter a TV name between 2 and 80 characters.", false)
	}
	random, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	deviceCode := "xtc_" + random
	// 40 bits, without visually ambiguous letters; per-IP ingress/poll limits.
	var raw [8]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return err
	}
	alphabet := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var code strings.Builder
	for _, b := range raw {
		code.WriteByte(alphabet[int(b)%len(alphabet)])
	}
	userCode := code.String()
	dh, err := security.HashToken("tv-pair-device", deviceCode)
	if err != nil {
		return err
	}
	uh, err := security.HashToken("tv-pair-user", userCode)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	p := &models.TVPairing{DeviceCodeHash: dh, UserCodeHash: uh, DeviceName: name, KeyThumbprint: proof.Thumbprint, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if err = database.Db.CreateTVPairing(c.UserContext(), p); err != nil {
		return err
	}
	u, _ := url.Parse(security.DPoPRequestURL(c))
	u.Path = "/tv/pair"
	u.RawQuery = ""
	approval := u.String()
	u.RawQuery = "code=" + userCode
	return c.JSON(fiber.Map{"device_code": deviceCode, "user_code": userCode[:4] + "-" + userCode[4:], "verification_uri": approval, "verification_uri_complete": u.String(), "expires_in": 600, "interval": 5})
}

func tvUserCode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

// GET is a preview for the signed-in browser. POST is the explicit decision;
// neither scanning a QR code nor visiting a link grants access on its own.
func V2TVDecision(c *fiber.Ctx) error {
	if _, ok := middleware.CurrentSession(c); !ok {
		return v2Error(c, 403, "browser_required", "Approve pairing in your signed-in browser.", false)
	}
	if err := requireMobileHTTPS(c); err != nil {
		return err
	}
	var req struct {
		UserCode string `json:"user_code"`
		Approve  bool   `json:"approve"`
	}
	if c.Method() == fiber.MethodGet {
		req.UserCode = c.Query("code")
	} else if decodeStrict(c, &req) != nil {
		return v2Error(c, 400, "invalid_code", "Enter the code on your TV.", false)
	}
	code := tvUserCode(req.UserCode)
	if len(code) != 8 {
		return v2Error(c, 400, "invalid_code", "Enter the eight-character code on your TV.", false)
	}
	hash, err := security.HashToken("tv-pair-user", code)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	p, err := database.Db.GetTVPairingByUserCode(c.UserContext(), hash, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v2Error(c, 404, "invalid_code", "This code expired or was already used.", false)
		}
		return err
	}
	if c.Method() == fiber.MethodGet {
		return c.JSON(fiber.Map{"device_name": p.DeviceName, "user_code": code[:4] + "-" + code[4:], "expires_at": p.ExpiresAt})
	}
	principal, _ := middleware.Principal(c)
	user, err := database.Db.GetUserByID(c.UserContext(), principal.UserID)
	if err != nil {
		return err
	}
	changed, err := database.Db.DecideTVPairing(c.UserContext(), hash, user.ID, user.AuthVersion, req.Approve, now)
	if err != nil {
		return err
	}
	if !changed {
		return v2Error(c, 409, "invalid_code", "This code is no longer available.", false)
	}
	auditSecurity(c, "tv_pairing_decision", "success", "tv_pairing", strconv.FormatInt(p.ID, 10), p.DeviceName, &user.ID)
	return c.SendStatus(204)
}

func V2TVToken(c *fiber.Ctx) error {
	proof, ok, err := tvProof(c, "")
	if !ok {
		return err
	}
	var req struct {
		GrantType    string `json:"grant_type"`
		DeviceCode   string `json:"device_code"`
		RefreshToken string `json:"refresh_token"`
	}
	if decodeStrict(c, &req) != nil {
		return v2Error(c, 400, "invalid_request", "Invalid token request.", false)
	}
	var d *models.TVDevice
	var refresh string
	now := time.Now().UTC()
	switch req.GrantType {
	case "urn:ietf:params:oauth:grant-type:device_code":
		if len(req.DeviceCode) > 256 || !strings.HasPrefix(req.DeviceCode, "xtc_") {
			return tvInvalidGrant(c)
		}
		hash, err := security.HashToken("tv-pair-device", req.DeviceCode)
		if err != nil {
			return err
		}
		r, err := security.RandomToken(48)
		if err != nil {
			return err
		}
		refresh = "xtr_" + r
		rh, err := security.HashToken("tv-refresh", refresh)
		if err != nil {
			return err
		}
		cipher, err := security.EncryptSecret([]byte(refresh))
		if err != nil {
			return err
		}
		d, cipher, err = database.Db.ExchangeTVPairing(c.UserContext(), hash, proof.Thumbprint, rh, cipher, security.RequestNetworkInfo(c).IP.String(), now)
		if errors.Is(err, queries.ErrTVPending) || errors.Is(err, queries.ErrTVSlowDown) {
			c.Set("Retry-After", "5")
			return v2Error(c, 400, err.Error(), "Waiting for approval on your phone.", true)
		}
		if errors.Is(err, queries.ErrTVGrant) {
			return tvInvalidGrant(c)
		}
		if err != nil {
			return err
		}
		plain, err := security.DecryptSecret(cipher)
		if err != nil {
			return err
		}
		refresh = string(plain)
	case "refresh_token":
		if len(req.RefreshToken) > 256 || !strings.HasPrefix(req.RefreshToken, "xtr_") {
			return tvInvalidGrant(c)
		}
		hash, err := security.HashToken("tv-refresh", req.RefreshToken)
		if err != nil {
			return err
		}
		d, err = database.Db.GetTVDeviceByRefresh(c.UserContext(), hash)
		if errors.Is(err, sql.ErrNoRows) {
			return tvInvalidGrant(c)
		}
		if err != nil {
			return err
		}
		if d.KeyThumbprint != proof.Thumbprint {
			return tvInvalidGrant(c)
		}
		refresh = req.RefreshToken
	default:
		return v2Error(c, 400, "unsupported_grant_type", "Unsupported TV grant.", false)
	}
	user, err := database.Db.GetUserByID(c.UserContext(), d.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return tvInvalidGrant(c)
	}
	if err != nil {
		return err
	}
	if d.RevokedAt != nil || user.DisabledAt != nil || user.MustChangePassword || d.AuthVersion != user.AuthVersion {
		return tvInvalidGrant(c)
	}
	r, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	access := "xta_" + r
	hash, err := security.HashToken("tv-access", access)
	if err != nil {
		return err
	}
	if err = database.Db.IssueTVAccess(c.UserContext(), d.ID, hash, now); errors.Is(err, queries.ErrTVGrant) {
		return tvInvalidGrant(c)
	} else if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"principal": mobilePrincipal(user), "device_id": d.ID, "access_token": access, "access_expires_at": now.Add(15 * time.Minute), "token_type": "DPoP", "expires_in": 900, "refresh_token": refresh})
}
func tvInvalidGrant(c *fiber.Ctx) error {
	return v2Error(c, 400, "invalid_grant", "This pairing expired or was revoked. Pair the TV again.", false)
}
func V2TVLogout(c *fiber.Ctx) error {
	d, ok := middleware.CurrentTVDevice(c)
	if !ok {
		return v2Error(c, 403, "tv_required", "A TV device session is required.", false)
	}
	if err := database.Db.RevokeTVDevice(c.UserContext(), d.ID, d.UserID); err != nil {
		return err
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("tv_device", d.ID, 0)
	auditSecurity(c, "tv_logout", "success", "tv_device", strconv.FormatInt(d.ID, 10), "", &d.UserID)
	return c.SendStatus(204)
}

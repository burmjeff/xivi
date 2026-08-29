package controllers

import (
	"crypto/hmac"
	"database/sql"
	"errors"
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

const (
	mobileAccessLifetime  = 15 * time.Minute
	mobileRefreshLifetime = 30 * 24 * time.Hour
)

func requireMobileHTTPS(c *fiber.Ctx) error {
	scope, ok := security.TransportScope(c)
	if !ok || scope != "https" {
		return v2Error(c, fiber.StatusForbidden, "https_required", "The mobile app requires a trusted HTTPS server.", false)
	}
	return nil
}

func normalizeDeviceName(value string) (string, bool) {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return value, len(value) >= 2 && len(value) <= 80
}

func mobilePrincipal(user *models.User) models.SessionPrincipal {
	lineups := user.LineupIDs
	if user.Role == models.RoleAdmin {
		lineups = []int64{}
	}
	return models.SessionPrincipal{
		UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role,
		MustChangePassword: user.MustChangePassword, MFAEnabled: user.MFAEnabled,
		MFARequired: false, LineupIDs: lineups,
	}
}

func createMobileSession(c *fiber.Ctx, user *models.User, deviceName string, mfaVerified bool) (models.MobileSessionEnvelope, error) {
	deviceName, valid := normalizeDeviceName(deviceName)
	if !valid {
		return models.MobileSessionEnvelope{}, errors.New("invalid device name")
	}
	accessRandom, err := security.RandomToken(32)
	if err != nil {
		return models.MobileSessionEnvelope{}, err
	}
	refreshRandom, err := security.RandomToken(48)
	if err != nil {
		return models.MobileSessionEnvelope{}, err
	}
	accessToken := "xma_" + accessRandom
	refreshToken := "xmr_" + refreshRandom
	accessHash, err := security.HashToken("mobile-access", accessToken)
	if err != nil {
		return models.MobileSessionEnvelope{}, err
	}
	refreshHash, err := security.HashToken("mobile-refresh", refreshToken)
	if err != nil {
		return models.MobileSessionEnvelope{}, err
	}
	now := time.Now().UTC()
	_, agentHash := requestUserAgent(c)
	session := &models.MobileSession{
		UserID: user.ID, AuthVersion: user.AuthVersion, DeviceName: deviceName,
		AccessTokenHash: accessHash, RefreshTokenHash: refreshHash, MFAVerified: mfaVerified,
		ReauthenticatedAt: now, CreatedAt: now, LastSeenAt: now,
		AccessExpiresAt: now.Add(mobileAccessLifetime), RefreshExpiresAt: now.Add(mobileRefreshLifetime),
		ClientIP: security.RequestNetworkInfo(c).IP.String(), UserAgentHash: agentHash,
	}
	if err := database.Db.CreateMobileSession(c.UserContext(), session); err != nil {
		return models.MobileSessionEnvelope{}, err
	}
	return models.MobileSessionEnvelope{
		Principal: mobilePrincipal(user), AccessToken: accessToken, AccessExpiresAt: session.AccessExpiresAt,
		RefreshToken: refreshToken, RefreshExpiresAt: session.RefreshExpiresAt,
	}, nil
}

func V2MobileLogin(c *fiber.Ctx) error {
	if err := requireMobileHTTPS(c); err != nil {
		return err
	}
	count, err := database.Db.UserCount(c.UserContext())
	if err != nil || count == 0 {
		return v2Error(c, fiber.StatusServiceUnavailable, "bootstrap_required", "Complete Xivi setup in a browser first.", false)
	}
	request := struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		DeviceName string `json:"device_name"`
		loginProtectionFields
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_login", "The login request was invalid.", false)
	}
	deviceName, deviceOK := normalizeDeviceName(request.DeviceName)
	if !deviceOK {
		return v2Error(c, fiber.StatusBadRequest, "invalid_device_name", "Enter a device name between 2 and 80 characters.", false)
	}
	username, normalizeErr := security.NormalizeUsername(request.Username)
	user, lookupErr := database.Db.GetUserByUsername(c.UserContext(), username)
	var userID *int64
	if lookupErr == nil {
		userID = &user.ID
	}
	identity, identityErr := loginProtection(c, username, userID)
	if identityErr != nil {
		return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "Sign-in protection could not be initialized.", true)
	}
	allowed, protectionErr := enforceLoginProtection(c, identity, request.loginProtectionFields)
	if !allowed {
		return protectionErr
	}
	hash := security.DummyPasswordHash()
	if lookupErr == nil {
		hash = user.PasswordHash
	}
	passwordOK, verifyErr := security.VerifyPassword(request.Password, hash)
	if errors.Is(verifyErr, security.ErrPasswordVerifierBusy) {
		c.Set(fiber.HeaderRetryAfter, "2")
		return v2Error(c, fiber.StatusTooManyRequests, "login_rate_limited", "Too many attempts. Try again shortly.", true)
	}
	if normalizeErr != nil || lookupErr != nil || user.DisabledAt != nil || !passwordOK {
		decision, recordErr := recordAuthenticationFailure(c, identity, "password", userID)
		if recordErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "The sign-in attempt could not be recorded.", true)
		}
		if decision.Blocked {
			return loginThrottleResponse(c, decision.RetryAfter)
		}
		delay := 125 * time.Millisecond * time.Duration(1<<min(max(decision.FailureCount-1, 0), 3))
		time.Sleep(delay)
		return v2Error(c, fiber.StatusUnauthorized, "invalid_credentials", "The username or password was invalid.", false)
	}
	if user.MFAEnabled {
		token, expires, challengeErr := createLoginChallenge(c, user, "https")
		if challengeErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "mfa_challenge_failed", "Verification could not be started.", true)
		}
		auditSecurity(c, "mobile_login_challenge", "success", "user", strconv.FormatInt(user.ID, 10), deviceName, &user.ID)
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"mfa_required": true, "challenge_token": token, "expires_at": expires,
		})
	}
	envelope, err := createMobileSession(c, user, deviceName, true)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "session_failed", "The mobile session could not be created.", true)
	}
	envelope.Principal = finishAuthentication(c, envelope.Principal, identity)
	auditSecurity(c, "mobile_login", "success", "user", strconv.FormatInt(user.ID, 10), deviceName, &user.ID)
	return c.JSON(envelope)
}

func V2MobileCompleteMFALogin(c *fiber.Ctx) error {
	if err := requireMobileHTTPS(c); err != nil {
		return err
	}
	request := struct {
		ChallengeToken string `json:"challenge_token"`
		Code           string `json:"code"`
		DeviceName     string `json:"device_name"`
		loginProtectionFields
	}{}
	if err := decodeStrict(c, &request); err != nil || len(request.ChallengeToken) > 256 ||
		!strings.HasPrefix(request.ChallengeToken, "xlc_") {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_challenge", "Verification expired. Sign in again.", false)
	}
	deviceName, deviceOK := normalizeDeviceName(request.DeviceName)
	if !deviceOK {
		return v2Error(c, fiber.StatusBadRequest, "invalid_device_name", "Enter a device name between 2 and 80 characters.", false)
	}
	hash, err := security.HashToken("login-challenge", request.ChallengeToken)
	if err != nil {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_challenge", "Verification expired. Sign in again.", false)
	}
	challenge, err := database.Db.GetLoginChallenge(c.UserContext(), hash)
	now := time.Now().UTC()
	_, agentHash := requestUserAgent(c)
	ip := security.RequestNetworkInfo(c).IP.String()
	validChallenge := err == nil && challenge.ConsumedAt == nil && challenge.FailureCount < loginChallengeFailures &&
		now.Before(challenge.ExpiresAt) && challenge.TransportScope == "https" && challenge.ClientIP == ip &&
		hmac.Equal(challenge.UserAgentHash, agentHash)
	if !validChallenge {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_challenge", "Verification expired. Sign in again.", false)
	}
	user, err := database.Db.GetUserByID(c.UserContext(), challenge.UserID)
	if err != nil || user.DisabledAt != nil || !user.MFAEnabled || user.AuthVersion != challenge.AuthVersion {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_challenge", "Verification expired. Sign in again.", false)
	}
	identity, identityErr := loginProtection(c, user.Username, &user.ID)
	if identityErr != nil {
		return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "Sign-in protection could not be initialized.", true)
	}
	allowed, protectionErr := enforceLoginProtection(c, identity, request.loginProtectionFields)
	if !allowed {
		return protectionErr
	}
	if !verifyUserMFA(c, user, request.Code) {
		_ = database.Db.RecordLoginChallengeFailure(c.UserContext(), challenge.ID)
		decision, recordErr := recordAuthenticationFailure(c, identity, "mfa", &user.ID)
		if recordErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "The verification attempt could not be recorded.", true)
		}
		if decision.Blocked {
			return loginThrottleResponse(c, decision.RetryAfter)
		}
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_code", "The verification or recovery code was invalid.", false)
	}
	consumed, err := database.Db.ConsumeLoginChallenge(c.UserContext(), challenge.ID, now, loginChallengeFailures)
	if err != nil || !consumed {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_challenge", "Verification expired. Sign in again.", false)
	}
	envelope, err := createMobileSession(c, user, deviceName, true)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "session_failed", "The mobile session could not be created.", true)
	}
	envelope.Principal = finishAuthentication(c, envelope.Principal, identity)
	auditSecurity(c, "mobile_login", "success", "user", strconv.FormatInt(user.ID, 10), deviceName, &user.ID)
	return c.JSON(envelope)
}

func V2MobileRefresh(c *fiber.Ctx) error {
	if err := requireMobileHTTPS(c); err != nil {
		return err
	}
	request := struct {
		RefreshToken string `json:"refresh_token"`
	}{}
	if err := decodeStrict(c, &request); err != nil || !strings.HasPrefix(request.RefreshToken, "xmr_") || len(request.RefreshToken) > 256 {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_refresh_token", "Sign in on your phone again.", false)
	}
	refreshHash, err := security.HashToken("mobile-refresh", request.RefreshToken)
	if err != nil {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_refresh_token", "Sign in on your phone again.", false)
	}
	session, err := database.Db.GetMobileSessionByRefreshHash(c.UserContext(), refreshHash)
	if errors.Is(err, sql.ErrNoRows) {
		session, err = database.Db.GetMobileSessionByRefreshHistory(c.UserContext(), refreshHash)
	}
	if err != nil {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_refresh_token", "Sign in on your phone again.", false)
	}
	now := time.Now().UTC()
	user, userErr := database.Db.GetUserByID(c.UserContext(), session.UserID)
	if session.RevokedAt != nil || session.UserDisabledAt != nil || userErr != nil || user.DisabledAt != nil ||
		user.AuthVersion != session.AuthVersion || (user.MFAEnabled && !session.MFAVerified) || !now.Before(session.RefreshExpiresAt) {
		_, _ = database.Db.RevokeMobileSession(c.UserContext(), session.ID, session.UserID)
		return v2Error(c, fiber.StatusUnauthorized, "invalid_refresh_token", "Sign in on your phone again.", false)
	}
	accessRandom, err := security.RandomToken(32)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "refresh_failed", "The session could not be refreshed.", true)
	}
	refreshRandom, err := security.RandomToken(48)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "refresh_failed", "The session could not be refreshed.", true)
	}
	accessToken := "xma_" + accessRandom
	newRefreshToken := "xmr_" + refreshRandom
	accessHash, err := security.HashToken("mobile-access", accessToken)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "refresh_failed", "The session could not be refreshed.", true)
	}
	newRefreshHash, err := security.HashToken("mobile-refresh", newRefreshToken)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "refresh_failed", "The session could not be refreshed.", true)
	}
	accessExpiry := now.Add(mobileAccessLifetime)
	err = database.Db.RotateMobileSession(c.UserContext(), session, refreshHash, accessHash, newRefreshHash, accessExpiry, now)
	if errors.Is(err, queries.ErrMobileRefreshReplay) {
		streaming.DefaultManager.DisconnectAuthorizedClients("mobile_session", session.ID, 0)
		auditSecurity(c, "mobile_refresh_replay", "denied", "mobile_session", strconv.FormatInt(session.ID, 10), session.DeviceName, &session.UserID)
		return v2Error(c, fiber.StatusUnauthorized, "refresh_token_reused", "This device session was revoked. Sign in again.", false)
	}
	if err != nil {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_refresh_token", "Sign in on your phone again.", false)
	}
	return c.JSON(models.MobileSessionEnvelope{
		Principal: mobilePrincipal(user), AccessToken: accessToken, AccessExpiresAt: accessExpiry,
		RefreshToken: newRefreshToken, RefreshExpiresAt: session.RefreshExpiresAt,
	})
}

func V2MobileLogout(c *fiber.Ctx) error {
	session, ok := middleware.CurrentMobileSession(c)
	if !ok {
		return v2Error(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.", false)
	}
	if _, err := database.Db.RevokeMobileSession(c.UserContext(), session.ID, session.UserID); err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "logout_failed", "The mobile session could not be ended.", true)
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("mobile_session", session.ID, 0)
	auditSecurity(c, "mobile_logout", "success", "mobile_session", strconv.FormatInt(session.ID, 10), session.DeviceName, &session.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2MobileChangePassword(c *fiber.Ctx) error {
	request := struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		MFACode         string `json:"mfa_code"`
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_request", "The password request was invalid.", false)
	}
	session, ok := middleware.CurrentMobileSession(c)
	if !ok {
		return v2Error(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.", false)
	}
	user, err := database.Db.GetUserByID(c.UserContext(), session.UserID)
	if err != nil {
		return v2Error(c, fiber.StatusUnauthorized, "authentication_required", "Sign in again.", false)
	}
	valid, verifyErr := security.VerifyPassword(request.CurrentPassword, user.PasswordHash)
	if errors.Is(verifyErr, security.ErrPasswordVerifierBusy) {
		c.Set(fiber.HeaderRetryAfter, "2")
		return v2Error(c, fiber.StatusTooManyRequests, "password_rate_limited", "Too many attempts. Try again shortly.", true)
	}
	if !valid || (user.MFAEnabled && !verifyUserMFA(c, user, request.MFACode)) {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_credentials", "The current password or verification code was invalid.", false)
	}
	if err := security.ValidatePassword(request.NewPassword, user.Username); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{Code: "weak_password", Message: "Choose a stronger password.", FieldErrors: map[string]string{"new_password": err.Error()}})
	}
	hash, err := security.HashPassword(request.NewPassword)
	if err != nil || database.Db.SetUserPassword(c.UserContext(), user.ID, hash, false) != nil {
		return v2Error(c, fiber.StatusInternalServerError, "password_change_failed", "The password could not be changed.", true)
	}
	if user.InitialPassword {
		security.SetInitialSetupRequired(false)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), user.ID)
	_ = database.Db.RevokeUserMobileSessions(c.UserContext(), user.ID)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), user.ID)
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), user.ID)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, user.ID)
	user, _ = database.Db.GetUserByID(c.UserContext(), user.ID)
	envelope, err := createMobileSession(c, user, session.DeviceName, true)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "session_failed", "Sign in again with the new password.", true)
	}
	auditSecurity(c, "password_change", "success", "user", strconv.FormatInt(user.ID, 10), "mobile", &user.ID)
	return c.JSON(envelope)
}

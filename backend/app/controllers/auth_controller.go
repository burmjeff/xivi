package controllers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
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
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

const (
	loginChallengeLifetime = 5 * time.Minute
	loginChallengeFailures = 5
	trustedBrowserLifetime = 30 * 24 * time.Hour
	maximumTrustedBrowsers = 20
	botChallengeLifetime   = 2 * time.Minute
	botChallengeDifficulty = 12
)

func decodeStrict(c *fiber.Ctx, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request contains trailing JSON")
		}
		return err
	}
	return nil
}

func auditSecurity(c *fiber.Ctx, action, outcome, resourceType, resourceID, detail string, targetUserID *int64, knownTargetUsername ...string) {
	var actor *int64
	actorUsername := ""
	actorDisplayName := ""
	if principal, ok := middleware.Principal(c); ok {
		id := principal.UserID
		actor = &id
		actorUsername = principal.Username
		actorDisplayName = principal.DisplayName
	}
	targetUsername := ""
	targetDisplayName := ""
	if len(knownTargetUsername) > 0 {
		targetUsername = knownTargetUsername[0]
		if len(knownTargetUsername) > 1 {
			targetDisplayName = knownTargetUsername[1]
		}
	} else if targetUserID != nil {
		if target, err := database.Db.GetUserByID(c.UserContext(), *targetUserID); err == nil {
			targetUsername = target.Username
			targetDisplayName = target.DisplayName
		}
	}
	ip := security.RequestNetworkInfo(c).IP.String()
	_ = database.Db.Audit(c.UserContext(), models.SecurityAuditEvent{
		ActorUserID: actor, ActorUsername: actorUsername, ActorDisplayName: actorDisplayName,
		TargetUserID: targetUserID, TargetUsername: targetUsername, TargetDisplayName: targetDisplayName,
		Action: action, Outcome: outcome, ResourceType: resourceType,
		ResourceID: resourceID, ClientIP: ip, Detail: detail,
	})
}

func V2BootstrapStatus(c *fiber.Ctx) error {
	// The default administrator can be used through any otherwise permitted
	// transport, but public callers do not need to know whether that temporary
	// credential is still active. Detailed first-run guidance is LAN-local.
	if !security.RequestNetworkInfo(c).DirectTrustedLAN {
		return c.JSON(fiber.Map{
			"bootstrap_required":               false,
			"initial_password_change_required": false,
			"initial_login_allowed":            false,
		})
	}
	count, err := database.Db.UserCount(c.UserContext())
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "bootstrap_status_failed", "Security status could not be loaded.", true)
	}
	initialSetup, err := database.Db.InitialAdminPasswordChangeRequired(c.UserContext(), "xivi")
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "bootstrap_status_failed", "Security status could not be loaded.", true)
	}
	security.SetInitialSetupRequired(initialSetup)
	_, transportAllowed := security.TransportScope(c)
	return c.JSON(fiber.Map{
		"bootstrap_required":               count == 0,
		"initial_password_change_required": initialSetup,
		"initial_login_allowed":            initialSetup && transportAllowed,
	})
}

type loginProtectionFields struct {
	BotChallengeToken string `json:"bot_challenge_token"`
	BotChallengeNonce string `json:"bot_challenge_nonce"`
}

type loginProtectionIdentity struct {
	Inputs      []models.AuthThrottleInput
	Hashes      [][]byte
	AccountHash []byte
	AddressHash []byte
}

func loginNetworkPrefix(ip net.IP) string {
	if ipv4 := ip.To4(); ipv4 != nil {
		return net.IP(ipv4.Mask(net.CIDRMask(24, 32))).String() + "/24"
	}
	if ipv6 := ip.To16(); ipv6 != nil {
		return net.IP(ipv6.Mask(net.CIDRMask(64, 128))).String() + "/64"
	}
	return "unknown"
}

func loginProtection(c *fiber.Ctx, username string, userID *int64) (loginProtectionIdentity, error) {
	ipValue := security.RequestNetworkInfo(c).IP
	ip := ipValue.String()
	network := loginNetworkPrefix(ipValue)
	values := []struct {
		typeName string
		value    string
		userID   *int64
	}{
		{"account", username, userID},
		{"address", ip, nil},
		{"network", network, nil},
		{"pair", username + "\x00" + ip, userID},
	}
	identity := loginProtectionIdentity{Inputs: make([]models.AuthThrottleInput, 0, len(values)), Hashes: make([][]byte, 0, len(values))}
	for _, value := range values {
		hash, err := security.HashToken("auth-throttle:"+value.typeName, value.value)
		if err != nil {
			return loginProtectionIdentity{}, err
		}
		identity.Inputs = append(identity.Inputs, models.AuthThrottleInput{BucketHash: hash,
			SubjectType: value.typeName, UserID: value.userID, ClientIP: ip, NetworkPrefix: network})
		identity.Hashes = append(identity.Hashes, hash)
		switch value.typeName {
		case "account":
			identity.AccountHash = hash
		case "address":
			identity.AddressHash = hash
		}
	}
	return identity, nil
}

func loginThrottleResponse(c *fiber.Ctx, retry time.Duration) error {
	seconds := max(1, int((retry+time.Second-1)/time.Second))
	c.Set(fiber.HeaderRetryAfter, strconv.Itoa(seconds))
	return v2Error(c, fiber.StatusTooManyRequests, "login_rate_limited",
		"Too many sign-in attempts. Try again after the cooldown.", true)
}

func leadingZeroBits(value [32]byte) int {
	bits := 0
	for _, item := range value {
		if item == 0 {
			bits += 8
			continue
		}
		for mask := byte(0x80); mask > 0 && item&mask == 0; mask >>= 1 {
			bits++
		}
		break
	}
	return bits
}

func verifyBotChallenge(c *fiber.Ctx, identity loginProtectionIdentity, fields loginProtectionFields) bool {
	if !strings.HasPrefix(fields.BotChallengeToken, "xbc_") || len(fields.BotChallengeToken) > 256 ||
		fields.BotChallengeNonce == "" || len(fields.BotChallengeNonce) > 32 {
		return false
	}
	tokenHash, err := security.HashToken("bot-challenge", fields.BotChallengeToken)
	if err != nil {
		return false
	}
	challenge, err := database.Db.GetBotChallenge(c.UserContext(), tokenHash)
	if err != nil || challenge.UsedAt != nil || time.Now().UTC().After(challenge.ExpiresAt) ||
		!hmac.Equal(challenge.AccountHash, identity.AccountHash) || !hmac.Equal(challenge.AddressHash, identity.AddressHash) {
		return false
	}
	proof := sha256.Sum256([]byte(fields.BotChallengeToken + ":" + fields.BotChallengeNonce))
	if leadingZeroBits(proof) < challenge.Difficulty {
		return false
	}
	consumed, err := database.Db.ConsumeBotChallenge(c.UserContext(), challenge.ID, time.Now().UTC())
	return err == nil && consumed
}

func issueBotChallenge(c *fiber.Ctx, identity loginProtectionIdentity) error {
	random, err := security.RandomToken(32)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "Sign-in protection could not be initialized.", true)
	}
	token := "xbc_" + random
	hash, err := security.HashToken("bot-challenge", token)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "Sign-in protection could not be initialized.", true)
	}
	now := time.Now().UTC()
	expires := now.Add(botChallengeLifetime)
	if err := database.Db.CreateBotChallenge(c.UserContext(), &models.BotChallenge{TokenHash: hash,
		AccountHash: identity.AccountHash, AddressHash: identity.AddressHash, Difficulty: botChallengeDifficulty,
		CreatedAt: now, ExpiresAt: expires}); err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "Sign-in protection could not be initialized.", true)
	}
	c.Set(fiber.HeaderRetryAfter, "0")
	return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Code: "bot_challenge_required",
		Message: "Please wait before trying again.", Retryable: true,
		Challenge: &models.BotChallengeResponse{Token: token, Difficulty: botChallengeDifficulty, ExpiresAt: expires}})
}

func enforceLoginProtection(c *fiber.Ctx, identity loginProtectionIdentity, fields loginProtectionFields) (bool, error) {
	now := time.Now().UTC()
	decision, err := database.Db.CheckAuthThrottle(c.UserContext(), identity.Hashes, now)
	if err != nil {
		return false, v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "Sign-in protection could not be checked.", true)
	}
	if decision.Blocked {
		_ = database.Db.RecordAuthThrottleDenial(c.UserContext(), identity.Hashes, now)
		return false, loginThrottleResponse(c, decision.RetryAfter)
	}
	if settings.Current().Security.AdaptiveLoginChallenge && decision.ChallengeRequired && !verifyBotChallenge(c, identity, fields) {
		return false, issueBotChallenge(c, identity)
	}
	return true, nil
}

func recordAuthenticationFailure(c *fiber.Ctx, identity loginProtectionIdentity, factor string, targetUserID *int64) (models.AuthThrottleDecision, error) {
	now := time.Now().UTC()
	decision, err := database.Db.RecordAuthFailure(c.UserContext(), identity.Inputs, factor, now)
	if err == nil && decision.NewlyBlocked {
		detail := factor + "_cooldown_started"
		auditSecurity(c, "login_throttled", "denied", "user", "", detail, targetUserID)
	}
	return decision, err
}

func finishAuthentication(c *fiber.Ctx, principal models.SessionPrincipal, identity loginProtectionIdentity) models.SessionPrincipal {
	notice, _ := database.Db.AccountMFANotice(c.UserContext(), identity.AccountHash)
	_ = database.Db.ResetAuthThrottle(c.UserContext(), [][]byte{identity.AccountHash, identity.Hashes[3]}, time.Now().UTC())
	principal.SecurityNotice = notice
	return principal
}

func createBrowserSession(c *fiber.Ctx, user *models.User, mfaVerified bool) (models.SessionPrincipal, error) {
	scope, ok := security.TransportScope(c)
	if !ok {
		return models.SessionPrincipal{}, errors.New("this network transport is not allowed")
	}
	random, err := security.RandomToken(32)
	if err != nil {
		return models.SessionPrincipal{}, err
	}
	token := "xss_" + random
	hash, err := security.HashToken("session", token)
	if err != nil {
		return models.SessionPrincipal{}, err
	}
	now := time.Now().UTC()
	idle, absolute := middleware.SessionDurations(user.Role, scope)
	agent := sha256.Sum256([]byte(c.Get(fiber.HeaderUserAgent)))
	session := &models.AuthSession{
		TokenHash: hash, UserID: user.ID, AuthVersion: user.AuthVersion, TransportScope: scope,
		MFAVerified: mfaVerified, ReauthenticatedAt: now, CreatedAt: now, LastSeenAt: now,
		IdleExpiresAt: now.Add(idle), AbsoluteExpiresAt: now.Add(absolute),
		ClientIP: security.RequestNetworkInfo(c).IP.String(), UserAgentHash: agent[:],
	}
	if err := database.Db.CreateSession(c.UserContext(), session); err != nil {
		return models.SessionPrincipal{}, err
	}
	middleware.SetSessionCookie(c, scope, token, session.AbsoluteExpiresAt)
	csrf, _ := security.CSRFToken(token)
	lineups := user.LineupIDs
	if user.Role == models.RoleAdmin {
		lineups = []int64{}
	}
	return models.SessionPrincipal{UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role,
		MustChangePassword: user.MustChangePassword, MFAEnabled: user.MFAEnabled,
		MFARequired: user.MFAEnabled && !mfaVerified, LineupIDs: lineups, CSRFToken: csrf}, nil
}

func verifyUserMFA(c *fiber.Ctx, user *models.User, code string) bool {
	if !user.MFAEnabled {
		return true
	}
	encrypted, lastCounter, err := database.Db.GetMFA(c.UserContext(), user.ID)
	if err != nil {
		return false
	}
	secret, err := security.DecryptSecret(encrypted)
	if err != nil {
		return false
	}
	if counter, valid := security.ValidateTOTP(string(secret), code, time.Now(), lastCounter); valid {
		used, _ := database.Db.UseMFACounter(c.UserContext(), user.ID, counter)
		return used
	}
	hash, err := security.HashToken("recovery", security.NormalizeRecoveryCode(code))
	if err != nil {
		return false
	}
	used, _ := database.Db.UseRecoveryCode(c.UserContext(), user.ID, hash)
	return used
}

func requestUserAgent(c *fiber.Ctx) (string, []byte) {
	value := strings.TrimSpace(c.Get(fiber.HeaderUserAgent))
	if len(value) > 255 {
		value = value[:255]
	}
	hash := sha256.Sum256([]byte(value))
	return value, hash[:]
}

func trustedBrowserValid(c *fiber.Ctx, user *models.User, scope string) bool {
	token := strings.TrimSpace(c.Cookies(middleware.TrustedBrowserCookieName(scope)))
	if token == "" || len(token) > 256 || !strings.HasPrefix(token, "xtb_") {
		return false
	}
	hash, err := security.HashToken("trusted-browser", token)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	browser, err := database.Db.GetValidTrustedBrowser(c.UserContext(), hash, user.ID, user.AuthVersion, scope, now)
	if err != nil {
		middleware.ClearTrustedBrowserCookie(c, scope)
		return false
	}
	_ = database.Db.TouchTrustedBrowser(c.UserContext(), browser.ID, now, security.RequestNetworkInfo(c).IP.String())
	return true
}

func createLoginChallenge(c *fiber.Ctx, user *models.User, scope string) (string, time.Time, error) {
	random, err := security.RandomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	token := "xlc_" + random
	hash, err := security.HashToken("login-challenge", token)
	if err != nil {
		return "", time.Time{}, err
	}
	_, agentHash := requestUserAgent(c)
	now := time.Now().UTC()
	expires := now.Add(loginChallengeLifetime)
	err = database.Db.CreateLoginChallenge(c.UserContext(), &models.LoginChallenge{
		TokenHash: hash, UserID: user.ID, AuthVersion: user.AuthVersion, TransportScope: scope,
		CreatedAt: now, ExpiresAt: expires, ClientIP: security.RequestNetworkInfo(c).IP.String(),
		UserAgentHash: agentHash,
	})
	return token, expires, err
}

func createTrustedBrowser(c *fiber.Ctx, user *models.User, scope string) error {
	random, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	token := "xtb_" + random
	hash, err := security.HashToken("trusted-browser", token)
	if err != nil {
		return err
	}
	agent, agentHash := requestUserAgent(c)
	now := time.Now().UTC()
	expires := now.Add(trustedBrowserLifetime)
	ip := security.RequestNetworkInfo(c).IP.String()
	if err := database.Db.CreateTrustedBrowser(c.UserContext(), &models.TrustedBrowser{
		TokenHash: hash, UserID: user.ID, AuthVersion: user.AuthVersion, TransportScope: scope,
		CreatedAt: now, LastUsedAt: now, ExpiresAt: expires, CreatedIP: ip, LastUsedIP: ip,
		UserAgent: agent, UserAgentHash: agentHash,
	}, maximumTrustedBrowsers); err != nil {
		return err
	}
	middleware.SetTrustedBrowserCookie(c, scope, token, expires)
	return nil
}

func V2Login(c *fiber.Ctx) error {
	scope, ok := security.TransportScope(c)
	if !ok || !middleware.ValidRequestOrigin(c) {
		return v2Error(c, fiber.StatusForbidden, "transport_rejected", "Use public HTTPS or an explicitly trusted LAN address.", false)
	}
	count, err := database.Db.UserCount(c.UserContext())
	if err != nil || count == 0 {
		return v2Error(c, fiber.StatusServiceUnavailable, "bootstrap_required", "The first-run administrator is not available. Restart Xivi and try again.", false)
	}
	request := struct {
		Username string `json:"username"`
		Password string `json:"password"`
		loginProtectionFields
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_login", "The login request was invalid.", false)
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
	valid := normalizeErr == nil && lookupErr == nil && user.DisabledAt == nil && passwordOK
	if !valid {
		decision, recordErr := recordAuthenticationFailure(c, identity, "password", userID)
		if recordErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "login_protection_failed", "The sign-in attempt could not be recorded.", true)
		}
		if decision.Blocked {
			return loginThrottleResponse(c, decision.RetryAfter)
		}
		// The delay grows before the persistent cooldown begins. Unknown and
		// known accounts follow the same hashing and verification path.
		delay := 125 * time.Millisecond * time.Duration(1<<min(max(decision.FailureCount-1, 0), 3))
		time.Sleep(delay)
		return v2Error(c, fiber.StatusUnauthorized, "invalid_credentials", "The username or password was invalid.", false)
	}
	if user.MFAEnabled && !trustedBrowserValid(c, user, scope) {
		token, expires, challengeErr := createLoginChallenge(c, user, scope)
		if challengeErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "mfa_challenge_failed", "Verification could not be started.", true)
		}
		target := user.ID
		auditSecurity(c, "login_challenge", "success", "user", strconv.FormatInt(user.ID, 10), "mfa_required", &target)
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"mfa_required": true, "challenge_token": token, "expires_at": expires,
		})
	}
	principal, err := createBrowserSession(c, user, true)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "session_failed", "The session could not be created.", true)
	}
	principal = finishAuthentication(c, principal, identity)
	target := user.ID
	auditSecurity(c, "login", "success", "user", strconv.FormatInt(user.ID, 10), "", &target)
	return c.JSON(principal)
}

func V2CompleteMFALogin(c *fiber.Ctx) error {
	scope, ok := security.TransportScope(c)
	if !ok || !middleware.ValidRequestOrigin(c) {
		return v2Error(c, fiber.StatusForbidden, "transport_rejected", "Use public HTTPS or an explicitly trusted LAN address.", false)
	}
	request := struct {
		ChallengeToken string `json:"challenge_token"`
		Code           string `json:"code"`
		TrustBrowser   bool   `json:"trust_browser"`
		loginProtectionFields
	}{}
	if err := decodeStrict(c, &request); err != nil || len(request.ChallengeToken) > 256 || !strings.HasPrefix(request.ChallengeToken, "xlc_") {
		return v2Error(c, fiber.StatusUnauthorized, "invalid_mfa_challenge", "Verification expired. Sign in again.", false)
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
		now.Before(challenge.ExpiresAt) && challenge.TransportScope == scope && challenge.ClientIP == ip &&
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
	principal, err := createBrowserSession(c, user, true)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "session_failed", "The session could not be created.", true)
	}
	principal = finishAuthentication(c, principal, identity)
	detail := ""
	if request.TrustBrowser {
		if err := createTrustedBrowser(c, user, scope); err != nil {
			detail = "trusted_browser_failed"
		} else {
			detail = "trusted_browser_created"
		}
	}
	target := user.ID
	auditSecurity(c, "login", "success", "user", strconv.FormatInt(user.ID, 10), detail, &target)
	return c.JSON(principal)
}

func V2Session(c *fiber.Ctx) error {
	principal, ok := middleware.Principal(c)
	if !ok {
		return v2Error(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.", false)
	}
	return c.JSON(principal)
}

func V2Logout(c *fiber.Ctx) error {
	session, ok := middleware.CurrentSession(c)
	if !ok {
		return v2Error(c, fiber.StatusUnauthorized, "authentication_required", "Sign in to continue.", false)
	}
	_, err := database.Db.RevokeUserSessionByID(c.UserContext(), session.ID, session.UserID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "logout_failed", "The session could not be ended. Try again.", true)
	}
	// A concurrent revocation is already the desired terminal state. Clear the
	// browser credential only after the database has confirmed that this session
	// is invalid, so the UI never presents a navigation as a successful logout.
	middleware.ClearSessionCookie(c, session.TransportScope)
	streaming.DefaultManager.DisconnectAuthorizedClients("session", session.ID, 0)
	auditSecurity(c, "logout", "success", "session", "", "", nil)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2Reauthenticate(c *fiber.Ctx) error {
	request := struct {
		Password string `json:"password"`
		// Kept temporarily for compatibility with clients built before
		// reauthentication became password-only. The value is intentionally
		// ignored; MFA remains enforced at sign-in.
		MFACode string `json:"mfa_code"`
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, 400, "invalid_request", "Password confirmation was invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	user, err := database.Db.GetUserByID(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, 401, "authentication_required", "Sign in again.", false)
	}
	valid, verifyErr := security.VerifyPassword(request.Password, user.PasswordHash)
	if errors.Is(verifyErr, security.ErrPasswordVerifierBusy) {
		c.Set(fiber.HeaderRetryAfter, "2")
		return v2Error(c, fiber.StatusTooManyRequests, "reauthentication_rate_limited", "Too many attempts. Try again shortly.", true)
	}
	if !valid {
		auditSecurity(c, "reauthenticate", "failure", "user", strconv.FormatInt(user.ID, 10), "invalid_credentials", &user.ID)
		return v2Error(c, 401, "invalid_credentials", "The password was invalid.", false)
	}
	session, _ := middleware.CurrentSession(c)
	_ = database.Db.MarkSessionReauthenticated(c.UserContext(), session.ID, time.Now().UTC(), session.MFAVerified)
	auditSecurity(c, "reauthenticate", "success", "user", strconv.FormatInt(user.ID, 10), "", &user.ID)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2ChangePassword(c *fiber.Ctx) error {
	request := struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		MFACode         string `json:"mfa_code"`
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, 400, "invalid_request", "The password request was invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	user, err := database.Db.GetUserByID(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, 401, "authentication_required", "Sign in again.", false)
	}
	valid, verifyErr := security.VerifyPassword(request.CurrentPassword, user.PasswordHash)
	if errors.Is(verifyErr, security.ErrPasswordVerifierBusy) {
		c.Set(fiber.HeaderRetryAfter, "2")
		return v2Error(c, fiber.StatusTooManyRequests, "password_rate_limited", "Too many attempts. Try again shortly.", true)
	}
	if !valid || (user.MFAEnabled && !verifyUserMFA(c, user, request.MFACode)) {
		return v2Error(c, 401, "invalid_credentials", "The current password or verification code was invalid.", false)
	}
	if err := security.ValidatePassword(request.NewPassword, user.Username); err != nil {
		return c.Status(400).JSON(models.APIError{Code: "weak_password", Message: "Choose a stronger password.", FieldErrors: map[string]string{"new_password": err.Error()}})
	}
	hash, err := security.HashPassword(request.NewPassword)
	if err != nil || database.Db.SetUserPassword(c.UserContext(), user.ID, hash, false) != nil {
		return v2Error(c, 500, "password_change_failed", "The password could not be changed.", true)
	}
	if user.InitialPassword {
		security.SetInitialSetupRequired(false)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), user.ID)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), user.ID)
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), user.ID)
	if session, ok := middleware.CurrentSession(c); ok {
		middleware.ClearTrustedBrowserCookie(c, session.TransportScope)
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, user.ID)
	user, _ = database.Db.GetUserByID(c.UserContext(), user.ID)
	newPrincipal, err := createBrowserSession(c, user, true)
	if err != nil {
		return v2Error(c, 500, "session_failed", "Sign in again with the new password.", true)
	}
	auditSecurity(c, "password_change", "success", "user", strconv.FormatInt(user.ID, 10), "", &user.ID)
	return c.JSON(newPrincipal)
}

func V2MFAEnroll(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	if principal.MFAEnabled {
		return v2Error(c, 409, "mfa_already_enabled", "Multifactor authentication is already enabled.", false)
	}
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		return v2Error(c, 500, "mfa_enrollment_failed", "MFA enrollment could not start.", true)
	}
	return c.JSON(fiber.Map{"secret": secret, "otpauth_uri": security.TOTPURI(secret, principal.Username)})
}

func V2MFAConfirm(c *fiber.Ctx) error {
	request := struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, 400, "invalid_request", "MFA confirmation was invalid.", false)
	}
	counter, valid := security.ValidateTOTP(strings.TrimSpace(request.Secret), request.Code, time.Now(), -1)
	if !valid {
		return v2Error(c, 400, "invalid_mfa_code", "The verification code was invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	encrypted, err := security.EncryptSecret([]byte(strings.TrimSpace(request.Secret)))
	if err != nil || database.Db.SaveMFA(c.UserContext(), principal.UserID, encrypted) != nil {
		return v2Error(c, 500, "mfa_save_failed", "MFA could not be enabled.", true)
	}
	_, _ = database.Db.UseMFACounter(c.UserContext(), principal.UserID, counter)
	codes, hashes, err := security.GenerateRecoveryCodes(10)
	if err != nil || database.Db.ReplaceRecoveryCodes(c.UserContext(), principal.UserID, hashes) != nil {
		_ = database.Db.DisableMFA(c.UserContext(), principal.UserID)
		return v2Error(c, 500, "recovery_codes_failed", "Recovery codes could not be created.", true)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), principal.UserID)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), principal.UserID)
	if session, ok := middleware.CurrentSession(c); ok {
		middleware.ClearTrustedBrowserCookie(c, session.TransportScope)
	}
	user, _ := database.Db.GetUserByID(c.UserContext(), principal.UserID)
	rotated, rotateErr := createBrowserSession(c, user, true)
	if rotateErr != nil {
		return v2Error(c, 500, "session_rotation_failed", "MFA was enabled; sign in again to continue.", true)
	}
	auditSecurity(c, "mfa_enable", "success", "user", strconv.FormatInt(principal.UserID, 10), "", &principal.UserID)
	return c.JSON(fiber.Map{"recovery_codes": codes, "session": rotated})
}

func V2MFADisable(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	if err := database.Db.DisableMFA(c.UserContext(), principal.UserID); err != nil {
		return v2Error(c, 500, "mfa_disable_failed", "MFA could not be disabled.", true)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), principal.UserID)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), principal.UserID)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, principal.UserID)
	if session, ok := middleware.CurrentSession(c); ok {
		middleware.ClearSessionCookie(c, session.TransportScope)
		middleware.ClearTrustedBrowserCookie(c, session.TransportScope)
	}
	auditSecurity(c, "mfa_disable", "success", "user", strconv.FormatInt(principal.UserID, 10), "", &principal.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2MFARegenerateRecoveryCodes(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	if !principal.MFAEnabled {
		return v2Error(c, 409, "mfa_not_enabled", "Enable MFA before creating recovery codes.", false)
	}
	codes, hashes, err := security.GenerateRecoveryCodes(10)
	if err != nil || database.Db.ReplaceRecoveryCodes(c.UserContext(), principal.UserID, hashes) != nil {
		return v2Error(c, 500, "recovery_codes_failed", "Recovery codes could not be regenerated.", true)
	}
	auditSecurity(c, "mfa_recovery_regenerate", "success", "user", strconv.FormatInt(principal.UserID, 10), "", &principal.UserID)
	return c.JSON(fiber.Map{"recovery_codes": codes})
}

func V2AccountSessions(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	items, err := database.Db.ListUserSessions(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, 500, "sessions_unavailable", "Sessions could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"items": items})
}

func V2RevokeAccountSession(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("session_id"))
	if !ok {
		return v2Error(c, 400, "invalid_session", "The session id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	revoked, err := database.Db.RevokeUserSessionByID(c.UserContext(), id, principal.UserID)
	if err != nil {
		return v2Error(c, 500, "session_revoke_failed", "The session could not be revoked.", true)
	}
	if !revoked {
		return v2Error(c, 404, "session_not_found", "The session was not found.", false)
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("session", id, 0)
	if current, currentOK := middleware.CurrentSession(c); currentOK && current.ID == id {
		middleware.ClearSessionCookie(c, current.TransportScope)
	}
	auditSecurity(c, "session_revoke", "success", "session", strconv.FormatInt(id, 10), "", &principal.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

func currentTrustedBrowserHash(c *fiber.Ctx, scope string) []byte {
	token := strings.TrimSpace(c.Cookies(middleware.TrustedBrowserCookieName(scope)))
	if token == "" || !strings.HasPrefix(token, "xtb_") || len(token) > 256 {
		return nil
	}
	hash, err := security.HashToken("trusted-browser", token)
	if err != nil {
		return nil
	}
	return hash
}

func V2AccountTrustedBrowsers(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	items, err := database.Db.ListTrustedBrowsers(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "trusted_browsers_unavailable", "Trusted browsers could not be loaded.", true)
	}
	scope, _ := security.TransportScope(c)
	currentHash := currentTrustedBrowserHash(c, scope)
	for index := range items {
		items[index].Current = len(currentHash) > 0 && hmac.Equal(items[index].TokenHash, currentHash)
	}
	return c.JSON(fiber.Map{"items": items})
}

func V2RevokeAccountTrustedBrowser(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("browser_id"))
	if !ok {
		return v2Error(c, fiber.StatusBadRequest, "invalid_trusted_browser", "The trusted browser id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	items, err := database.Db.ListTrustedBrowsers(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "trusted_browser_revoke_failed", "The trusted browser could not be revoked.", true)
	}
	scope, _ := security.TransportScope(c)
	currentHash := currentTrustedBrowserHash(c, scope)
	isCurrent := false
	for _, item := range items {
		if item.ID == id && len(currentHash) > 0 && hmac.Equal(item.TokenHash, currentHash) {
			isCurrent = true
			break
		}
	}
	revoked, err := database.Db.RevokeTrustedBrowser(c.UserContext(), id, principal.UserID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "trusted_browser_revoke_failed", "The trusted browser could not be revoked.", true)
	}
	if !revoked {
		return v2Error(c, fiber.StatusNotFound, "trusted_browser_not_found", "The trusted browser was not found.", false)
	}
	if isCurrent {
		middleware.ClearTrustedBrowserCookie(c, scope)
	}
	auditSecurity(c, "trusted_browser_revoke", "success", "trusted_browser", strconv.FormatInt(id, 10), "", &principal.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2RevokeAllAccountTrustedBrowsers(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	if err := database.Db.RevokeUserTrustedBrowsers(c.UserContext(), principal.UserID); err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "trusted_browser_revoke_failed", "Trusted browsers could not be revoked.", true)
	}
	scope, _ := security.TransportScope(c)
	middleware.ClearTrustedBrowserCookie(c, scope)
	auditSecurity(c, "trusted_browsers_revoke", "success", "user", strconv.FormatInt(principal.UserID, 10), "all", &principal.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

type identityWriteRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func normalizedIdentity(c *fiber.Ctx) (identityWriteRequest, error) {
	request := identityWriteRequest{}
	if err := decodeStrict(c, &request); err != nil {
		return request, err
	}
	username, err := security.NormalizeUsername(request.Username)
	if err != nil {
		return request, err
	}
	displayName, err := security.NormalizeDisplayName(request.DisplayName)
	if err != nil {
		return request, err
	}
	request.Username = username
	request.DisplayName = displayName
	return request, nil
}

func identityDetail(usernameChanged, displayNameChanged bool) string {
	changes := make([]string, 0, 2)
	if usernameChanged {
		changes = append(changes, "username_changed")
	}
	if displayNameChanged {
		changes = append(changes, "display_name_changed")
	}
	return strings.Join(changes, ",")
}

// V2UpdateAccountProfile allows an authenticated user to manage mutable
// identity without changing the immutable user id used by grants, media keys,
// and audit foreign keys.
func V2UpdateAccountProfile(c *fiber.Ctx) error {
	request, err := normalizedIdentity(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code: "invalid_profile", Message: "The account identity was invalid.",
			FieldErrors: map[string]string{"profile": err.Error()},
		})
	}
	principal, _ := middleware.Principal(c)
	user, err := database.Db.GetUserByID(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, fiber.StatusUnauthorized, "authentication_required", "Sign in again.", false)
	}
	usernameChanged := user.Username != request.Username
	displayNameChanged := user.DisplayName != request.DisplayName
	if !usernameChanged && !displayNameChanged {
		return c.JSON(principal)
	}
	if err := database.Db.UpdateUserIdentity(c.UserContext(), user.ID, request.Username, request.DisplayName); err != nil {
		if errors.Is(err, queries.ErrUsernameInUse) {
			return c.Status(fiber.StatusConflict).JSON(models.APIError{
				Code: "username_in_use", Message: "That username is already used.",
				FieldErrors: map[string]string{"username": "Choose a different username."},
			})
		}
		return v2Error(c, fiber.StatusInternalServerError, "profile_update_failed", "The account identity could not be updated.", true)
	}

	response := principal
	response.Username = request.Username
	response.DisplayName = request.DisplayName
	if usernameChanged {
		currentSession, sessionOK := middleware.CurrentSession(c)
		_ = database.Db.RevokeUserSessions(c.UserContext(), user.ID)
		_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), user.ID)
		streaming.DefaultManager.DisconnectUserSessionClients(user.ID)
		updated, loadErr := database.Db.GetUserByID(c.UserContext(), user.ID)
		if loadErr != nil || !sessionOK {
			return v2Error(c, fiber.StatusInternalServerError, "session_rotation_failed", "The username changed. Sign in again to continue.", true)
		}
		response, err = createBrowserSession(c, updated, currentSession.MFAVerified)
		if err != nil {
			middleware.ClearSessionCookie(c, currentSession.TransportScope)
			middleware.ClearTrustedBrowserCookie(c, currentSession.TransportScope)
			return v2Error(c, fiber.StatusInternalServerError, "session_rotation_failed", "The username changed. Sign in again to continue.", true)
		}
		middleware.ClearTrustedBrowserCookie(c, currentSession.TransportScope)
	}
	targetID := user.ID
	auditSecurity(c, "profile_update", "success", "user", strconv.FormatInt(user.ID, 10),
		identityDetail(usernameChanged, displayNameChanged), &targetID)
	return c.JSON(response)
}

func V2StudioUsers(c *fiber.Ctx) error {
	users, err := database.Db.ListUsers(c.UserContext())
	if err != nil {
		return v2Error(c, 500, "users_unavailable", "Users could not be loaded.", true)
	}
	lineups, _ := database.Db.GetLineupGrants(c.UserContext())
	return c.JSON(fiber.Map{"items": users, "lineups": lineups})
}

func V2StudioSecurityAudit(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit < 1 || limit > 200 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_limit", "Use a limit from 1 to 200.", false)
	}
	var beforeID int64
	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		decoded, err := security.ParsePageCursor(cursor, paginationBinding(c))
		if err != nil || decoded < 1 {
			return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The audit cursor is invalid.", false)
		}
		beforeID = int64(decoded)
	}
	items, total, err := database.Db.ListSecurityAuditEvents(c.UserContext(), beforeID, limit)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "security_audit_unavailable", "The security audit could not be loaded.", true)
	}
	var nextCursor *string
	if len(items) == limit && len(items) > 0 {
		value, cursorErr := security.EncodePageCursor(int(items[len(items)-1].ID), paginationBinding(c))
		if cursorErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next audit page could not be created.", true)
		}
		nextCursor = &value
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nextCursor, "total": total})
}

func V2StudioAuthProtection(c *fiber.Ctx) error {
	now := time.Now().UTC()
	items, err := database.Db.ListAuthThrottleBuckets(c.UserContext(), now, now.AddDate(0, 0, -30))
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "auth_protection_unavailable", "Sign-in protection activity could not be loaded.", true)
	}
	active, passwordFailures, mfaFailures, denied := 0, 0, 0, 0
	responseItems := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		if item.BlockedUntil != nil && item.BlockedUntil.After(now) {
			active++
		}
		// Account buckets are the canonical totals; the other independent
		// buckets intentionally observe the same attempt from another angle.
		if item.SubjectType == "account" {
			passwordFailures += item.PasswordFailures
			mfaFailures += item.MFAFailures
		}
		denied += item.DeniedRequests
		subject := "Unknown account"
		switch item.SubjectType {
		case "account":
			if item.Username != "" {
				subject = item.Username
				if item.DisplayName != "" {
					subject = item.DisplayName + " (@" + item.Username + ")"
				}
			}
		case "address":
			subject = item.ClientIP
		case "network":
			subject = item.NetworkPrefix
		case "pair":
			if item.Username != "" {
				subject = item.Username + " · " + item.ClientIP
			} else {
				subject = "Unknown account · " + item.ClientIP
			}
		}
		responseItems = append(responseItems, fiber.Map{
			"id": item.ID, "subject_type": item.SubjectType, "subject": subject,
			"user_id": item.UserID, "client_ip": item.ClientIP, "network_prefix": item.NetworkPrefix,
			"consecutive_failures": item.ConsecutiveFailures, "password_failures": item.PasswordFailures,
			"mfa_failures": item.MFAFailures, "denied_requests": item.DeniedRequests,
			"last_factor": item.LastFactor, "last_failed_at": item.LastFailedAt,
			"blocked_until": item.BlockedUntil,
		})
	}
	return c.JSON(fiber.Map{"summary": fiber.Map{"active_throttles": active,
		"password_failures": passwordFailures, "mfa_failures": mfaFailures, "denied_requests": denied},
		"items": responseItems, "retention_days": 30})
}

func V2ClearStudioAuthProtection(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("bucket_id"), 10, 64)
	if err != nil || id < 1 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_throttle", "The sign-in throttle was invalid.", false)
	}
	cleared, err := database.Db.ClearAuthThrottleBucket(c.UserContext(), id)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "throttle_clear_failed", "The sign-in throttle could not be cleared.", true)
	}
	if !cleared {
		return v2Error(c, fiber.StatusNotFound, "throttle_not_found", "The sign-in throttle was not found.", false)
	}
	auditSecurity(c, "login_throttle_clear", "success", "auth_throttle", strconv.FormatInt(id, 10), "", nil)
	return c.SendStatus(fiber.StatusNoContent)
}

type userWriteRequest struct {
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Password    string  `json:"password"`
	Role        string  `json:"role"`
	Disabled    bool    `json:"disabled"`
	LineupIDs   []int64 `json:"lineup_ids"`
}

func validateLineupGrants(c *fiber.Ctx, role string, values []int64) ([]int64, bool) {
	if role == models.RoleAdmin {
		return nil, true
	}
	principal, _ := middleware.Principal(c)
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, lineupID := range values {
		if lineupID < 1 {
			return nil, false
		}
		if _, duplicate := seen[lineupID]; duplicate {
			continue
		}
		allowed, err := database.Db.UserCanAccessLineup(c.UserContext(), principal.UserID, principal.Role, lineupID)
		if err != nil || !allowed {
			return nil, false
		}
		seen[lineupID] = struct{}{}
		result = append(result, lineupID)
	}
	return result, true
}

func V2CreateStudioUser(c *fiber.Ctx) error {
	request := userWriteRequest{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, 400, "invalid_user", "The user details were invalid.", false)
	}
	username, err := security.NormalizeUsername(request.Username)
	if err != nil {
		return c.Status(400).JSON(models.APIError{Code: "invalid_user", Message: "The username was invalid.", FieldErrors: map[string]string{"username": err.Error()}})
	}
	displayName, err := security.NormalizeDisplayName(request.DisplayName)
	if err != nil {
		return c.Status(400).JSON(models.APIError{Code: "invalid_user", Message: "The display name was invalid.", FieldErrors: map[string]string{"display_name": err.Error()}})
	}
	if request.Role != models.RoleAdmin && request.Role != models.RoleViewer {
		return v2Error(c, 400, "invalid_role", "Choose admin or viewer.", false)
	}
	lineupIDs, validLineups := validateLineupGrants(c, request.Role, request.LineupIDs)
	if !validLineups {
		return v2Error(c, 404, "lineup_not_found", "A selected lineup was not found.", false)
	}
	request.LineupIDs = lineupIDs
	if err := security.ValidatePassword(request.Password, username); err != nil {
		return c.Status(400).JSON(models.APIError{Code: "weak_password", Message: "Choose a stronger temporary password.", FieldErrors: map[string]string{"password": err.Error()}})
	}
	hash, err := security.HashPassword(request.Password)
	if err != nil {
		return v2Error(c, 500, "password_hash_failed", "The account could not be created.", true)
	}
	id, err := database.Db.CreateUser(c.UserContext(), username, displayName, hash, request.Role, true, request.LineupIDs)
	if err != nil {
		return v2Error(c, 409, "user_create_failed", "The username is already used or the lineup selection is invalid.", false)
	}
	auditSecurity(c, "user_create", "success", "user", strconv.FormatInt(id, 10), "", &id)
	user, _ := database.Db.GetUserByID(c.UserContext(), id)
	return c.Status(fiber.StatusCreated).JSON(user)
}

func V2UpdateStudioUser(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	request := struct {
		Role      string  `json:"role"`
		Disabled  bool    `json:"disabled"`
		LineupIDs []int64 `json:"lineup_ids"`
	}{}
	if err := decodeStrict(c, &request); err != nil || (request.Role != models.RoleAdmin && request.Role != models.RoleViewer) {
		return v2Error(c, 400, "invalid_user", "The user details were invalid.", false)
	}
	lineupIDs, validLineups := validateLineupGrants(c, request.Role, request.LineupIDs)
	if !validLineups {
		return v2Error(c, 404, "lineup_not_found", "A selected lineup was not found.", false)
	}
	request.LineupIDs = lineupIDs
	target, err := database.Db.GetUserByID(c.UserContext(), id)
	if err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	if target.Role == models.RoleAdmin && target.DisabledAt == nil && (request.Role != models.RoleAdmin || request.Disabled) {
		count, _ := database.Db.EnabledAdminCount(c.UserContext())
		if count <= 1 {
			return v2Error(c, 409, "last_admin", "The final enabled administrator cannot be disabled or demoted.", false)
		}
	}
	if err := database.Db.UpdateUser(c.UserContext(), id, request.Role, request.Disabled, request.LineupIDs); err != nil {
		if errors.Is(err, queries.ErrLastEnabledAdmin) {
			return v2Error(c, 409, "last_admin", "The final enabled administrator cannot be disabled or demoted.", false)
		}
		return v2Error(c, 409, "user_update_failed", "The user could not be updated.", false)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), id)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), id)
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), id)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "user_update", "success", "user", strconv.FormatInt(id, 10), "sessions_and_keys_revoked", &id)
	user, _ := database.Db.GetUserByID(c.UserContext(), id)
	return c.JSON(user)
}

func V2UpdateStudioUserProfile(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, fiber.StatusBadRequest, "invalid_user", "The user id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	if id == principal.UserID {
		return v2Error(c, fiber.StatusConflict, "self_profile_update_rejected", "Change your own identity from the Account page.", false)
	}
	request, err := normalizedIdentity(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code: "invalid_profile", Message: "The account identity was invalid.",
			FieldErrors: map[string]string{"profile": err.Error()},
		})
	}
	target, err := database.Db.GetUserByID(c.UserContext(), id)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "user_not_found", "The user was not found.", false)
	}
	usernameChanged := target.Username != request.Username
	displayNameChanged := target.DisplayName != request.DisplayName
	if !usernameChanged && !displayNameChanged {
		return c.JSON(target)
	}
	if err := database.Db.UpdateUserIdentity(c.UserContext(), id, request.Username, request.DisplayName); err != nil {
		if errors.Is(err, queries.ErrUsernameInUse) {
			return c.Status(fiber.StatusConflict).JSON(models.APIError{
				Code: "username_in_use", Message: "That username is already used.",
				FieldErrors: map[string]string{"username": "Choose a different username."},
			})
		}
		return v2Error(c, fiber.StatusInternalServerError, "profile_update_failed", "The account identity could not be updated.", true)
	}
	if usernameChanged {
		_ = database.Db.RevokeUserSessions(c.UserContext(), id)
		_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), id)
		streaming.DefaultManager.DisconnectUserSessionClients(id)
	}
	auditSecurity(c, "user_profile_update", "success", "user", strconv.FormatInt(id, 10),
		identityDetail(usernameChanged, displayNameChanged), &id)
	user, _ := database.Db.GetUserByID(c.UserContext(), id)
	return c.JSON(user)
}

func V2ResetStudioUserPassword(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	request := struct {
		Password string `json:"password"`
	}{}
	if decodeStrict(c, &request) != nil {
		return v2Error(c, 400, "invalid_request", "A temporary password is required.", false)
	}
	user, err := database.Db.GetUserByID(c.UserContext(), id)
	if err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	if err := security.ValidatePassword(request.Password, user.Username); err != nil {
		return c.Status(400).JSON(models.APIError{Code: "weak_password", Message: "Choose a stronger temporary password.", FieldErrors: map[string]string{"password": err.Error()}})
	}
	hash, err := security.HashPassword(request.Password)
	if err != nil {
		return v2Error(c, 500, "password_hash_failed", "The password could not be reset.", true)
	}
	if database.Db.SetUserPassword(c.UserContext(), id, hash, true) != nil {
		return v2Error(c, 500, "password_reset_failed", "The password could not be reset.", true)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), id)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), id)
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), id)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "password_reset", "success", "user", strconv.FormatInt(id, 10), "sessions_and_keys_revoked", &id)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2ResetStudioUserMFA(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	if _, err := database.Db.GetUserByID(c.UserContext(), id); err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	if err := database.Db.DisableMFA(c.UserContext(), id); err != nil {
		return v2Error(c, 500, "mfa_reset_failed", "MFA could not be reset.", true)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), id)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), id)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "mfa_reset", "success", "user", strconv.FormatInt(id, 10), "", &id)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2DeleteStudioUser(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	if id == principal.UserID {
		return v2Error(c, 409, "self_delete_rejected", "Use another administrator to delete this account.", false)
	}
	target, err := database.Db.GetUserByID(c.UserContext(), id)
	if err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	if target.Role == models.RoleAdmin && target.DisabledAt == nil {
		count, _ := database.Db.EnabledAdminCount(c.UserContext())
		if count <= 1 {
			return v2Error(c, 409, "last_admin", "The final enabled administrator cannot be deleted.", false)
		}
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	if err := database.Db.DeleteUser(c.UserContext(), id); err != nil {
		if errors.Is(err, queries.ErrLastEnabledAdmin) {
			return v2Error(c, 409, "last_admin", "The final enabled administrator cannot be deleted.", false)
		}
		return v2Error(c, 500, "user_delete_failed", "The user could not be deleted.", true)
	}
	auditSecurity(c, "user_delete", "success", "user", strconv.FormatInt(id, 10), "", nil, target.Username, target.DisplayName)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2RevokeStudioUserSessions(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	if _, err := database.Db.GetUserByID(c.UserContext(), id); err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	_ = database.Db.RevokeUserSessions(c.UserContext(), id)
	_ = database.Db.RevokeUserTrustedBrowsers(c.UserContext(), id)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "user_sessions_revoke", "success", "user", strconv.FormatInt(id, 10), "sessions_and_trusted_browsers_revoked", &id)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2RevokeStudioUserMediaKeys(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	if _, err := database.Db.GetUserByID(c.UserContext(), id); err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), id)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "user_media_keys_revoke", "success", "user", strconv.FormatInt(id, 10), "", &id)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2StudioUserMediaKeys(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("user_id"))
	if !ok {
		return v2Error(c, 400, "invalid_user", "The user id is invalid.", false)
	}
	if _, err := database.Db.GetUserByID(c.UserContext(), id); err != nil {
		return v2Error(c, 404, "user_not_found", "The user was not found.", false)
	}
	items, err := database.Db.ListMediaKeys(c.UserContext(), id)
	if err != nil {
		return v2Error(c, 500, "media_keys_unavailable", "Media keys could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"items": items})
}

func mediaKeyOutputLinks(key *models.MediaAccessKey) (map[string]map[string]string, error) {
	if key.RevokedAt != nil || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now().UTC())) || len(key.TokenCipher) == 0 {
		return nil, nil
	}
	plaintext, err := security.DecryptSecret(key.TokenCipher)
	if err != nil {
		return nil, err
	}
	token := string(plaintext)
	if !strings.HasPrefix(token, key.TokenPrefix+".") {
		return nil, errors.New("encrypted media credential does not match its prefix")
	}
	hash, err := security.HashToken("media", token)
	if err != nil || !hmac.Equal(hash, key.TokenHash) {
		return nil, errors.New("encrypted media credential does not match its authentication hash")
	}

	policy := settings.Current().Security
	publicMediaBase := policy.PublicMediaBaseURL
	if publicMediaBase == "" {
		publicMediaBase = policy.PublicBaseURL
	}
	localBase, _ := url.Parse(policy.LocalBaseURL)
	localTransportAvailable := localBase != nil &&
		(localBase.Scheme == "https" || policy.AllowLANHTTP)
	links := make(map[string]map[string]string, len(key.LineupIDs))
	for _, lineupID := range key.LineupIDs {
		id := strconv.FormatInt(lineupID, 10)
		suffix := "?access_token=" + url.QueryEscape(token)
		values := map[string]string{}
		if key.NetworkScope == "public" && publicMediaBase != "" {
			values["public_m3u"] = publicMediaBase + "/media/v1/lineups/" + id + "/playlist.m3u" + suffix
			values["public_xmltv"] = publicMediaBase + "/media/v1/lineups/" + id + "/guide.xml" + suffix
		}
		if localTransportAvailable {
			values["local_m3u"] = policy.LocalBaseURL + "/media/v1/lineups/" + id + "/playlist.m3u" + suffix
			values["local_xmltv"] = policy.LocalBaseURL + "/media/v1/lineups/" + id + "/guide.xml" + suffix
		}
		links[id] = values
	}
	return links, nil
}

func V2RevokeStudioUserMediaKey(c *fiber.Ctx) error {
	userID, userOK := security.ParsePositiveID(c.Params("user_id"))
	keyID, keyOK := security.ParsePositiveID(c.Params("key_id"))
	if !userOK || !keyOK {
		return v2Error(c, 400, "invalid_media_key", "The user or media key id is invalid.", false)
	}
	revoked, err := database.Db.RevokeMediaKey(c.UserContext(), keyID, userID)
	if err != nil {
		return v2Error(c, 500, "media_key_revoke_failed", "The media key could not be revoked.", true)
	}
	if !revoked {
		return v2Error(c, 404, "media_key_not_found", "The media key was not found.", false)
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("media_key", keyID, 0)
	auditSecurity(c, "admin_media_key_revoke", "success", "media_key", strconv.FormatInt(keyID, 10), "", &userID)
	return c.SendStatus(fiber.StatusNoContent)
}

func V2AccountMediaKeys(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	keys, err := database.Db.ListMediaKeys(c.UserContext(), principal.UserID)
	if err != nil {
		return v2Error(c, 500, "media_keys_unavailable", "Media keys could not be loaded.", true)
	}
	for index := range keys {
		keys[index].Links, err = mediaKeyOutputLinks(&keys[index])
		if err != nil {
			return v2Error(c, 500, "media_key_links_unavailable", "Device output links could not be loaded.", true)
		}
	}
	policy := settings.Current().Security
	return c.JSON(fiber.Map{
		"items":                  keys,
		"public_https_available": policy.PublicBaseURL != "" || policy.PublicMediaBaseURL != "",
		"retention_days":         policy.AuditRetentionDays,
	})
}

func V2CreateAccountMediaKey(c *fiber.Ctx) error {
	request := struct {
		Name         string     `json:"name"`
		NetworkScope string     `json:"network_scope"`
		LineupIDs    []int64    `json:"lineup_ids"`
		ExpiresAt    *time.Time `json:"expires_at"`
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, 400, "invalid_media_key", "The media key details were invalid.", false)
	}
	request.Name = strings.TrimSpace(request.Name)
	policy := settings.Current().Security
	localBase, _ := url.Parse(policy.LocalBaseURL)
	localTransportAvailable := localBase != nil &&
		(localBase.Scheme == "https" || policy.AllowLANHTTP)
	if len(request.Name) < 1 || len(request.Name) > 80 || strings.IndexFunc(request.Name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 ||
		(request.NetworkScope != "public" && request.NetworkScope != "lan") || len(request.LineupIDs) == 0 ||
		(request.ExpiresAt != nil && !request.ExpiresAt.After(time.Now().UTC())) ||
		(request.NetworkScope == "lan" && !localTransportAvailable) {
		return v2Error(c, 400, "invalid_media_key", "A name, scope, and at least one lineup are required.", false)
	}
	principal, _ := middleware.Principal(c)
	lineupIDs := make([]int64, 0, len(request.LineupIDs))
	seenLineups := make(map[int64]struct{}, len(request.LineupIDs))
	for _, lineupID := range request.LineupIDs {
		if _, duplicate := seenLineups[lineupID]; duplicate {
			continue
		}
		allowed, err := database.Db.UserCanAccessLineup(c.UserContext(), principal.UserID, principal.Role, lineupID)
		if err != nil || !allowed {
			return v2Error(c, 404, "lineup_not_found", "A selected lineup was not found.", false)
		}
		seenLineups[lineupID] = struct{}{}
		lineupIDs = append(lineupIDs, lineupID)
	}
	prefixRandom, err := security.RandomToken(9)
	if err != nil {
		return v2Error(c, 500, "media_key_create_failed", "The media key could not be created.", true)
	}
	secret, err := security.RandomToken(32)
	if err != nil {
		return v2Error(c, 500, "media_key_create_failed", "The media key could not be created.", true)
	}
	token := "xmk_" + prefixRandom + "." + secret
	hash, err := security.HashToken("media", token)
	if err != nil {
		return v2Error(c, 500, "media_key_create_failed", "The media key could not be created.", true)
	}
	ciphertext, err := security.EncryptSecret([]byte(token))
	if err != nil {
		return v2Error(c, 500, "media_key_create_failed", "The media key could not be created.", true)
	}
	key := &models.MediaAccessKey{UserID: principal.UserID, Name: request.Name, TokenPrefix: "xmk_" + prefixRandom, TokenHash: hash, TokenCipher: ciphertext, NetworkScope: request.NetworkScope, CreatedAt: time.Now().UTC(), ExpiresAt: request.ExpiresAt}
	key.LineupIDs = lineupIDs
	key.Links, err = mediaKeyOutputLinks(key)
	if err != nil {
		return v2Error(c, 500, "media_key_create_failed", "The media key could not be created.", true)
	}
	if err := database.Db.CreateMediaKey(c.UserContext(), key, lineupIDs); err != nil {
		return v2Error(c, 500, "media_key_create_failed", "The media key could not be created.", true)
	}
	auditSecurity(c, "media_key_create", "success", "media_key", strconv.FormatInt(key.ID, 10), request.NetworkScope, &principal.UserID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"key": key})
}

func V2RevokeAccountMediaKey(c *fiber.Ctx) error {
	id, ok := security.ParsePositiveID(c.Params("key_id"))
	if !ok {
		return v2Error(c, 400, "invalid_media_key", "The media key id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	revoked, err := database.Db.RevokeMediaKey(c.UserContext(), id, principal.UserID)
	if err != nil {
		return v2Error(c, 500, "media_key_revoke_failed", "The media key could not be revoked.", true)
	}
	if !revoked {
		return v2Error(c, 404, "media_key_not_found", "The media key was not found.", false)
	}
	streaming.DefaultManager.DisconnectAuthorizedClients("media_key", id, 0)
	auditSecurity(c, "media_key_revoke", "success", "media_key", strconv.FormatInt(id, 10), "", &principal.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

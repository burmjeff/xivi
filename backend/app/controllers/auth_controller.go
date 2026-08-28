package controllers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

type loginAttempt struct {
	Failures int
	Window   time.Time
}

var loginAttempts = struct {
	sync.Mutex
	items map[string]loginAttempt
}{items: map[string]loginAttempt{}}

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
	if principal, ok := middleware.Principal(c); ok {
		id := principal.UserID
		actor = &id
		actorUsername = principal.Username
	}
	targetUsername := ""
	if len(knownTargetUsername) > 0 {
		targetUsername = knownTargetUsername[0]
	} else if targetUserID != nil {
		if target, err := database.Db.GetUserByID(c.UserContext(), *targetUserID); err == nil {
			targetUsername = target.Username
		}
	}
	ip := security.RequestNetworkInfo(c).IP.String()
	_ = database.Db.Audit(c.UserContext(), models.SecurityAuditEvent{
		ActorUserID: actor, ActorUsername: actorUsername, TargetUserID: targetUserID,
		TargetUsername: targetUsername, Action: action, Outcome: outcome, ResourceType: resourceType,
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

func loginRateKeys(c *fiber.Ctx, username string) (string, string) {
	ip := security.RequestNetworkInfo(c).IP.String()
	return "account|" + ip + "|" + username, "address|" + ip
}

func loginRateLimited(accountKey, addressKey string) bool {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	now := time.Now()
	blocked := false
	for key, maximum := range map[string]int{accountKey: 5, addressKey: 30} {
		entry := loginAttempts.items[key]
		if entry.Window.IsZero() || now.Sub(entry.Window) > 15*time.Minute {
			delete(loginAttempts.items, key)
			continue
		}
		blocked = blocked || entry.Failures >= maximum
	}
	return blocked
}

func recordLoginFailure(accountKey, addressKey string) int {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	now := time.Now()
	accountFailures := 0
	for _, key := range []string{accountKey, addressKey} {
		entry := loginAttempts.items[key]
		if entry.Window.IsZero() || now.Sub(entry.Window) > 15*time.Minute {
			entry = loginAttempt{Window: now}
		}
		entry.Failures++
		loginAttempts.items[key] = entry
		if key == accountKey {
			accountFailures = entry.Failures
		}
	}
	if len(loginAttempts.items) > 10000 {
		for candidate, value := range loginAttempts.items {
			if now.Sub(value.Window) > 15*time.Minute {
				delete(loginAttempts.items, candidate)
			}
		}
		if len(loginAttempts.items) > 10000 {
			// The map is an abuse-control cache, not durable security state. Evict
			// one arbitrary bucket rather than allowing attacker-selected keys to
			// consume memory without bound.
			for candidate := range loginAttempts.items {
				delete(loginAttempts.items, candidate)
				break
			}
		}
	}
	return accountFailures
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
	return models.SessionPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role,
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

func V2Login(c *fiber.Ctx) error {
	if _, ok := security.TransportScope(c); !ok || !middleware.ValidRequestOrigin(c) {
		return v2Error(c, fiber.StatusForbidden, "transport_rejected", "Use public HTTPS or an explicitly trusted LAN address.", false)
	}
	count, err := database.Db.UserCount(c.UserContext())
	if err != nil || count == 0 {
		return v2Error(c, fiber.StatusServiceUnavailable, "bootstrap_required", "The first-run administrator is not available. Restart Xivi and try again.", false)
	}
	request := struct {
		Username string `json:"username"`
		Password string `json:"password"`
		MFACode  string `json:"mfa_code"`
	}{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_login", "The login request was invalid.", false)
	}
	username, normalizeErr := security.NormalizeUsername(request.Username)
	accountKey, addressKey := loginRateKeys(c, username)
	if loginRateLimited(accountKey, addressKey) {
		c.Set(fiber.HeaderRetryAfter, "900")
		auditSecurity(c, "login", "denied", "user", "", "rate_limited", nil)
		return v2Error(c, fiber.StatusTooManyRequests, "login_rate_limited", "Too many attempts. Try again later.", true)
	}
	user, lookupErr := database.Db.GetUserByUsername(c.UserContext(), username)
	hash := security.DummyPasswordHash()
	if lookupErr == nil {
		hash = user.PasswordHash
	}
	passwordOK, verifyErr := security.VerifyPassword(request.Password, hash)
	if errors.Is(verifyErr, security.ErrPasswordVerifierBusy) {
		c.Set(fiber.HeaderRetryAfter, "2")
		auditSecurity(c, "login", "denied", "user", "", "verification_capacity", nil)
		return v2Error(c, fiber.StatusTooManyRequests, "login_rate_limited", "Too many attempts. Try again shortly.", true)
	}
	valid := normalizeErr == nil && lookupErr == nil && user.DisabledAt == nil && passwordOK
	if valid && user.MFAEnabled {
		valid = verifyUserMFA(c, user, request.MFACode)
	}
	if !valid {
		failures := recordLoginFailure(accountKey, addressKey)
		auditSecurity(c, "login", "failure", "user", "", "invalid_credentials", nil)
		// A short bounded delay raises the cost of online guessing while the
		// request and bucket ceilings prevent attacker-controlled lockout.
		delay := 100 * time.Millisecond * time.Duration(1<<min(failures-1, 4))
		time.Sleep(delay)
		return v2Error(c, fiber.StatusUnauthorized, "invalid_credentials", "The username, password, or verification code was invalid.", false)
	}
	loginAttempts.Lock()
	delete(loginAttempts.items, accountKey)
	loginAttempts.Unlock()
	principal, err := createBrowserSession(c, user, !user.MFAEnabled || strings.TrimSpace(request.MFACode) != "")
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "session_failed", "The session could not be created.", true)
	}
	target := user.ID
	auditSecurity(c, "login", "success", "user", strconv.FormatInt(user.ID, 10), "", &target)
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
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), user.ID)
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
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, principal.UserID)
	if session, ok := middleware.CurrentSession(c); ok {
		middleware.ClearSessionCookie(c, session.TransportScope)
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
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The audit cursor is invalid.", false)
		}
		beforeID, err = strconv.ParseInt(string(decoded), 10, 64)
		if err != nil || beforeID < 1 {
			return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The audit cursor is invalid.", false)
		}
	}
	items, total, err := database.Db.ListSecurityAuditEvents(c.UserContext(), beforeID, limit)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "security_audit_unavailable", "The security audit could not be loaded.", true)
	}
	var nextCursor *string
	if len(items) == limit && len(items) > 0 {
		value := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(items[len(items)-1].ID, 10)))
		nextCursor = &value
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nextCursor, "total": total})
}

type userWriteRequest struct {
	Username  string  `json:"username"`
	Password  string  `json:"password"`
	Role      string  `json:"role"`
	Disabled  bool    `json:"disabled"`
	LineupIDs []int64 `json:"lineup_ids"`
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
	id, err := database.Db.CreateUser(c.UserContext(), username, hash, request.Role, true, request.LineupIDs)
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
	_ = database.Db.RevokeUserMediaKeys(c.UserContext(), id)
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "user_update", "success", "user", strconv.FormatInt(id, 10), "sessions_and_keys_revoked", &id)
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
	auditSecurity(c, "user_delete", "success", "user", strconv.FormatInt(id, 10), "", nil, target.Username)
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
	streaming.DefaultManager.DisconnectAuthorizedClients("", 0, id)
	auditSecurity(c, "user_sessions_revoke", "success", "user", strconv.FormatInt(id, 10), "", &id)
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

	localBase, _ := url.Parse(settings.APP_SETTINGS.Security.LocalBaseURL)
	localTransportAvailable := localBase != nil &&
		(localBase.Scheme == "https" || settings.APP_SETTINGS.Security.AllowLANHTTP)
	links := make(map[string]map[string]string, len(key.LineupIDs))
	for _, lineupID := range key.LineupIDs {
		id := strconv.FormatInt(lineupID, 10)
		suffix := "?access_token=" + url.QueryEscape(token)
		values := map[string]string{}
		if key.NetworkScope == "public" && settings.APP_SETTINGS.Security.PublicBaseURL != "" {
			values["public_m3u"] = settings.APP_SETTINGS.Security.PublicBaseURL + "/media/v1/lineups/" + id + "/playlist.m3u" + suffix
			values["public_xmltv"] = settings.APP_SETTINGS.Security.PublicBaseURL + "/media/v1/lineups/" + id + "/guide.xml" + suffix
		}
		if localTransportAvailable {
			values["local_m3u"] = settings.APP_SETTINGS.Security.LocalBaseURL + "/media/v1/lineups/" + id + "/playlist.m3u" + suffix
			values["local_xmltv"] = settings.APP_SETTINGS.Security.LocalBaseURL + "/media/v1/lineups/" + id + "/guide.xml" + suffix
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
	return c.JSON(fiber.Map{
		"items":                  keys,
		"public_https_available": settings.APP_SETTINGS.Security.PublicBaseURL != "",
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
	localBase, _ := url.Parse(settings.APP_SETTINGS.Security.LocalBaseURL)
	localTransportAvailable := localBase != nil &&
		(localBase.Scheme == "https" || settings.APP_SETTINGS.Security.AllowLANHTTP)
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

package models

import "time"

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type User struct {
	ID                 int64      `db:"id" json:"id"`
	Username           string     `db:"username" json:"username"`
	DisplayName        string     `db:"display_name" json:"display_name"`
	PasswordHash       string     `db:"password_hash" json:"-"`
	Role               string     `db:"role" json:"role"`
	MustChangePassword bool       `db:"must_change_password" json:"must_change_password"`
	InitialPassword    bool       `db:"initial_password" json:"-"`
	AuthVersion        int64      `db:"auth_version" json:"-"`
	DisabledAt         *time.Time `db:"disabled_at" json:"disabled_at,omitempty"`
	CreatedAt          time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at" json:"updated_at"`
	MFAEnabled         bool       `db:"mfa_enabled" json:"mfa_enabled"`
	LineupIDs          []int64    `json:"lineup_ids"`
}

type SessionPrincipal struct {
	UserID             int64                 `json:"user_id"`
	Username           string                `json:"username"`
	DisplayName        string                `json:"display_name"`
	Role               string                `json:"role"`
	MustChangePassword bool                  `json:"must_change_password"`
	MFAEnabled         bool                  `json:"mfa_enabled"`
	MFARequired        bool                  `json:"mfa_required"`
	LineupIDs          []int64               `json:"lineup_ids"`
	CSRFToken          string                `json:"csrf_token,omitempty"`
	SecurityNotice     *AuthenticationNotice `json:"security_notice,omitempty"`
}

type AuthenticationNotice struct {
	Kind    string    `json:"kind"`
	Count   int       `json:"count"`
	LastAt  time.Time `json:"last_at"`
	LastIP  string    `json:"last_ip,omitempty"`
	Message string    `json:"message"`
}

func (p SessionPrincipal) IsAdmin() bool { return p.Role == RoleAdmin }

type AuthSession struct {
	ID                 int64      `db:"id"`
	TokenHash          []byte     `db:"token_hash"`
	UserID             int64      `db:"user_id"`
	AuthVersion        int64      `db:"auth_version"`
	TransportScope     string     `db:"transport_scope"`
	MFAVerified        bool       `db:"mfa_verified"`
	ReauthenticatedAt  time.Time  `db:"reauthenticated_at"`
	CreatedAt          time.Time  `db:"created_at"`
	LastSeenAt         time.Time  `db:"last_seen_at"`
	IdleExpiresAt      time.Time  `db:"idle_expires_at"`
	AbsoluteExpiresAt  time.Time  `db:"absolute_expires_at"`
	RevokedAt          *time.Time `db:"revoked_at"`
	ClientIP           string     `db:"client_ip"`
	UserAgentHash      []byte     `db:"user_agent_hash"`
	Username           string     `db:"username"`
	DisplayName        string     `db:"display_name"`
	Role               string     `db:"role"`
	MustChangePassword bool       `db:"must_change_password"`
	UserDisabledAt     *time.Time `db:"user_disabled_at"`
	MFAEnabled         bool       `db:"mfa_enabled"`
}

type AuthSessionMetadata struct {
	ID                int64      `db:"id" json:"id"`
	TransportScope    string     `db:"transport_scope" json:"transport_scope"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	LastSeenAt        time.Time  `db:"last_seen_at" json:"last_seen_at"`
	IdleExpiresAt     time.Time  `db:"idle_expires_at" json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time  `db:"absolute_expires_at" json:"absolute_expires_at"`
	RevokedAt         *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	ClientIP          string     `db:"client_ip" json:"client_ip"`
}

type LoginChallenge struct {
	ID             int64      `db:"id"`
	TokenHash      []byte     `db:"token_hash"`
	UserID         int64      `db:"user_id"`
	AuthVersion    int64      `db:"auth_version"`
	TransportScope string     `db:"transport_scope"`
	CreatedAt      time.Time  `db:"created_at"`
	ExpiresAt      time.Time  `db:"expires_at"`
	ConsumedAt     *time.Time `db:"consumed_at"`
	FailureCount   int        `db:"failure_count"`
	ClientIP       string     `db:"client_ip"`
	UserAgentHash  []byte     `db:"user_agent_hash"`
}

type TrustedBrowser struct {
	ID             int64      `db:"id"`
	TokenHash      []byte     `db:"token_hash" json:"-"`
	UserID         int64      `db:"user_id" json:"-"`
	AuthVersion    int64      `db:"auth_version" json:"-"`
	TransportScope string     `db:"transport_scope" json:"transport_scope"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	LastUsedAt     time.Time  `db:"last_used_at" json:"last_used_at"`
	ExpiresAt      time.Time  `db:"expires_at" json:"expires_at"`
	RevokedAt      *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	CreatedIP      string     `db:"created_ip" json:"created_ip"`
	LastUsedIP     string     `db:"last_used_ip" json:"last_used_ip"`
	UserAgent      string     `db:"user_agent" json:"user_agent"`
	UserAgentHash  []byte     `db:"user_agent_hash" json:"-"`
	Current        bool       `db:"-" json:"current"`
}

type AuthThrottleBucket struct {
	ID                  int64      `db:"id" json:"id"`
	BucketHash          []byte     `db:"bucket_hash" json:"-"`
	SubjectType         string     `db:"subject_type" json:"subject_type"`
	UserID              *int64     `db:"user_id" json:"user_id,omitempty"`
	Username            string     `db:"username" json:"username,omitempty"`
	DisplayName         string     `db:"display_name" json:"display_name,omitempty"`
	ClientIP            string     `db:"client_ip" json:"client_ip,omitempty"`
	NetworkPrefix       string     `db:"network_prefix" json:"network_prefix,omitempty"`
	ConsecutiveFailures int        `db:"consecutive_failures" json:"consecutive_failures"`
	PasswordFailures    int        `db:"password_failures" json:"password_failures"`
	MFAFailures         int        `db:"mfa_failures" json:"mfa_failures"`
	PendingMFANotice    int        `db:"pending_mfa_notice" json:"-"`
	DeniedRequests      int        `db:"denied_requests" json:"denied_requests"`
	WindowStartedAt     time.Time  `db:"window_started_at" json:"window_started_at"`
	LastFailedAt        time.Time  `db:"last_failed_at" json:"last_failed_at"`
	BlockedUntil        *time.Time `db:"blocked_until" json:"blocked_until,omitempty"`
	LastFactor          string     `db:"last_factor" json:"last_factor"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
}

type AuthThrottleInput struct {
	BucketHash    []byte
	SubjectType   string
	UserID        *int64
	ClientIP      string
	NetworkPrefix string
}

type AuthThrottleDecision struct {
	Blocked           bool
	RetryAfter        time.Duration
	ChallengeRequired bool
	NewlyBlocked      bool
	FailureCount      int
}

type BotChallenge struct {
	ID          int64      `db:"id"`
	TokenHash   []byte     `db:"token_hash"`
	AccountHash []byte     `db:"account_hash"`
	AddressHash []byte     `db:"address_hash"`
	Difficulty  int        `db:"difficulty"`
	CreatedAt   time.Time  `db:"created_at"`
	ExpiresAt   time.Time  `db:"expires_at"`
	UsedAt      *time.Time `db:"used_at"`
}

type LineupGrant struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

type MediaAccessKey struct {
	ID            int64                        `db:"id" json:"id"`
	UserID        int64                        `db:"user_id" json:"user_id"`
	Username      string                       `db:"username" json:"username,omitempty"`
	Name          string                       `db:"name" json:"name"`
	TokenPrefix   string                       `db:"token_prefix" json:"token_prefix"`
	TokenHash     []byte                       `db:"token_hash" json:"-"`
	TokenCipher   []byte                       `db:"token_cipher" json:"-"`
	NetworkScope  string                       `db:"network_scope" json:"network_scope"`
	CreatedAt     time.Time                    `db:"created_at" json:"created_at"`
	ExpiresAt     *time.Time                   `db:"expires_at" json:"expires_at,omitempty"`
	LastUsedAt    *time.Time                   `db:"last_used_at" json:"last_used_at,omitempty"`
	LastUsedIP    string                       `db:"last_used_ip" json:"last_used_ip,omitempty"`
	RevokedAt     *time.Time                   `db:"revoked_at" json:"revoked_at,omitempty"`
	LineupIDs     []int64                      `json:"lineup_ids"`
	Links         map[string]map[string]string `db:"-" json:"links,omitempty"`
	OutputAliases []MediaOutputAlias           `db:"-" json:"-"`
}

type MediaOutputAlias struct {
	ID         int64     `db:"id" json:"-"`
	MediaKeyID int64     `db:"media_key_id" json:"-"`
	LineupID   int64     `db:"lineup_id" json:"-"`
	CodeHash   []byte    `db:"code_hash" json:"-"`
	CodeCipher []byte    `db:"code_cipher" json:"-"`
	CreatedAt  time.Time `db:"created_at" json:"-"`
}

type SecurityAuditEvent struct {
	ID                int64     `db:"id" json:"id"`
	ActorUserID       *int64    `db:"actor_user_id" json:"actor_user_id,omitempty"`
	ActorUsername     string    `db:"actor_username" json:"actor_username,omitempty"`
	ActorDisplayName  string    `db:"actor_display_name" json:"actor_display_name,omitempty"`
	TargetUserID      *int64    `db:"target_user_id" json:"target_user_id,omitempty"`
	TargetUsername    string    `db:"target_username" json:"target_username,omitempty"`
	TargetDisplayName string    `db:"target_display_name" json:"target_display_name,omitempty"`
	Action            string    `db:"action" json:"action"`
	Outcome           string    `db:"outcome" json:"outcome"`
	ResourceType      string    `db:"resource_type" json:"resource_type,omitempty"`
	ResourceID        string    `db:"resource_id" json:"resource_id,omitempty"`
	ClientIP          string    `db:"client_ip" json:"client_ip,omitempty"`
	Detail            string    `db:"detail" json:"detail,omitempty"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
}

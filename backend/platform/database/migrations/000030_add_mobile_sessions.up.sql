-- Revocable native-app sessions. Access tokens are short-lived; refresh-token
-- history is retained until the session expires so replay can revoke the whole
-- device session instead of being mistaken for an ordinary invalid token.

CREATE TABLE mobile_session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    auth_version INTEGER NOT NULL,
    device_name VARCHAR(80) NOT NULL,
    access_token_hash BLOB NOT NULL UNIQUE,
    refresh_token_hash BLOB NOT NULL UNIQUE,
    mfa_verified BOOLEAN NOT NULL DEFAULT FALSE,
    reauthenticated_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP NOT NULL,
    access_expires_at TIMESTAMP NOT NULL,
    refresh_expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP NULL,
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent_hash BLOB NULL
);
CREATE INDEX idx_mobile_session_user_active
    ON mobile_session(user_id, revoked_at, refresh_expires_at);
CREATE INDEX idx_mobile_session_expiry
    ON mobile_session(refresh_expires_at, revoked_at);

CREATE TABLE mobile_refresh_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mobile_session_id INTEGER NOT NULL REFERENCES mobile_session(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE,
    used_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_mobile_refresh_history_session
    ON mobile_refresh_history(mobile_session_id, expires_at);
CREATE INDEX idx_mobile_refresh_history_expiry
    ON mobile_refresh_history(expires_at);

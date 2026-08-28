CREATE TABLE auth_login_challenge (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash BLOB NOT NULL UNIQUE,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    auth_version INTEGER NOT NULL,
    transport_scope VARCHAR(16) NOT NULL CHECK (transport_scope IN ('https', 'lan_http')),
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP NULL,
    failure_count INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent_hash BLOB NOT NULL
);
CREATE INDEX idx_auth_login_challenge_expiry ON auth_login_challenge(expires_at, consumed_at);
CREATE INDEX idx_auth_login_challenge_user ON auth_login_challenge(user_id, created_at DESC);

CREATE TABLE trusted_browser (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash BLOB NOT NULL UNIQUE,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    auth_version INTEGER NOT NULL,
    transport_scope VARCHAR(16) NOT NULL CHECK (transport_scope IN ('https', 'lan_http')),
    created_at TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP NULL,
    created_ip VARCHAR(64) NOT NULL DEFAULT '',
    last_used_ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(255) NOT NULL DEFAULT '',
    user_agent_hash BLOB NOT NULL
);
CREATE INDEX idx_trusted_browser_user_active ON trusted_browser(user_id, revoked_at, expires_at, last_used_at DESC);
CREATE INDEX idx_trusted_browser_expiry ON trusted_browser(expires_at, revoked_at);

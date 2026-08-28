-- Authentication, authorization, media credentials, and bounded security audit.

CREATE TABLE app_user (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(64) NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    role VARCHAR(16) NOT NULL CHECK (role IN ('admin', 'viewer')),
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    auth_version INTEGER NOT NULL DEFAULT 1,
    disabled_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_lineup (
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    lineup_id INTEGER NOT NULL REFERENCES template(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, lineup_id)
);
CREATE INDEX idx_user_lineup_lineup ON user_lineup(lineup_id, user_id);

CREATE TABLE auth_session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash BLOB NOT NULL UNIQUE,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    auth_version INTEGER NOT NULL,
    transport_scope VARCHAR(16) NOT NULL CHECK (transport_scope IN ('https', 'lan_http')),
    mfa_verified BOOLEAN NOT NULL DEFAULT FALSE,
    reauthenticated_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP NOT NULL,
    idle_expires_at TIMESTAMP NOT NULL,
    absolute_expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP NULL,
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent_hash BLOB NULL
);
CREATE INDEX idx_auth_session_user_active ON auth_session(user_id, revoked_at, absolute_expires_at);
CREATE INDEX idx_auth_session_expiry ON auth_session(absolute_expires_at, idle_expires_at);

CREATE TABLE user_mfa (
    user_id INTEGER PRIMARY KEY REFERENCES app_user(id) ON DELETE CASCADE,
    encrypted_secret BLOB NOT NULL,
    last_counter INTEGER NOT NULL DEFAULT -1,
    enabled_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE user_mfa_recovery_code (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    code_hash BLOB NOT NULL,
    used_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_mfa_recovery_user_unused ON user_mfa_recovery_code(user_id, used_at);

CREATE TABLE media_access_key (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    name VARCHAR(80) NOT NULL,
    token_prefix VARCHAR(24) NOT NULL UNIQUE,
    token_hash BLOB NOT NULL UNIQUE,
    network_scope VARCHAR(16) NOT NULL CHECK (network_scope IN ('public', 'lan')),
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NULL,
    last_used_at TIMESTAMP NULL,
    last_used_ip VARCHAR(64) NOT NULL DEFAULT '',
    revoked_at TIMESTAMP NULL
);
CREATE INDEX idx_media_key_user_active ON media_access_key(user_id, revoked_at, expires_at);

CREATE TABLE media_key_lineup (
    media_key_id INTEGER NOT NULL REFERENCES media_access_key(id) ON DELETE CASCADE,
    lineup_id INTEGER NOT NULL REFERENCES template(id) ON DELETE CASCADE,
    PRIMARY KEY (media_key_id, lineup_id)
);
CREATE INDEX idx_media_key_lineup_lineup ON media_key_lineup(lineup_id, media_key_id);

CREATE TABLE security_audit_event (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id INTEGER NULL REFERENCES app_user(id) ON DELETE SET NULL,
    target_user_id INTEGER NULL REFERENCES app_user(id) ON DELETE SET NULL,
    action VARCHAR(64) NOT NULL,
    outcome VARCHAR(16) NOT NULL CHECK (outcome IN ('success', 'failure', 'denied')),
    resource_type VARCHAR(32) NOT NULL DEFAULT '',
    resource_id VARCHAR(128) NOT NULL DEFAULT '',
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_security_audit_created ON security_audit_event(created_at DESC, id DESC);
CREATE INDEX idx_security_audit_actor ON security_audit_event(actor_user_id, created_at DESC);

ALTER TABLE template ADD COLUMN virtual_tuner_token_version INTEGER NOT NULL DEFAULT 1;

-- TV enrollment is deliberately independent from expiring browser/mobile sessions.
CREATE TABLE tv_device (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    auth_version INTEGER NOT NULL,
    device_name TEXT NOT NULL,
    key_thumbprint TEXT NOT NULL,
    refresh_token_hash BLOB NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP NULL,
    client_ip TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_tv_device_user ON tv_device(user_id, revoked_at);
CREATE TABLE tv_access_token (
    token_hash BLOB PRIMARY KEY,
    device_id INTEGER NOT NULL REFERENCES tv_device(id) ON DELETE CASCADE,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_tv_access_expiry ON tv_access_token(expires_at);
CREATE TABLE tv_pairing (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_code_hash BLOB NOT NULL UNIQUE,
    user_code_hash BLOB NOT NULL UNIQUE,
    device_name TEXT NOT NULL,
    key_thumbprint TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    decision TEXT NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending','approved','denied')),
    user_id INTEGER NULL REFERENCES app_user(id) ON DELETE CASCADE,
    auth_version INTEGER NULL,
    device_id INTEGER NULL REFERENCES tv_device(id) ON DELETE CASCADE,
    refresh_token_cipher BLOB NULL,
    last_polled_at TIMESTAMP NULL
);
CREATE INDEX idx_tv_pairing_expiry ON tv_pairing(expires_at);
-- Replay evidence is shared across processes and retained through its validity window.
CREATE TABLE tv_dpop_replay (
    key_thumbprint TEXT NOT NULL,
    jti TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    PRIMARY KEY(key_thumbprint, jti)
);
CREATE INDEX idx_tv_dpop_expiry ON tv_dpop_replay(expires_at);
CREATE TABLE viewer_preferences (
    user_id INTEGER PRIMARY KEY REFERENCES app_user(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL DEFAULT 0,
    document TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

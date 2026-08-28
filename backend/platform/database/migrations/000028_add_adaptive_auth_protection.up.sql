CREATE TABLE auth_throttle_bucket (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bucket_hash BLOB NOT NULL UNIQUE,
    subject_type VARCHAR(16) NOT NULL CHECK (subject_type IN ('account', 'address', 'network', 'pair')),
    user_id INTEGER NULL REFERENCES app_user(id) ON DELETE SET NULL,
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    network_prefix VARCHAR(80) NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    password_failures INTEGER NOT NULL DEFAULT 0 CHECK (password_failures >= 0),
    mfa_failures INTEGER NOT NULL DEFAULT 0 CHECK (mfa_failures >= 0),
    pending_mfa_notice INTEGER NOT NULL DEFAULT 0 CHECK (pending_mfa_notice >= 0),
    denied_requests INTEGER NOT NULL DEFAULT 0 CHECK (denied_requests >= 0),
    window_started_at TIMESTAMP NOT NULL,
    last_failed_at TIMESTAMP NOT NULL,
    blocked_until TIMESTAMP NULL,
    last_factor VARCHAR(16) NOT NULL DEFAULT '' CHECK (last_factor IN ('', 'password', 'mfa')),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_auth_throttle_active ON auth_throttle_bucket(blocked_until, last_failed_at DESC);
CREATE INDEX idx_auth_throttle_user ON auth_throttle_bucket(user_id, subject_type, last_failed_at DESC);

CREATE TABLE auth_bot_challenge (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash BLOB NOT NULL UNIQUE,
    account_hash BLOB NOT NULL,
    address_hash BLOB NOT NULL,
    difficulty INTEGER NOT NULL CHECK (difficulty BETWEEN 8 AND 24),
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP NULL
);
CREATE INDEX idx_auth_bot_challenge_expiry ON auth_bot_challenge(expires_at, used_at);

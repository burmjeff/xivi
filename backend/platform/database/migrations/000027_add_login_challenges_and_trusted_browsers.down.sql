DROP INDEX IF EXISTS idx_trusted_browser_expiry;
DROP INDEX IF EXISTS idx_trusted_browser_user_active;
DROP TABLE IF EXISTS trusted_browser;

DROP INDEX IF EXISTS idx_auth_login_challenge_user;
DROP INDEX IF EXISTS idx_auth_login_challenge_expiry;
DROP TABLE IF EXISTS auth_login_challenge;

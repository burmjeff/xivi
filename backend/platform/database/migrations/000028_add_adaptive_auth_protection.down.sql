DROP INDEX IF EXISTS idx_auth_bot_challenge_expiry;
DROP TABLE IF EXISTS auth_bot_challenge;

DROP INDEX IF EXISTS idx_auth_throttle_user;
DROP INDEX IF EXISTS idx_auth_throttle_active;
DROP TABLE IF EXISTS auth_throttle_bucket;

-- Distinguish the one known first-run credential from later administrator
-- password resets, which also require a password change but are not defaults.
ALTER TABLE app_user ADD COLUMN initial_password BOOLEAN NOT NULL DEFAULT FALSE;

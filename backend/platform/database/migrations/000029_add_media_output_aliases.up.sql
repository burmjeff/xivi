-- +migrate Up

-- Short output aliases are separate from the full media credential embedded in
-- generated playlists. Each alias grants only one existing key/lineup pair and
-- inherits that key's network scope, expiry, revocation, and account status.
CREATE TABLE media_output_alias (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_key_id INTEGER NOT NULL REFERENCES media_access_key(id) ON DELETE CASCADE,
    lineup_id INTEGER NOT NULL REFERENCES template(id) ON DELETE CASCADE,
    code_hash BLOB NOT NULL UNIQUE,
    code_cipher BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE(media_key_id, lineup_id),
    FOREIGN KEY(media_key_id, lineup_id)
        REFERENCES media_key_lineup(media_key_id, lineup_id) ON DELETE CASCADE
);
CREATE INDEX idx_media_output_alias_key ON media_output_alias(media_key_id, lineup_id);

-- +migrate Up

ALTER TABLE templatechannelitem ADD COLUMN match_method VARCHAR(32) NOT NULL DEFAULT 'legacy';
ALTER TABLE templatechannelitem ADD COLUMN match_score REAL NULL;
ALTER TABLE templatechannelitem ADD COLUMN runner_up_score REAL NULL;
ALTER TABLE templatechannelitem ADD COLUMN matcher_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE templatechannelitem ADD COLUMN manual_locked BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_templatechannelitem_match_method ON templatechannelitem(match_method);

-- A user deleting a proposed/automatic match is a durable decision. Store the
-- stable provider identity rather than the volatile playlist-channel row ID.
CREATE TABLE channelmatchrejection (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL,
    playlist_id INTEGER NOT NULL,
    tvg_id_norm VARCHAR(255) NOT NULL DEFAULT '',
    name_norm VARCHAR(255) NOT NULL,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE,
    FOREIGN KEY (playlist_id) REFERENCES playlist(id) ON DELETE CASCADE,
    UNIQUE(channel_id, playlist_id, tvg_id_norm, name_norm) ON CONFLICT IGNORE
);

CREATE INDEX idx_channelmatchrejection_lookup
ON channelmatchrejection(channel_id, playlist_id, tvg_id_norm, name_norm);

-- Enforce the application's one-source-per-playlist rule at the database
-- boundary as matching workers may finish concurrently.
CREATE TRIGGER templatechannelitem_one_source_per_playlist
BEFORE INSERT ON templatechannelitem
WHEN EXISTS (
    SELECT 1
    FROM templatechannelitem existing
    JOIN playlistchannel existing_pc ON existing_pc.id = existing.playlist_channel_id
    JOIN playlistgroup existing_pg ON existing_pg.id = existing_pc.group_id
    JOIN playlistchannel new_pc ON new_pc.id = NEW.playlist_channel_id
    JOIN playlistgroup new_pg ON new_pg.id = new_pc.group_id
    WHERE existing.channel_id = NEW.channel_id
      AND existing_pg.playlist_id = new_pg.playlist_id
)
BEGIN
    SELECT RAISE(IGNORE);
END;

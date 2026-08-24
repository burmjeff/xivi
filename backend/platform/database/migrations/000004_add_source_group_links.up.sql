-- +migrate Up

-- A source-backed lineup group is a durable subscription, not a fuzzy channel
-- match. Keep its state separate from the legacy dynamic flags so failed or
-- temporarily empty imports cannot silently destroy the last valid lineup.
CREATE TABLE lineup_group_source_link (
    group_id INTEGER PRIMARY KEY,
    playlist_id INTEGER NOT NULL,
    source_group_id INTEGER NULL,
    source_group_name VARCHAR(255) NOT NULL,
    follow_group_name BOOLEAN NOT NULL DEFAULT true,
    follow_channel_names BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    last_synced_at DATETIME NULL,
    last_error TEXT NOT NULL DEFAULT '',
    added_count INTEGER NOT NULL DEFAULT 0,
    updated_count INTEGER NOT NULL DEFAULT 0,
    removed_count INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (group_id) REFERENCES templategroup(id) ON DELETE CASCADE,
    FOREIGN KEY (playlist_id) REFERENCES playlist(id) ON DELETE CASCADE,
    FOREIGN KEY (source_group_id) REFERENCES playlistgroup(id) ON DELETE SET NULL
);

-- Provenance records allow reconciliation to remove only membership managed by
-- this subscription. The stable identity survives source-channel row churn.
CREATE TABLE lineup_group_source_member (
    group_id INTEGER NOT NULL,
    template_channel_id INTEGER NOT NULL,
    source_channel_id INTEGER NULL,
    source_identity VARCHAR(320) NOT NULL,
    last_source_name VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (group_id, template_channel_id),
    UNIQUE (group_id, source_identity),
    FOREIGN KEY (group_id) REFERENCES lineup_group_source_link(group_id) ON DELETE CASCADE,
    FOREIGN KEY (template_channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE,
    FOREIGN KEY (source_channel_id) REFERENCES playlistchannel(id) ON DELETE SET NULL
);

CREATE INDEX idx_source_group_link_playlist ON lineup_group_source_link(playlist_id);
CREATE INDEX idx_source_group_link_source_group ON lineup_group_source_link(source_group_id);
CREATE INDEX idx_source_group_member_source_channel ON lineup_group_source_member(source_channel_id);

-- Preserve existing installations. The first sync adopts legacy channels by
-- TVG ID before creating anything new.
INSERT INTO lineup_group_source_link (
    group_id,
    playlist_id,
    source_group_id,
    source_group_name,
    follow_group_name,
    follow_channel_names,
    status
)
SELECT
    tg.id,
    pg.playlist_id,
    pg.id,
    pg.name,
    true,
    true,
    'pending'
FROM templategroup tg
JOIN playlistgroup pg ON pg.id = tg.dynamicgroup
WHERE COALESCE(tg.dynamic, false) = true;

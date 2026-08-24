-- +migrate Up

-- Preserve one automatic match per playlist while allowing people to build an
-- explicit primary/backup stack from any source, including the same playlist.
DROP TRIGGER IF EXISTS templatechannelitem_one_source_per_playlist;

CREATE TRIGGER templatechannelitem_one_automatic_source_per_playlist
BEFORE INSERT ON templatechannelitem
WHEN COALESCE(NEW.manual_locked, false) = false AND EXISTS (
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

-- Older writes used 0/1 for every variant. Convert each channel to a stable,
-- gapless failover order without changing its existing relative order.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY channel_id ORDER BY orderr, id) AS position
    FROM templatechannelitem
)
UPDATE templatechannelitem
SET orderr = (SELECT position FROM ranked WHERE ranked.id = templatechannelitem.id);

CREATE INDEX IF NOT EXISTS idx_templatechannelitem_channel_order
ON templatechannelitem(channel_id, orderr, id);

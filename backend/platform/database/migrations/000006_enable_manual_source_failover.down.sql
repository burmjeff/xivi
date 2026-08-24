-- +migrate Down

DROP INDEX IF EXISTS idx_templatechannelitem_channel_order;
DROP TRIGGER IF EXISTS templatechannelitem_one_automatic_source_per_playlist;

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

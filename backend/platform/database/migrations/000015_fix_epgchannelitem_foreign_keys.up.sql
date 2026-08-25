-- The original table referenced epg_channel/epg_programme, while the actual
-- legacy tables are named epgchannel/epgprogramme. SQLite resolves foreign-key
-- targets during deletes, so the typo prevented retention cleanup from pruning
-- old programmes even when epgchannelitem was empty.
DROP TRIGGER IF EXISTS delete_epg_channel;

CREATE TABLE epgchannelitem_fixed (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epg_channel_id INTEGER NOT NULL,
    epg_programme_id INTEGER NOT NULL,
    FOREIGN KEY (epg_channel_id) REFERENCES epgchannel(id) ON DELETE CASCADE,
    FOREIGN KEY (epg_programme_id) REFERENCES epgprogramme(id) ON DELETE CASCADE
);

INSERT INTO epgchannelitem_fixed (id, epg_channel_id, epg_programme_id)
SELECT item.id, item.epg_channel_id, item.epg_programme_id
FROM epgchannelitem AS item
JOIN epgchannel AS channel ON channel.id = item.epg_channel_id
JOIN epgprogramme AS programme ON programme.id = item.epg_programme_id;

DROP TABLE epgchannelitem;
ALTER TABLE epgchannelitem_fixed RENAME TO epgchannelitem;

CREATE TRIGGER delete_epg_channel
AFTER DELETE ON epgprogramme
FOR EACH ROW
BEGIN
    DELETE FROM epgchannel WHERE id NOT IN (SELECT epg_channel_id FROM epgchannelitem);
END;

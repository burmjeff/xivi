DROP TRIGGER IF EXISTS delete_epg_channel;

CREATE TABLE epgchannelitem_legacy (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epg_channel_id INTEGER NOT NULL,
    epg_programme_id INTEGER NOT NULL,
    FOREIGN KEY (epg_channel_id) REFERENCES epg_channel(id) ON DELETE CASCADE,
    FOREIGN KEY (epg_programme_id) REFERENCES epg_programme(id) ON DELETE CASCADE
);

INSERT INTO epgchannelitem_legacy (id, epg_channel_id, epg_programme_id)
SELECT id, epg_channel_id, epg_programme_id FROM epgchannelitem;

DROP TABLE epgchannelitem;
ALTER TABLE epgchannelitem_legacy RENAME TO epgchannelitem;

CREATE TRIGGER delete_epg_channel
AFTER DELETE ON epgprogramme
FOR EACH ROW
BEGIN
    DELETE FROM epgchannel WHERE id NOT IN (SELECT epg_channel_id FROM epgchannelitem);
END;

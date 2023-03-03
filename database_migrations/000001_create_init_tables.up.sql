-- +migrate Up

-- Create playlist table
CREATE TABLE playlist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    url VARCHAR (255) NOT NULL,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- Create playlistgroup table
CREATE TABLE playlistgroup (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) NOT NULL
);

-- Create playlistchannel table
CREATE TABLE playlistchannel (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tvgid VARCHAR (255) NULL,
    name VARCHAR (255) NOT NULL,
    tvg_logo VARCHAR (255) NULL,
    enabled BOOLEAN NOT NULL,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
);

-- Create channelurl table
CREATE TABLE channelurl (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url VARCHAR (255) NOT NULL,
    playlist_id INTEGER NOT NULL,
    playlist_channel_id INTEGER NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (playlist_id) REFERENCES playlist(id) ON DELETE CASCADE,
    FOREIGN KEY (playlist_channel_id) REFERENCES playlistchannel(id) ON DELETE CASCADE
);

-- Create playlist_group_item table
CREATE TABLE playlist_group_item (
    playlist_id INTEGER NOT NULL,
    group_id INTEGER NULL,
    PRIMARY KEY (playlist_id, group_id),
    FOREIGN KEY (playlist_id) REFERENCES playlist(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES playlistgroup(id) ON DELETE CASCADE
);

-- Create playlist_group_channel table
CREATE TABLE playlist_group_channel (
    playlist_id INTEGER NOT NULL,
    group_id INTEGER NULL,
    channel_id INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, group_id, channel_id),
    FOREIGN KEY (playlist_id, group_id) REFERENCES playlist_group_item(playlist_id, group_id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES playlistchannel(id) ON DELETE CASCADE
);

-- Create template table
CREATE TABLE template (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL
);

-- Create templategroup table
CREATE TABLE templategroup (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL
);

-- Create templatechannel table
CREATE TABLE templatechannel (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    tvgid VARCHAR (255) UNIQUE NULL,
    logo VARCHAR (255) NULL,
    uuid VARCHAR (255) UNIQUE NOT NULL
);

-- Create template_group_item table
CREATE TABLE template_group_item (
    template_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (template_id, group_id),
    FOREIGN KEY (template_id) REFERENCES template(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES templategroup(id) ON DELETE CASCADE
);

-- Create template_group_channel table
CREATE TABLE template_group_channel (
    group_id INTEGER NULL,
    channel_id INTEGER NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (group_id, channel_id),
    FOREIGN KEY (group_id) REFERENCES templategroup(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE
);

-- Create templatechannelitem table
CREATE TABLE templatechannelitem (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL,
    playlist_channel_id INTEGER NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE,
    FOREIGN KEY (playlist_channel_id) REFERENCES playlistchannel(id) ON DELETE CASCADE
    ON DELETE CASCADE
);

-- Create epg table
CREATE TABLE epg (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    url VARCHAR (255) NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0
);

-- Create epgchannel table
CREATE TABLE epgchannel (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channelid VARCHAR (255) NOT NULL,
    displayname VARCHAR (255) NOT NULL,
    "icon.src" VARCHAR (255)
);

-- Create epgprogramme table
CREATE TABLE epgprogramme (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    start DATETIME DEFAULT (datetime('now','localtime')) NOT NULL,
    stop DATETIME DEFAULT (datetime('now','localtime')) NOT NULL,
    channel VARCHAR (20) NOT NULL,
    "title.value" VARCHAR (255) NULL,
    "title.lang" VARCHAR (20) NULL,
    subtitle VARCHAR (255) NULL,
    desc VARCHAR (500) NULL,
    categories VARCHAR NULL,
    "icon.src" VARCHAR (255) NULL,
    directors VARCHAR NULL,
    presenters VARCHAR NULL,
    producers VARCHAR NULL,
    actors VARCHAR NULL,
    "episodenumber.system" VARCHAR (20) NULL,
    "episodenumber.value" VARCHAR (20) NULL,
    "rating.system" VARCHAR (20) NULL,
    "rating.value" VARCHAR (20) NULL,
    "video.quality" VARCHAR (20) NULL,
    date VARCHAR (4) NULL
);

-- Create epgchannelitem table
CREATE TABLE epgchannelitem (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epg_channel_id INTEGER NOT NULL,
    epg_programme_id INTEGER NOT NULL,
    FOREIGN KEY (epg_channel_id) REFERENCES epg_channel(id) ON DELETE CASCADE,
    FOREIGN KEY (epg_programme_id) REFERENCES epg_programme(id) ON DELETE CASCADE
);

-- Create templatechannelvectors table
CREATE TABLE templatechannelvectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    channel_id INTEGER NOT NULL,
    FOREIGN KEY (name) REFERENCES channelvectors(name) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE
);

-- Create channelvectors table
CREATE TABLE channelvectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    vector BLOB NOT NULL
);

-- Create channelfilters table
CREATE TABLE channelfilters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    oldname VARCHAR (255) UNIQUE NOT NULL,
    newname VARCHAR (255) NOT NULL
);

-- Add indexes
CREATE INDEX idx_playlist_channel ON playlistchannel (name);
CREATE INDEX idx_epg_epgprogramme ON epgprogramme (channel);
CREATE INDEX idx_template_tvgid ON templatechannel (tvgid);
CREATE INDEX idx_template_vector ON templatechannelvectors (name);
CREATE INDEX idx_playlist_vector ON channelvectors (name);
CREATE INDEX idx_channel_filters ON channelfilters (oldname);
CREATE INDEX idx_template_group_item_orderr ON template_group_item (orderr);
CREATE INDEX idx_template_group_channel_orderr ON template_group_channel (orderr);

-- Add triggers
CREATE TRIGGER template_group_item_order
AFTER INSERT ON template_group_item
FOR EACH ROW
BEGIN
  UPDATE template_group_item SET orderr = COALESCE((SELECT MAX(orderr) FROM template_group_item), 0) + 1
  WHERE ROWID = new.ROWID;
END;

CREATE TRIGGER template_group_channel_order
AFTER INSERT ON template_group_channel
FOR EACH ROW
BEGIN
  UPDATE template_group_channel SET orderr = COALESCE((SELECT MAX(orderr) FROM template_group_channel), 0) + 1
  WHERE ROWID = new.ROWID;
END;

CREATE TRIGGER increment_tmpl_channel_order
AFTER INSERT ON templatechannelitem
BEGIN
    UPDATE templatechannelitem
    SET orderr = (
        SELECT MAX(orderr) + 1
        FROM templatechannelitem
    )
    WHERE id = new.id;
END;

CREATE TRIGGER increment_channel_url_order
AFTER INSERT ON channelurl
BEGIN
    UPDATE channelurl
    SET orderr = (
        SELECT MAX(orderr) + 1
        FROM channelurl
    )
    WHERE id = new.id;
END;

CREATE TRIGGER increment_epg_order
AFTER INSERT ON epg
BEGIN
    UPDATE epg
    SET orderr = (
        SELECT MAX(orderr) + 1
        FROM epg
    )
    WHERE id = new.id;
END;

CREATE TRIGGER delete_playlistchannel_cascade
AFTER DELETE ON playlistchannel
FOR EACH ROW
BEGIN
    DELETE FROM playlist_group_item WHERE group_id NOT IN (SELECT group_id FROM playlist_group_channel);
    DELETE FROM playlistgroup WHERE id NOT IN (SELECT group_id FROM playlist_group_item);
END;

CREATE TRIGGER delete_channelurl_cascade
AFTER DELETE ON channelurl
FOR EACH ROW
BEGIN
    DELETE FROM playlistchannel WHERE id NOT IN (SELECT playlist_channel_id FROM channelurl);
    DELETE FROM playlist_group_item WHERE group_id NOT IN (SELECT group_id FROM playlist_group_channel);
    DELETE FROM playlistgroup WHERE id NOT IN (SELECT group_id FROM playlist_group_item);
END;

CREATE TRIGGER delete_playlist_cascade
AFTER DELETE ON playlist
FOR EACH ROW
BEGIN
    DELETE FROM channelurl WHERE playlist_id = old.id;
    DELETE FROM playlistchannel WHERE id NOT IN (SELECT playlist_channel_id FROM channelurl);
END;

CREATE TRIGGER delete_epg_channel_cascade
AFTER DELETE ON epgprogramme
FOR EACH ROW
BEGIN
    DELETE FROM epgchannelitem WHERE epg_programme_id = old.id;
    DELETE FROM epgchannel WHERE id NOT IN (SELECT epg_channel_id FROM epgchannelitem);
END;

CREATE TRIGGER delete_template_channel_cascade
AFTER DELETE ON templatechannel
FOR EACH ROW
BEGIN
    DELETE FROM templatechannelvectors WHERE channel_id = old.id;
END;
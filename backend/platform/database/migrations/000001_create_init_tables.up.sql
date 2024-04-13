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
    name VARCHAR (255) NOT NULL,
    playlist_id INTEGER NOT NULL,
    enabled BOOLEAN DEFAULT true NOT NULL,
    FOREIGN KEY (playlist_id) REFERENCES playlist(id) ON DELETE CASCADE
);

-- Create playlistchannel table
CREATE TABLE playlistchannel (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tvg_id VARCHAR (255) NULL,
    tvg_name VARCHAR (255) NULL,
    tvg_logo VARCHAR (255) NULL,
    title VARCHAR (255) NOT NULL,
    group_id INTEGER NOT NULL,
    enabled BOOLEAN DEFAULT true NOT NULL,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (group_id) REFERENCES playlistgroup(id) ON DELETE CASCADE
);

-- Create channelurl table
CREATE TABLE channelurl (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url VARCHAR (255) NOT NULL,
    channel_id INTEGER NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime')),
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
    name VARCHAR (255) UNIQUE NOT NULL,
    dynamic BOOLEAN,
    dynamicgroup INTEGER NULL,
    FOREIGN KEY (dynamicgroup) REFERENCES playlistgroup(id) ON DELETE SET NULL
);

-- Create templatechannel table
CREATE TABLE templatechannel (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) NOT NULL,
    tvgid VARCHAR (255) NULL,
    logoid INTEGER DEFAULT 0 NULL,
    uuid VARCHAR (255) UNIQUE NOT NULL,
    FOREIGN KEY (logoid) REFERENCES logo(id) ON DELETE SET DEFAULT
);

-- Create logo table
CREATE TABLE logo (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL
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
    group_id INTEGER NOT NULL,
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
    FOREIGN KEY (playlist_channel_id) REFERENCES playlistchannel(id) ON DELETE CASCADE,
    UNIQUE(channel_id, playlist_channel_id) ON CONFLICT IGNORE
);

-- Create epg table
CREATE TABLE epg (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    url VARCHAR (255) NOT NULL,
    orderr INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime'))
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
    desc VARCHAR (2000) NULL,
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
    date VARCHAR (12) NULL
);

-- Create epgchannelitem table
CREATE TABLE epgchannelitem (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    epg_channel_id INTEGER NOT NULL,
    epg_programme_id INTEGER NOT NULL,
    FOREIGN KEY (epg_channel_id) REFERENCES epg_channel(id) ON DELETE CASCADE,
    FOREIGN KEY (epg_programme_id) REFERENCES epg_programme(id) ON DELETE CASCADE
);

-- Create channelvectors table
CREATE TABLE channelvectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    vector BLOB NOT NULL
);

-- Create templatechannelvectors table
CREATE TABLE templatechannelvectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL UNIQUE,
    vector_id INTEGER NOT NULL,
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE,
    FOREIGN KEY (vector_id) REFERENCES channelvectors(id) ON DELETE CASCADE
);

-- Create playlistchannelvectors table
CREATE TABLE playlistchannelvectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL UNIQUE,
    vector_id INTEGER NOT NULL,
    FOREIGN KEY (channel_id) REFERENCES playlistchannel(id) ON DELETE CASCADE,
    FOREIGN KEY (vector_id) REFERENCES channelvectors(id) ON DELETE CASCADE
);

-- Create regexfilters table
CREATE TABLE regexfilters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR (255) UNIQUE NOT NULL,
    regex VARCHAR (255) NOT NULL
);

-- Create playlist_group_filters table
CREATE TABLE groupfilters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    filter_id INTEGER NOT NULL,
    type BOOLEAN NOT NULL,
    FOREIGN KEY (group_id) REFERENCES playlistgroup(id) ON DELETE CASCADE,
    FOREIGN KEY (filter_id) REFERENCES regexfilters(id) ON DELETE CASCADE
);

-- Add indexes
CREATE INDEX idx_playlist_channel ON playlistchannel (title);
CREATE INDEX idx_epg_epgprogramme ON epgprogramme (channel);
CREATE INDEX idx_template_tvgid ON templatechannel (tvgid);
CREATE INDEX idx_channel_vector ON channelvectors (name);
CREATE INDEX idx_template_vector ON templatechannelvectors (channel_id);
CREATE INDEX idx_playlist_vector ON playlistchannelvectors (channel_id);
CREATE INDEX idx_regex_filters ON regexfilters (id);
CREATE INDEX idx_group_filters ON groupfilters (group_id);
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

CREATE TRIGGER before_delete_playlistgroup
BEFORE DELETE ON playlistgroup
FOR EACH ROW
BEGIN
    UPDATE templategroup SET dynamic = false WHERE dynamicgroup = old.id;
END;

CREATE TRIGGER delete_playlistchannel
AFTER DELETE ON playlistchannel
FOR EACH ROW
BEGIN
    DELETE FROM playlistgroup WHERE id NOT IN (SELECT group_id FROM playlistchannel) AND enabled = true;
END;

CREATE TRIGGER delete_channelurl
AFTER DELETE ON channelurl
FOR EACH ROW
BEGIN
    DELETE FROM playlistchannel WHERE id NOT IN (SELECT channel_id FROM channelurl);
END;

CREATE TRIGGER delete_epg_channel
AFTER DELETE ON epgprogramme
FOR EACH ROW
BEGIN
    DELETE FROM epgchannel WHERE id NOT IN (SELECT epg_channel_id FROM epgchannelitem);
END;

CREATE TRIGGER delete_template_group
AFTER DELETE ON templategroup
FOR EACH ROW
BEGIN
    DELETE FROM templatechannel WHERE id NOT IN (SELECT channel_id FROM template_group_channel);
END;

CREATE TRIGGER delete_logo
BEFORE DELETE ON logo
FOR EACH ROW
BEGIN
    UPDATE templatechannel SET logoid = 0 WHERE logoid = old.id;
END;

-- Initial Values
INSERT INTO logo VALUES (0, "xivi_channel");
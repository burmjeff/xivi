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
    group_id INTEGER NULL,
    enabled BOOLEAN NOT NULL,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (group_id) REFERENCES playlistgroup(id)
);

-- Create channelurl table
CREATE TABLE channelurl (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url VARCHAR (255) NOT NULL,
    playlist_id INTEGER NOT NULL,
    playlist_channel_id INTEGER NOT NULL,
    orderr INTEGER UNIQUE NOT NULL,
    created_at DATETIME DEFAULT (datetime('now','localtime')),
    updated_at DATETIME DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (playlist_id) REFERENCES playlist(id),
    FOREIGN KEY (playlist_channel_id) REFERENCES playlistchannel(id)
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
    tvgid VARCHAR (255) NULL,
    logo VARCHAR (255) NULL,
    uuid VARCHAR (255) UNIQUE NOT NULL
);

-- Create templateitem table
CREATE TABLE templateitem (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    orderr INTEGER UNIQUE NOT NULL,
    FOREIGN KEY (template_id) REFERENCES template(id),
    FOREIGN KEY (group_id) REFERENCES templategroup(id)
    ON DELETE CASCADE
);

-- Create templategroupitem table
CREATE TABLE templategroupitem (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    channel_id INTEGER NOT NULL,
    orderr INTEGER UNIQUE NOT NULL,
    FOREIGN KEY (group_id) REFERENCES templategroup(id),
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id)
    ON DELETE CASCADE
);

-- Create templatechannelitem table
CREATE TABLE templatechannelitem (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL,
    playlist_channel_id INTEGER NOT NULL,
    orderr INTEGER UNIQUE NOT NULL,
    FOREIGN KEY (channel_id) REFERENCES templatechannel(id),
    FOREIGN KEY (playlist_channel_id) REFERENCES playlistchannel(id)
    ON DELETE CASCADE
);

-- Add indexes
CREATE INDEX idx_playlist_channel ON playlistchannel (name);

-- Add triggers
CREATE TRIGGER increment_tmpl_order
AFTER INSERT ON templateitem
BEGIN
    UPDATE templateitem
    SET orderr = (
        SELECT MAX(orderr) + 1
        FROM templateitem
    )
    WHERE id = new.id;
END;

CREATE TRIGGER increment_tmpl_group_order
AFTER INSERT ON templategroupitem
BEGIN
    UPDATE templategroupitem
    SET orderr = (
        SELECT MAX(orderr) + 1
        FROM templategroupitem
    )
    WHERE id = new.id;
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

CREATE TRIGGER delete_playlist_cascade
AFTER DELETE ON playlist
FOR EACH ROW
BEGIN
    DELETE FROM channelurl WHERE playlist_id = old.id;
    DELETE FROM playlistchannel WHERE id NOT IN (SELECT playlist_channel_id FROM channelurl);
END;

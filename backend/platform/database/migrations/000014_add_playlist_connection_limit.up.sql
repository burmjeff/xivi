ALTER TABLE playlist
    ADD COLUMN connection_limit INTEGER NOT NULL DEFAULT 1
    CHECK (connection_limit >= 1 AND connection_limit <= 100);

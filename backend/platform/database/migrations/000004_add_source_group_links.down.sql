-- +migrate Down

DROP INDEX IF EXISTS idx_source_group_member_source_channel;
DROP INDEX IF EXISTS idx_source_group_link_source_group;
DROP INDEX IF EXISTS idx_source_group_link_playlist;
DROP TABLE IF EXISTS lineup_group_source_member;
DROP TABLE IF EXISTS lineup_group_source_link;

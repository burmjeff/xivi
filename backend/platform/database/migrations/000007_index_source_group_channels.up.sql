-- +migrate Up

-- Source Browser group pages and lineup-aware usage filters both start from a
-- provider group. Avoid scanning the entire source catalog for each page.
CREATE INDEX IF NOT EXISTS idx_playlistchannel_group
ON playlistchannel(group_id);

-- Watch pagination traverses a lineup in its saved group/channel order. These
-- covering prefixes avoid scanning order indexes for unrelated lineups and
-- avoid re-sorting every channel inside a group.
CREATE INDEX IF NOT EXISTS idx_template_group_item_template_order
    ON template_group_item (template_id, orderr, group_id);

CREATE INDEX IF NOT EXISTS idx_template_group_channel_group_order
    ON template_group_channel (group_id, orderr, channel_id);

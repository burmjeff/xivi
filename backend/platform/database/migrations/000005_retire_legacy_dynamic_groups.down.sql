-- +migrate Down

-- Restore the compatibility flags only for groups with a durable source link.
UPDATE templategroup
SET dynamic = true,
    dynamicgroup = (
        SELECT source_group_id
        FROM lineup_group_source_link
        WHERE lineup_group_source_link.group_id = templategroup.id
    )
WHERE EXISTS (
    SELECT 1
    FROM lineup_group_source_link
    WHERE lineup_group_source_link.group_id = templategroup.id
);

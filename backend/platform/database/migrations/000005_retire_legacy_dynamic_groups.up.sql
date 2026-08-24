-- +migrate Up

-- Source subscriptions were migrated into lineup_group_source_link in 000004.
-- Clear the former control flags so no runtime can accidentally take the
-- destructive legacy dynamic-group path again.
UPDATE templategroup
SET dynamic = false,
    dynamicgroup = NULL
WHERE COALESCE(dynamic, false) = true OR dynamicgroup IS NOT NULL;

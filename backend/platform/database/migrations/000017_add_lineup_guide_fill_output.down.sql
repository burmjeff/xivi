-- +migrate Down

ALTER TABLE template DROP COLUMN fill_missing_guide_slots;

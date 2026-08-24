-- +migrate Down

ALTER TABLE template DROP COLUMN virtual_tuner_enabled;

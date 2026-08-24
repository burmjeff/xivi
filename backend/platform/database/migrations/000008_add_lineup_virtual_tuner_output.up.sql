-- +migrate Up

-- Virtual tuner output is opt-in for both existing and newly created lineups.
ALTER TABLE template ADD COLUMN virtual_tuner_enabled BOOLEAN NOT NULL DEFAULT false;

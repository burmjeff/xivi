-- +migrate Up

-- Preserve the historical compatibility behavior while making it explicit
-- and independently configurable for every published lineup.
ALTER TABLE template ADD COLUMN fill_missing_guide_slots BOOLEAN NOT NULL DEFAULT true;

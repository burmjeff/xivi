-- +migrate Down

DROP TRIGGER IF EXISTS templatechannelitem_one_source_per_playlist;
DROP INDEX IF EXISTS idx_channelmatchrejection_lookup;
DROP TABLE IF EXISTS channelmatchrejection;
DROP INDEX IF EXISTS idx_templatechannelitem_match_method;
ALTER TABLE templatechannelitem DROP COLUMN manual_locked;
ALTER TABLE templatechannelitem DROP COLUMN matcher_version;
ALTER TABLE templatechannelitem DROP COLUMN runner_up_score;
ALTER TABLE templatechannelitem DROP COLUMN match_score;
ALTER TABLE templatechannelitem DROP COLUMN match_method;

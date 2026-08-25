ALTER TABLE stream_session_history ADD COLUMN channel_id INTEGER NULL;
ALTER TABLE stream_session_history ADD COLUMN channel_name TEXT NOT NULL DEFAULT '';

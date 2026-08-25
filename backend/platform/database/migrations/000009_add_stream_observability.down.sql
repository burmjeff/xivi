DROP INDEX IF EXISTS idx_stream_event_created;
DROP INDEX IF EXISTS idx_stream_event_incident_created;
DROP INDEX IF EXISTS idx_stream_connection_history_incident;
DROP INDEX IF EXISTS idx_stream_session_history_stream_started;
DROP INDEX IF EXISTS idx_stream_session_history_started;
DROP TABLE IF EXISTS stream_event;
DROP TABLE IF EXISTS stream_connection_history;
DROP TABLE IF EXISTS stream_session_history;

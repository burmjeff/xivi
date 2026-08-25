CREATE TABLE stream_session_history (
    incident_id TEXT PRIMARY KEY,
    stream_id TEXT NOT NULL,
    state TEXT NOT NULL,
    source_count INTEGER NOT NULL DEFAULT 0,
    reconnects INTEGER NOT NULL DEFAULT 0,
    bytes_ingested INTEGER NOT NULL DEFAULT 0,
    bytes_delivered INTEGER NOT NULL DEFAULT 0,
    slow_client_drops INTEGER NOT NULL DEFAULT 0,
    started_at DATETIME NOT NULL,
    first_media_at DATETIME NULL,
    ended_at DATETIME NULL,
    end_reason TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE stream_connection_history (
    id TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL,
    stream_id TEXT NOT NULL,
    protocol TEXT NOT NULL,
    remote_ip TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    started_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    ended_at DATETIME NULL,
    bytes_delivered INTEGER NOT NULL DEFAULT 0,
    end_reason TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (incident_id) REFERENCES stream_session_history(incident_id) ON DELETE CASCADE
);

CREATE TABLE stream_event (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id TEXT NOT NULL,
    connection_id TEXT NULL,
    stream_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    code TEXT NOT NULL,
    message TEXT NOT NULL,
    source_position INTEGER NOT NULL DEFAULT 0,
    details TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    FOREIGN KEY (incident_id) REFERENCES stream_session_history(incident_id) ON DELETE CASCADE
);

CREATE INDEX idx_stream_session_history_started
    ON stream_session_history(started_at DESC);
CREATE INDEX idx_stream_session_history_stream_started
    ON stream_session_history(stream_id, started_at DESC);
CREATE INDEX idx_stream_connection_history_incident
    ON stream_connection_history(incident_id, started_at DESC);
CREATE INDEX idx_stream_event_incident_created
    ON stream_event(incident_id, created_at DESC);
CREATE INDEX idx_stream_event_created
    ON stream_event(created_at DESC);

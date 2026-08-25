-- Stored timestamps can include different UTC offsets. Index the normalized
-- instant used by retention queries instead of comparing their raw text.
DROP INDEX IF EXISTS idx_operation_job_finished_terminal;
DROP INDEX IF EXISTS idx_stream_session_history_ended;
DROP INDEX IF EXISTS idx_epgprogramme_stop;

CREATE INDEX IF NOT EXISTS idx_operation_job_finished_utc
    ON operation_job(datetime(finished_at), id)
    WHERE status IN ('succeeded', 'failed', 'cancelled') AND finished_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_operation_job_updated_utc
    ON operation_job(datetime(updated_at), id)
    WHERE status IN ('succeeded', 'failed', 'cancelled') AND finished_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_stream_session_history_ended_utc
    ON stream_session_history(datetime(ended_at), incident_id)
    WHERE ended_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_epgprogramme_stop_utc
    ON epgprogramme(datetime(stop), id);

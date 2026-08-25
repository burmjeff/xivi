CREATE INDEX IF NOT EXISTS idx_templatechannelvectors_vector
    ON templatechannelvectors(vector_id);

CREATE INDEX IF NOT EXISTS idx_playlistchannelvectors_vector
    ON playlistchannelvectors(vector_id);

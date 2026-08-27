-- +migrate Up

-- Acknowledgements are scoped to the exact set of channels that shared an ID.
-- If membership changes, the stored signature no longer matches and the issue
-- automatically returns to Needs Review.
CREATE TABLE lineup_tvgid_review_ack (
    lineup_id INTEGER NOT NULL,
    tvg_id_norm VARCHAR(255) NOT NULL,
    channel_signature TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (lineup_id, tvg_id_norm),
    FOREIGN KEY (lineup_id) REFERENCES template(id) ON DELETE CASCADE
);


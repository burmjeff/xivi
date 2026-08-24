-- +migrate Up

CREATE TABLE operation_job (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind VARCHAR(32) NOT NULL,
    resource VARCHAR(32) NOT NULL,
    resource_id INTEGER NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    progress INTEGER NOT NULL DEFAULT 0 CHECK(progress >= 0 AND progress <= 100),
    message VARCHAR(512) NOT NULL DEFAULT '',
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME NULL
);

CREATE INDEX idx_operation_job_status_created ON operation_job(status, created_at DESC);
CREATE INDEX idx_operation_job_resource ON operation_job(resource, resource_id, created_at DESC);

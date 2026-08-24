-- +migrate Down

DROP INDEX IF EXISTS idx_operation_job_resource;
DROP INDEX IF EXISTS idx_operation_job_status_created;
DROP TABLE IF EXISTS operation_job;

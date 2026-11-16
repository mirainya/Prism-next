-- Reason: persist the import mode and idempotency key before any external write.
-- Requirement: a restart or unavailable async endpoint must not trigger a second upload.
-- Scope: one immutable intent per Attempt/result ordinal; no source URL or credential.
-- Deployment: apply before starting the matching Prism build; existing imports are unchanged.
CREATE TABLE IF NOT EXISTS gw_result_import_jobs (
    attempt_id BIGINT UNSIGNED NOT NULL,
    result_ordinal INT UNSIGNED NOT NULL,
    import_mode VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    task_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    storage_path VARCHAR(1024) NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    source_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    storage_binding_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    legacy_started BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (attempt_id, result_ordinal),
    UNIQUE KEY uk_gw_result_import_task (task_key),
    CONSTRAINT fk_gw_result_import_attempt FOREIGN KEY (attempt_id) REFERENCES gw_api_call_attempts(id),
    CONSTRAINT ck_gw_result_import_mode CHECK (import_mode IN ('detect', 'async_v1', 'legacy_url'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Count actual delivery failures independently from lease claims/polling.
CREATE TABLE IF NOT EXISTS gw_delivery_recovery_failures (
    outbox_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    failure_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
    CONSTRAINT fk_gw_delivery_failure_outbox FOREIGN KEY (outbox_id) REFERENCES gw_async_outbox(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

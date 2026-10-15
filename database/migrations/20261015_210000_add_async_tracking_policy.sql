-- Persist per-channel polling policy and per-execution snapshots.
-- Scope: new tables only; existing executions initialize on their next query.
CREATE TABLE IF NOT EXISTS gw_async_poll_policies (
    channel_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    transient_window_seconds INT UNSIGNED NOT NULL DEFAULT 900,
    reconciliation_window_seconds INT UNSIGNED NOT NULL DEFAULT 86400,
    max_retry_seconds INT UNSIGNED NOT NULL DEFAULT 300,
    reconciliation_interval_seconds INT UNSIGNED NOT NULL DEFAULT 300,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT ck_async_poll_policy_windows CHECK (transient_window_seconds BETWEEN 1 AND 86400 AND reconciliation_window_seconds BETWEEN transient_window_seconds AND 2592000),
    CONSTRAINT ck_async_poll_policy_intervals CHECK (max_retry_seconds BETWEEN 1 AND 300 AND reconciliation_interval_seconds BETWEEN 1 AND 3600),
    CONSTRAINT fk_async_poll_policy_channel FOREIGN KEY (channel_id) REFERENCES gateway_channels(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS gw_async_tracking (
    async_execution_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    transient_window_seconds INT UNSIGNED NOT NULL,
    reconciliation_window_seconds INT UNSIGNED NOT NULL,
    max_retry_seconds INT UNSIGNED NOT NULL,
    reconciliation_interval_seconds INT UNSIGNED NOT NULL,
    mode VARCHAR(24) NOT NULL DEFAULT 'active',
    first_error_at DATETIME(3) NULL,
    reconciliation_until DATETIME(3) NULL,
    last_error_code VARCHAR(128) NOT NULL DEFAULT '',
    operator_id BIGINT UNSIGNED NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT ck_async_tracking_mode CHECK (mode IN ('active','reconciling','paused','manual')),
    CONSTRAINT fk_async_tracking_execution FOREIGN KEY (async_execution_id) REFERENCES gw_async_executions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

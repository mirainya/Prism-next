-- Clean application schema baseline; no provider onboarding data.
-- Generated from a fully migrated isolated MySQL 8 database.

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!50503 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `account_model_states` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `account_id` bigint unsigned NOT NULL COMMENT '账号ID',
  `model_code` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '模型标识',
  `disabled_until` datetime(3) NOT NULL COMMENT '熔断到期时间',
  `reason` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT '' COMMENT '熔断原因',
  `status_code` bigint DEFAULT '0' COMMENT '触发的HTTP状态码',
  `fail_count` bigint DEFAULT '1' COMMENT '累计触发次数',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_account_model` (`account_id`,`model_code`),
  KEY `idx_account_model_states_disabled_until` (`disabled_until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `account_models` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `account_id` bigint unsigned NOT NULL COMMENT '账号(key)ID',
  `model_code` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '模型标识',
  `vendor_model` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT '' COMMENT '上游模型名(空=用model_code)',
  `priority` bigint DEFAULT '0' COMMENT '优先级(降序)',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `price_mode` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'token' COMMENT '计价模式',
  `input_price` decimal(12,8) DEFAULT '0.00000000' COMMENT '输入价格',
  `output_price` decimal(12,8) DEFAULT '0.00000000' COMMENT '输出价格',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_am_account_model` (`account_id`,`model_code`),
  KEY `idx_account_models_priority` (`priority`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `ai_files` (
  `id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `filename` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `purpose` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL,
  `bytes` bigint NOT NULL,
  `mime_type` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `content` longblob NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'processed',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ai_files_user` (`user_id`),
  KEY `idx_ai_files_token` (`token_id`),
  KEY `idx_ai_files_purpose` (`purpose`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `ai_response_idempotency_cache` (
  `token_id` bigint unsigned NOT NULL,
  `idempotency_key` varbinary(128) NOT NULL,
  `request_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `owner` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `response_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `response_json` json DEFAULT NULL,
  `expires_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`token_id`,`idempotency_key`),
  KEY `idx_ai_response_idempotency_status` (`status`),
  KEY `idx_ai_response_idempotency_expires` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `ai_responses` (
  `id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `model` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `background` tinyint(1) DEFAULT '0',
  `store` tinyint(1) DEFAULT '1',
  `previous_response_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider_response_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `channel_id` bigint unsigned DEFAULT '0',
  `key_id` bigint unsigned DEFAULT '0',
  `upstream_transport` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `request_log_id` bigint unsigned DEFAULT '0',
  `request_json` json DEFAULT NULL,
  `request_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_owner` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `execution_attempt` bigint NOT NULL DEFAULT '0',
  `result_ready_at` datetime(3) DEFAULT NULL,
  `input_items` json DEFAULT NULL,
  `output_items` json DEFAULT NULL,
  `response_json` json DEFAULT NULL,
  `usage_json` json DEFAULT NULL,
  `metadata` json DEFAULT NULL,
  `error_json` json DEFAULT NULL,
  `idempotency_key` varbinary(512) NOT NULL DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  `completed_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_ai_responses_idempotency` (`token_id`,`idempotency_key`),
  KEY `idx_ai_responses_user` (`user_id`),
  KEY `idx_ai_responses_token` (`token_id`),
  KEY `idx_ai_responses_call` (`call_id`),
  KEY `idx_ai_responses_model` (`model`),
  KEY `idx_ai_responses_status` (`status`),
  KEY `idx_ai_responses_previous` (`previous_response_id`),
  KEY `idx_ai_responses_request_log` (`request_log_id`),
  KEY `idx_ai_responses_lease` (`lease_expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `api_access_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `request_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_id` bigint unsigned NOT NULL DEFAULT '0',
  `token_id` bigint unsigned NOT NULL DEFAULT '0',
  `actor_type` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'anonymous',
  `method` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `path` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL,
  `route` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `query` text COLLATE utf8mb4_unicode_ci,
  `status_code` bigint NOT NULL DEFAULT '0',
  `duration_ms` bigint NOT NULL DEFAULT '0',
  `ip` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_agent` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_message` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_api_access_logs_request_id` (`request_id`),
  KEY `idx_api_access_logs_call_id` (`call_id`),
  KEY `idx_api_access_user_created` (`user_id`,`created_at`),
  KEY `idx_api_access_token_created` (`token_id`,`created_at`),
  KEY `idx_api_access_logs_actor_type` (`actor_type`),
  KEY `idx_api_access_logs_method` (`method`),
  KEY `idx_api_access_logs_route` (`route`),
  KEY `idx_api_access_logs_status_code` (`status_code`),
  KEY `idx_api_access_logs_error_code` (`error_code`),
  KEY `idx_api_access_logs_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `api_call_attempts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `attempt_no` bigint NOT NULL,
  `route_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'gateway_v2',
  `stage` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `ability_id` bigint unsigned NOT NULL DEFAULT '0',
  `channel_id` bigint unsigned NOT NULL DEFAULT '0',
  `key_id` bigint unsigned NOT NULL DEFAULT '0',
  `endpoint_id` bigint unsigned NOT NULL DEFAULT '0',
  `account_id` bigint unsigned NOT NULL DEFAULT '0',
  `protocol` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `vendor_model` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `transport` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `request_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'started',
  `http_status` bigint NOT NULL DEFAULT '0',
  `error_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_message` text COLLATE utf8mb4_unicode_ci,
  `error_retryable` tinyint(1) NOT NULL DEFAULT '0',
  `input_tokens` bigint NOT NULL DEFAULT '0',
  `output_tokens` bigint NOT NULL DEFAULT '0',
  `total_tokens` bigint NOT NULL DEFAULT '0',
  `cached_input_tokens` bigint NOT NULL DEFAULT '0',
  `reasoning_output_tokens` bigint NOT NULL DEFAULT '0',
  `usage_json` json DEFAULT NULL,
  `duration_ms` bigint NOT NULL DEFAULT '0',
  `ttft_ms` bigint NOT NULL DEFAULT '0',
  `provider_response_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `started_at` datetime(3) NOT NULL,
  `first_byte_at` datetime(3) DEFAULT NULL,
  `completed_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_api_call_attempt_no` (`call_id`,`attempt_no`),
  KEY `idx_api_call_attempts_route_kind` (`route_kind`),
  KEY `idx_api_call_attempts_stage` (`stage`),
  KEY `idx_api_call_attempts_ability_id` (`ability_id`),
  KEY `idx_api_call_attempts_channel_id` (`channel_id`),
  KEY `idx_api_call_attempts_key_id` (`key_id`),
  KEY `idx_api_call_attempts_endpoint_id` (`endpoint_id`),
  KEY `idx_api_call_attempts_account_id` (`account_id`),
  KEY `idx_api_call_attempts_transport` (`transport`),
  KEY `idx_api_call_attempts_status` (`status`),
  KEY `idx_api_call_attempts_error_code` (`error_code`),
  KEY `idx_api_call_attempts_provider_response_id` (`provider_response_id`),
  KEY `idx_api_call_attempts_started_at` (`started_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `api_call_payloads` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `attempt_id` bigint unsigned NOT NULL DEFAULT '0',
  `kind` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `content_type` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'application/json',
  `data` longblob,
  `encrypted` tinyint(1) NOT NULL DEFAULT '0',
  `truncated` tinyint(1) NOT NULL DEFAULT '0',
  `original_bytes` bigint NOT NULL DEFAULT '0',
  `expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_api_call_payload_kind` (`call_id`,`kind`),
  KEY `idx_api_call_payloads_attempt_id` (`attempt_id`),
  KEY `idx_api_call_payloads_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `api_calls` (
  `id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `endpoint` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `operation` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `model` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'received',
  `is_stream` tinyint(1) NOT NULL DEFAULT '0',
  `background` tinyint(1) NOT NULL DEFAULT '0',
  `store` tinyint(1) NOT NULL DEFAULT '0',
  `retain_payload` tinyint(1) NOT NULL DEFAULT '0',
  `payload_expires_at` datetime(3) DEFAULT NULL,
  `resource_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `resource_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `conversation_id` bigint unsigned NOT NULL DEFAULT '0',
  `project_conversation` tinyint(1) NOT NULL DEFAULT '0',
  `final_attempt_id` bigint unsigned NOT NULL DEFAULT '0',
  `attempt_count` bigint NOT NULL DEFAULT '0',
  `input_tokens` bigint NOT NULL DEFAULT '0',
  `output_tokens` bigint NOT NULL DEFAULT '0',
  `total_tokens` bigint NOT NULL DEFAULT '0',
  `cached_input_tokens` bigint NOT NULL DEFAULT '0',
  `reasoning_output_tokens` bigint NOT NULL DEFAULT '0',
  `usage_json` json DEFAULT NULL,
  `reserved_amount` decimal(20,8) NOT NULL DEFAULT '0.00000000',
  `final_cost` decimal(20,8) NOT NULL DEFAULT '0.00000000',
  `refunded_amount` decimal(20,8) NOT NULL DEFAULT '0.00000000',
  `http_status` bigint NOT NULL DEFAULT '0',
  `error_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_message` text COLLATE utf8mb4_unicode_ci,
  `error_param` json DEFAULT NULL,
  `error_retryable` tinyint(1) NOT NULL DEFAULT '0',
  `started_at` datetime(3) NOT NULL,
  `first_byte_at` datetime(3) DEFAULT NULL,
  `completed_at` datetime(3) DEFAULT NULL,
  `duration_ms` bigint NOT NULL DEFAULT '0',
  `ttft_ms` bigint NOT NULL DEFAULT '0',
  `client_disconnected` tinyint(1) NOT NULL DEFAULT '0',
  `lease_owner` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_api_calls_request_id` (`request_id`),
  KEY `idx_api_calls_user_created` (`user_id`,`created_at`),
  KEY `idx_api_calls_token_created` (`token_id`,`created_at`),
  KEY `idx_api_calls_endpoint` (`endpoint`),
  KEY `idx_api_calls_operation` (`operation`),
  KEY `idx_api_calls_model` (`model`),
  KEY `idx_api_calls_status_created` (`status`,`created_at`),
  KEY `idx_api_calls_payload_expires_at` (`payload_expires_at`),
  KEY `idx_api_calls_resource` (`resource_type`,`resource_id`),
  KEY `idx_api_calls_conversation_id` (`conversation_id`),
  KEY `idx_api_calls_final_attempt_id` (`final_attempt_id`),
  KEY `idx_api_calls_error_code` (`error_code`),
  KEY `idx_api_calls_started_at` (`started_at`),
  KEY `idx_api_calls_lease_expires_at` (`lease_expires_at`),
  KEY `idx_api_calls_created_at` (`created_at`),
  KEY `idx_api_calls_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `audit_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `request_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `actor_type` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'anonymous',
  `actor_user_id` bigint unsigned NOT NULL DEFAULT '0',
  `actor_token_id` bigint unsigned NOT NULL DEFAULT '0',
  `action` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `resource_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `resource_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `outcome` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `http_status` bigint NOT NULL DEFAULT '0',
  `ip` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `metadata` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_audit_events_request_id` (`request_id`),
  KEY `idx_audit_events_actor_type` (`actor_type`),
  KEY `idx_audit_actor_created` (`actor_user_id`,`created_at`),
  KEY `idx_audit_events_actor_token_id` (`actor_token_id`),
  KEY `idx_audit_events_action` (`action`),
  KEY `idx_audit_events_resource_type` (`resource_type`),
  KEY `idx_audit_events_resource_id` (`resource_id`),
  KEY `idx_audit_events_outcome` (`outcome`),
  KEY `idx_audit_events_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `balance_entries` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `entry_key` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_key` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `account_type` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `account_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL DEFAULT '0',
  `token_id` bigint unsigned NOT NULL DEFAULT '0',
  `direction` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `category` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(20,8) NOT NULL,
  `balance_before` decimal(20,8) NOT NULL,
  `balance_after` decimal(20,8) NOT NULL,
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `attempt_id` bigint unsigned NOT NULL DEFAULT '0',
  `actor_user_id` bigint unsigned NOT NULL DEFAULT '0',
  `metadata` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_balance_entries_entry_key` (`entry_key`),
  KEY `idx_balance_entries_source_key` (`source_key`),
  KEY `idx_balance_account_created` (`account_type`,`account_id`,`created_at`),
  KEY `idx_balance_user_created` (`user_id`,`created_at`),
  KEY `idx_balance_token_created` (`token_id`,`created_at`),
  KEY `idx_balance_entries_direction` (`direction`),
  KEY `idx_balance_entries_category` (`category`),
  KEY `idx_balance_entries_call_id` (`call_id`),
  KEY `idx_balance_entries_attempt_id` (`attempt_id`),
  KEY `idx_balance_entries_actor_user_id` (`actor_user_id`),
  KEY `idx_balance_entries_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_account_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `billing_account_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_account_state_events_version` (`billing_account_id`,`state_version`),
  CONSTRAINT `fk_billing_account_state_events_account` FOREIGN KEY (`billing_account_id`) REFERENCES `billing_accounts` (`id`),
  CONSTRAINT `ck_billing_account_state_events_state` CHECK ((`new_state` in (_utf8mb4'open',_utf8mb4'frozen',_utf8mb4'closed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_accounts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `system_state_id` tinyint unsigned NOT NULL DEFAULT '1',
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `posted_balance` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `held_amount` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `credit_limit` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_accounts_user` (`user_id`),
  UNIQUE KEY `uq_billing_accounts_id_currency` (`id`,`currency_code`,`currency_version`),
  KEY `fk_billing_accounts_system_currency` (`system_state_id`,`currency_code`,`currency_version`),
  CONSTRAINT `fk_billing_accounts_system_currency` FOREIGN KEY (`system_state_id`, `currency_code`, `currency_version`) REFERENCES `billing_system_state` (`id`, `currency_code`, `currency_version`),
  CONSTRAINT `ck_billing_accounts_amounts` CHECK (((`held_amount` >= 0) and (`credit_limit` >= 0) and (((`posted_balance` + `credit_limit`) >= 0) or (`status` = _utf8mb4'frozen')))),
  CONSTRAINT `ck_billing_accounts_status` CHECK ((`status` in (_utf8mb4'open',_utf8mb4'frozen',_utf8mb4'closed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_call_rate_snapshots` (
  `reservation_id` bigint unsigned NOT NULL,
  `sell_schedule` json NOT NULL,
  `request_facts` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`reservation_id`),
  CONSTRAINT `fk_billing_rate_snapshot_reservation` FOREIGN KEY (`reservation_id`) REFERENCES `billing_reservations` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_billing_call_rate_snapshot_no_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_billing_call_rate_snapshot_no_update` BEFORE UPDATE ON `billing_call_rate_snapshots` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Call rate snapshots are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_billing_call_rate_snapshot_no_delete`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_billing_call_rate_snapshot_no_delete` BEFORE DELETE ON `billing_call_rate_snapshots` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Call rate snapshots are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_currency_definitions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `definition_version` int unsigned NOT NULL,
  `fraction_digits` tinyint unsigned NOT NULL,
  `rounding_mode` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `max_amount` decimal(38,18) NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_currency_code_version` (`currency_code`,`definition_version`),
  UNIQUE KEY `uq_billing_currency_id_code_version` (`id`,`currency_code`,`definition_version`),
  CONSTRAINT `ck_billing_currency_fraction_digits` CHECK ((`fraction_digits` <= 18)),
  CONSTRAINT `ck_billing_currency_max_amount` CHECK ((`max_amount` > 0)),
  CONSTRAINT `ck_billing_currency_rounding` CHECK ((`rounding_mode` in (_utf8mb4'half_even',_utf8mb4'half_up',_utf8mb4'floor',_utf8mb4'ceiling'))),
  CONSTRAINT `ck_billing_currency_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `billing_account_id` bigint unsigned NOT NULL,
  `reservation_id` bigint unsigned DEFAULT NULL,
  `call_id` bigint unsigned DEFAULT NULL,
  `event_key` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `event_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `posting_rule_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_events_account_key` (`billing_account_id`,`event_key`),
  UNIQUE KEY `uq_billing_events_id_currency` (`id`,`currency_code`,`currency_version`),
  KEY `fk_billing_events_account` (`billing_account_id`,`currency_code`,`currency_version`),
  KEY `fk_billing_events_reservation` (`reservation_id`),
  KEY `fk_billing_events_call` (`call_id`),
  KEY `fk_billing_event_posting_rule` (`posting_rule_id`,`event_type`,`currency_code`,`currency_version`),
  CONSTRAINT `fk_billing_event_posting_rule` FOREIGN KEY (`posting_rule_id`, `event_type`, `currency_code`, `currency_version`) REFERENCES `billing_posting_rules` (`id`, `event_type`, `currency_code`, `currency_version`),
  CONSTRAINT `fk_billing_events_account` FOREIGN KEY (`billing_account_id`, `currency_code`, `currency_version`) REFERENCES `billing_accounts` (`id`, `currency_code`, `currency_version`),
  CONSTRAINT `fk_billing_events_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_billing_events_reservation` FOREIGN KEY (`reservation_id`) REFERENCES `billing_reservations` (`id`),
  CONSTRAINT `ck_billing_events_amount` CHECK ((`amount` >= 0)),
  CONSTRAINT `ck_billing_events_type` CHECK ((`event_type` in (_utf8mb4'funding_credit',_utf8mb4'funding_debit',_utf8mb4'reservation_created',_utf8mb4'reservation_held_unknown',_utf8mb4'reservation_settled',_utf8mb4'reservation_released',_utf8mb4'refund',_utf8mb4'charge_adjustment',_utf8mb4'budget_limit_exceeded')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `idempotent_key` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '幂等键(如task_no)',
  `token_id` bigint unsigned DEFAULT NULL COMMENT '令牌ID',
  `user_id` bigint unsigned DEFAULT NULL COMMENT '用户ID',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `attempt_id` bigint unsigned NOT NULL DEFAULT '0',
  `phase` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `pricing_snapshot` json DEFAULT NULL,
  `amount` decimal(20,8) NOT NULL DEFAULT '0.00000000' COMMENT '金额',
  `type` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型(deduct/refund)',
  `status` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'success' COMMENT '状态',
  `remark` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '备注',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_billing_logs_idempotent_key` (`idempotent_key`),
  KEY `idx_billing_logs_created_at` (`created_at`),
  KEY `idx_billing_logs_deleted_at` (`deleted_at`),
  KEY `idx_billing_logs_token_id` (`token_id`),
  KEY `idx_billing_logs_user_id` (`user_id`),
  KEY `idx_billing_logs_call_id` (`call_id`),
  KEY `idx_billing_logs_attempt_id` (`attempt_id`),
  KEY `idx_billing_logs_phase` (`phase`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_posting_rules` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `event_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `rule_version` int unsigned NOT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `currency_version` int unsigned DEFAULT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `amount_source` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'event_amount',
  `user_direction` varchar(8) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `system_account_code` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `budget_effect` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'none',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_posting_rules_type_version` (`event_type`,`rule_version`),
  UNIQUE KEY `uq_billing_posting_rule_identity` (`id`,`event_type`,`currency_code`,`currency_version`),
  KEY `fk_billing_posting_rules_currency` (`currency_code`,`currency_version`),
  CONSTRAINT `fk_billing_posting_rules_currency` FOREIGN KEY (`currency_code`, `currency_version`) REFERENCES `billing_currency_definitions` (`currency_code`, `definition_version`),
  CONSTRAINT `ck_billing_posting_rule_amount` CHECK ((`amount_source` = _utf8mb4'event_amount')),
  CONSTRAINT `ck_billing_posting_rule_budget` CHECK ((`budget_effect` in (_utf8mb4'none',_utf8mb4'hold',_utf8mb4'release',_utf8mb4'consume'))),
  CONSTRAINT `ck_billing_posting_rule_direction` CHECK ((((`user_direction` is null) and (`system_account_code` is null)) or ((`user_direction` is not null) and (`user_direction` in (_utf8mb4'debit',_utf8mb4'credit')) and (`system_account_code` is not null)))),
  CONSTRAINT `ck_billing_posting_rules_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_billing_posting_rule_no_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_billing_posting_rule_no_update` BEFORE UPDATE ON `billing_posting_rules` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Posting rules are immutable; create a new version' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_billing_posting_rule_no_delete`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_billing_posting_rule_no_delete` BEFORE DELETE ON `billing_posting_rules` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Posting rules are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_reservations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned NOT NULL,
  `billing_account_id` bigint unsigned NOT NULL,
  `budget_window_id` bigint unsigned NOT NULL,
  `amount` decimal(38,18) NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `resolved_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_reservations_call` (`call_id`),
  KEY `fk_billing_reservations_account` (`billing_account_id`),
  KEY `fk_billing_reservations_budget_window` (`budget_window_id`),
  CONSTRAINT `fk_billing_reservations_account` FOREIGN KEY (`billing_account_id`) REFERENCES `billing_accounts` (`id`),
  CONSTRAINT `fk_billing_reservations_budget_window` FOREIGN KEY (`budget_window_id`) REFERENCES `token_budget_windows` (`id`),
  CONSTRAINT `fk_billing_reservations_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `ck_billing_reservations_amount` CHECK ((`amount` >= 0)),
  CONSTRAINT `ck_billing_reservations_state` CHECK ((`state` in (_utf8mb4'active',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unknown_hold')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_settlements` (
  `reservation_id` bigint unsigned NOT NULL,
  `billing_event_id` bigint unsigned NOT NULL,
  `authorized_amount` decimal(38,18) NOT NULL,
  `actual_amount` decimal(38,18) NOT NULL,
  `charge_details` json DEFAULT NULL,
  `authorization_excess` decimal(38,18) NOT NULL,
  `debt_after` decimal(38,18) NOT NULL,
  `budget_excess` decimal(38,18) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`reservation_id`),
  UNIQUE KEY `uq_billing_settlement_event` (`billing_event_id`),
  CONSTRAINT `fk_billing_settlement_event` FOREIGN KEY (`billing_event_id`) REFERENCES `billing_events` (`id`),
  CONSTRAINT `fk_billing_settlement_reservation` FOREIGN KEY (`reservation_id`) REFERENCES `billing_reservations` (`id`),
  CONSTRAINT `ck_billing_settlement_amounts` CHECK (((`authorized_amount` >= 0) and (`actual_amount` >= 0) and (`authorization_excess` = greatest((`actual_amount` - `authorized_amount`),0)) and (`debt_after` >= 0) and (`budget_excess` >= 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_billing_settlement_no_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_billing_settlement_no_update` BEFORE UPDATE ON `billing_settlements` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Settlements are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_billing_settlement_no_delete`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_billing_settlement_no_delete` BEFORE DELETE ON `billing_settlements` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Settlements are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `billing_system_state` (
  `id` tinyint unsigned NOT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `initialization_version` bigint unsigned NOT NULL DEFAULT '1',
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_billing_system_state_currency` (`id`,`currency_code`,`currency_version`),
  KEY `fk_billing_system_state_currency` (`currency_code`,`currency_version`),
  CONSTRAINT `fk_billing_system_state_currency` FOREIGN KEY (`currency_code`, `currency_version`) REFERENCES `billing_currency_definitions` (`currency_code`, `definition_version`),
  CONSTRAINT `ck_billing_system_state_singleton` CHECK ((`id` = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `channel_accounts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `channel_id` bigint unsigned NOT NULL COMMENT '所属渠道ID',
  `name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '账号名称',
  `api_key` text COLLATE utf8mb4_unicode_ci COMMENT 'API密钥',
  `config` json DEFAULT NULL COMMENT '账号配置(JSON)',
  `weight` bigint DEFAULT '10' COMMENT '负载均衡权重',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `max_tasks` bigint DEFAULT '0' COMMENT '最大并发任务数(0=不限制)',
  `current_tasks` bigint DEFAULT '0' COMMENT '当前任务数',
  `supported_models` json DEFAULT NULL COMMENT '支持的模型列表(空=全部)',
  PRIMARY KEY (`id`),
  KEY `idx_channel_accounts_created_at` (`created_at`),
  KEY `idx_channel_accounts_deleted_at` (`deleted_at`),
  KEY `idx_channel_accounts_channel_id` (`channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `channel_request_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `attempt_id` bigint unsigned NOT NULL DEFAULT '0',
  `task_id` bigint unsigned DEFAULT NULL COMMENT '关联任务ID',
  `task_no` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '任务编号',
  `conversation_id` bigint unsigned DEFAULT NULL COMMENT '关联对话ID(Chat)',
  `channel_id` bigint unsigned DEFAULT NULL COMMENT '渠道ID',
  `account_id` bigint unsigned DEFAULT NULL COMMENT '渠道账号ID',
  `capability_code` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '能力编码或模型编码',
  `request_type` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求类型',
  `is_stream` tinyint(1) DEFAULT '0' COMMENT '是否流式请求',
  `model_code` varchar(80) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求模型',
  `vendor_model` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '供应商模型',
  `upstream_transport` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '实际上游传输协议',
  `request_path` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '实际上游请求路径',
  `finish_reason` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '完成原因',
  `response_preview` text COLLATE utf8mb4_unicode_ci COMMENT '响应摘要',
  `usage_prompt_tokens` bigint DEFAULT '0' COMMENT '输入token摘要',
  `usage_completion_tokens` bigint DEFAULT '0' COMMENT '输出token摘要',
  `usage_total_tokens` bigint DEFAULT '0' COMMENT '总token摘要',
  `method` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求方法',
  `url` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '请求URL',
  `request_headers` text COLLATE utf8mb4_unicode_ci COMMENT '请求头(JSON)',
  `request_body` mediumtext COLLATE utf8mb4_unicode_ci COMMENT '请求体',
  `status_code` bigint DEFAULT NULL COMMENT 'HTTP状态码',
  `response_body` mediumtext COLLATE utf8mb4_unicode_ci COMMENT '响应体',
  `duration_ms` bigint DEFAULT NULL COMMENT '耗时(毫秒)',
  `error_message` text COLLATE utf8mb4_unicode_ci COMMENT '错误信息',
  `request_at` datetime(3) DEFAULT NULL COMMENT '请求时间',
  PRIMARY KEY (`id`),
  KEY `idx_channel_request_logs_created_at` (`created_at`),
  KEY `idx_channel_request_logs_deleted_at` (`deleted_at`),
  KEY `idx_channel_request_logs_call_id` (`call_id`),
  KEY `idx_channel_request_logs_attempt_id` (`attempt_id`),
  KEY `idx_channel_request_logs_task_id` (`task_id`),
  KEY `idx_channel_request_logs_task_no` (`task_no`),
  KEY `idx_channel_request_logs_conversation_id` (`conversation_id`),
  KEY `idx_channel_request_logs_channel_id` (`channel_id`),
  KEY `idx_channel_request_at` (`channel_id`,`request_at`),
  KEY `idx_channel_request_logs_capability_code` (`capability_code`),
  KEY `idx_capability_request_at` (`capability_code`,`request_at`),
  KEY `idx_channel_request_logs_request_type` (`request_type`),
  KEY `idx_channel_request_logs_upstream_transport` (`upstream_transport`),
  KEY `idx_channel_request_logs_request_at` (`request_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `channels` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '渠道类型标识',
  `name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '渠道名称',
  `base_url` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '基础URL',
  `callback_secret` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '回调签名密钥',
  `config` json DEFAULT NULL COMMENT '渠道配置(JSON)',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `sort` bigint DEFAULT '0' COMMENT '排序(降序)',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_channels_type` (`type`),
  KEY `idx_channels_created_at` (`created_at`),
  KEY `idx_channels_deleted_at` (`deleted_at`),
  KEY `idx_channels_sort` (`sort`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `conversation_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `conversation_id` bigint unsigned NOT NULL,
  `turn_id` bigint unsigned NOT NULL,
  `turn_sequence` bigint unsigned NOT NULL,
  `direction` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ordinal` bigint NOT NULL,
  `canonical_json` json NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_conversation_item_ordinal` (`turn_id`,`ordinal`),
  KEY `idx_conversation_items_order` (`conversation_id`,`turn_sequence`,`ordinal`),
  KEY `idx_conversation_items_direction` (`direction`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `conversation_projection_outbox` (
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `conversation_id` bigint unsigned NOT NULL DEFAULT '0',
  `previous_response_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `request_log_id` bigint unsigned NOT NULL DEFAULT '0',
  `provider_response_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `finish_reason` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `input_json` longblob,
  `output_json` longblob,
  `input_ready` tinyint(1) NOT NULL DEFAULT '0',
  `input_prepared` tinyint(1) NOT NULL DEFAULT '0',
  `context_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `output_ready` tinyint(1) NOT NULL DEFAULT '0',
  `retry_count` bigint NOT NULL DEFAULT '0',
  `last_error` text COLLATE utf8mb4_unicode_ci,
  `last_attempt_at` datetime(3) DEFAULT NULL,
  `next_attempt_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`call_id`),
  KEY `idx_conversation_projection_conversation_id` (`conversation_id`),
  KEY `idx_conversation_projection_previous_response_id` (`previous_response_id`),
  KEY `idx_conversation_projection_request_log_id` (`request_log_id`),
  KEY `idx_conversation_projection_retry` (`next_attempt_at`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `conversation_turns` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `conversation_id` bigint unsigned NOT NULL,
  `turn_sequence` bigint unsigned NOT NULL,
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_log_id` bigint unsigned NOT NULL DEFAULT '0',
  `model` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider_response_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'completed',
  `context_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'legacy',
  `input_tokens` bigint NOT NULL DEFAULT '0',
  `output_tokens` bigint NOT NULL DEFAULT '0',
  `total_tokens` bigint NOT NULL DEFAULT '0',
  `cost` decimal(20,8) NOT NULL DEFAULT '0.00000000',
  `latency_ms` bigint NOT NULL DEFAULT '0',
  `finish_reason` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `error_message` text COLLATE utf8mb4_unicode_ci,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_conversation_turn_sequence` (`conversation_id`,`turn_sequence`),
  UNIQUE KEY `idx_conversation_turns_call_id` (`call_id`),
  KEY `idx_conversation_turn_created` (`conversation_id`,`created_at`),
  KEY `idx_conversation_turns_request_log_id` (`request_log_id`),
  KEY `idx_conversation_turns_model` (`model`),
  KEY `idx_conversation_turns_provider_response_id` (`provider_response_id`),
  KEY `idx_conversation_turns_status` (`status`),
  KEY `idx_conversation_turns_error_code` (`error_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `conversations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `user_id` bigint unsigned NOT NULL COMMENT '用户ID',
  `token_id` bigint unsigned NOT NULL COMMENT 'Token ID',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '最近关联调用ID',
  `title` varchar(200) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '对话标题',
  `model` varchar(80) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '最后使用模型',
  `system_prompt` longtext COLLATE utf8mb4_unicode_ci COMMENT '系统提示词',
  `last_request_log_id` bigint unsigned DEFAULT '0' COMMENT '最近一次请求日志ID',
  `last_status` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '最近一次请求状态',
  `provider_response_id` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT '' COMMENT '上游有状态对话ID(如火山response_id)',
  `provider_key_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT '上游状态所属网关Key',
  `upstream_transport` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '上游状态所属Transport',
  `total_tokens` bigint DEFAULT '0' COMMENT '累计token',
  `message_count` bigint DEFAULT '0' COMMENT '消息数量',
  `canonical_item_count` bigint unsigned NOT NULL DEFAULT '0' COMMENT '可匹配 canonical 项数',
  `canonical_bytes` bigint unsigned NOT NULL DEFAULT '0' COMMENT '已完成 canonical 正文字节数',
  `canonical_match_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '已完成 canonical 滚动匹配哈希',
  `canonical_state_version` tinyint unsigned NOT NULL DEFAULT '0' COMMENT 'canonical 匹配状态版本',
  `turn_sequence` bigint unsigned NOT NULL DEFAULT '0' COMMENT '最后分配的轮次序号',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  PRIMARY KEY (`id`),
  KEY `idx_conversations_canonical_match` (`user_id`,`token_id`,`status`,`updated_at`,`id`),
  KEY `idx_conversations_created_at` (`created_at`),
  KEY `idx_conversations_deleted_at` (`deleted_at`),
  KEY `idx_conversations_user_id` (`user_id`),
  KEY `idx_conversations_token_id` (`token_id`),
  KEY `idx_conversations_call_id` (`call_id`),
  KEY `idx_conversations_last_request_log_id` (`last_request_log_id`),
  KEY `idx_conversations_provider_response_id` (`provider_response_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `crypto_key_readiness` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `deployment_generation_id` bigint unsigned NOT NULL,
  `deployment_member_id` bigint unsigned NOT NULL,
  `keyring_id` bigint unsigned NOT NULL,
  `key_version` int unsigned NOT NULL,
  `operation` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `checked_at` datetime(3) NOT NULL,
  `expires_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_crypto_key_readiness_member_key_operation` (`deployment_member_id`,`keyring_id`,`key_version`,`operation`),
  KEY `fk_crypto_key_readiness_member` (`deployment_generation_id`,`deployment_member_id`),
  KEY `fk_crypto_key_readiness_key_version` (`keyring_id`,`key_version`),
  CONSTRAINT `fk_crypto_key_readiness_key_version` FOREIGN KEY (`keyring_id`, `key_version`) REFERENCES `crypto_key_versions` (`keyring_id`, `key_version`),
  CONSTRAINT `fk_crypto_key_readiness_member` FOREIGN KEY (`deployment_generation_id`, `deployment_member_id`) REFERENCES `gw_deployment_members` (`deployment_generation_id`, `id`),
  CONSTRAINT `ck_crypto_key_readiness_operation` CHECK ((`operation` in (_utf8mb4'mac',_utf8mb4'wrap',_utf8mb4'unwrap',_utf8mb4'encrypt',_utf8mb4'decrypt'))),
  CONSTRAINT `ck_crypto_key_readiness_status` CHECK ((`status` in (_utf8mb4'ready',_utf8mb4'failed',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `crypto_key_versions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `keyring_id` bigint unsigned NOT NULL,
  `key_version` int unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `provider_key_ref` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `algorithm` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `retired_at` datetime(3) DEFAULT NULL,
  `current_keyring_id` bigint unsigned GENERATED ALWAYS AS ((case when (`status` = _utf8mb4'current') then `keyring_id` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_crypto_key_versions_ring_version` (`keyring_id`,`key_version`),
  UNIQUE KEY `uq_crypto_key_versions_ring_id_id` (`keyring_id`,`id`),
  UNIQUE KEY `uq_crypto_key_versions_one_current` (`current_keyring_id`),
  CONSTRAINT `fk_crypto_key_versions_keyring` FOREIGN KEY (`keyring_id`) REFERENCES `crypto_keyring_state` (`id`),
  CONSTRAINT `ck_crypto_key_versions_algorithm` CHECK ((`algorithm` = _utf8mb4'aes-256-gcm')),
  CONSTRAINT `ck_crypto_key_versions_status` CHECK ((`status` in (_utf8mb4'preparing',_utf8mb4'readable',_utf8mb4'current',_utf8mb4'retired',_utf8mb4'security_revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `crypto_keyring_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `purpose` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `current_version` int unsigned DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_crypto_keyring_state_purpose` (`purpose`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `encrypted_blob_key_wraps` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `encrypted_blob_id` bigint unsigned NOT NULL,
  `keyring_id` bigint unsigned NOT NULL,
  `kek_version` int unsigned NOT NULL,
  `wrap_nonce` varbinary(32) NOT NULL,
  `wrapped_dek` varbinary(255) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_encrypted_blob_key_wraps_blob_ring_version` (`encrypted_blob_id`,`keyring_id`,`kek_version`),
  KEY `fk_encrypted_blob_key_wraps_key_version` (`keyring_id`,`kek_version`),
  CONSTRAINT `fk_encrypted_blob_key_wraps_blob` FOREIGN KEY (`encrypted_blob_id`, `keyring_id`) REFERENCES `encrypted_blobs` (`id`, `keyring_id`),
  CONSTRAINT `fk_encrypted_blob_key_wraps_key_version` FOREIGN KEY (`keyring_id`, `kek_version`) REFERENCES `crypto_key_versions` (`keyring_id`, `key_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `encrypted_blobs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `keyring_id` bigint unsigned NOT NULL,
  `purpose` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `schema_version` int unsigned NOT NULL,
  `aad_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `nonce` varbinary(32) NOT NULL,
  `ciphertext` longblob NOT NULL,
  `content_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `content_length` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `purged_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_encrypted_blobs_keyring_id_id` (`keyring_id`,`id`),
  UNIQUE KEY `uq_encrypted_blobs_id_keyring_id` (`id`,`keyring_id`),
  KEY `idx_encrypted_blobs_purpose_created` (`purpose`,`created_at`),
  KEY `idx_encrypted_blobs_purged_at` (`purged_at`),
  CONSTRAINT `fk_encrypted_blobs_keyring` FOREIGN KEY (`keyring_id`) REFERENCES `crypto_keyring_state` (`id`),
  CONSTRAINT `ck_encrypted_blobs_length` CHECK ((`content_length` <= 1073741824))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `endpoint_accounts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `endpoint_id` bigint unsigned NOT NULL COMMENT '端点ID',
  `account_id` bigint unsigned NOT NULL COMMENT '渠道账号(key)ID',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `priority` bigint NOT NULL DEFAULT '0' COMMENT '优先级(降序)',
  `weight` bigint NOT NULL DEFAULT '10' COMMENT '同优先级流量权重',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_endpoint_account` (`endpoint_id`,`account_id`),
  KEY `idx_endpoint_accounts_account_id` (`account_id`),
  KEY `idx_endpoint_accounts_route` (`endpoint_id`,`status`,`priority`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `endpoint_adapter_revisions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `adapter_id` bigint unsigned NOT NULL,
  `version` bigint NOT NULL,
  `digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `config` json NOT NULL,
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_endpoint_adapter_revision` (`adapter_id`,`version`),
  KEY `idx_endpoint_adapter_revisions_digest` (`digest`),
  KEY `idx_endpoint_adapter_revisions_created_by` (`created_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `endpoint_adapters` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `endpoint_id` bigint unsigned NOT NULL,
  `code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `active_revision_id` bigint unsigned NOT NULL DEFAULT '0',
  `status` tinyint NOT NULL DEFAULT '1',
  `config` json DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_endpoint_adapter` (`endpoint_id`,`code`),
  KEY `idx_endpoint_adapters_revision` (`active_revision_id`),
  KEY `idx_endpoint_adapters_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `endpoint_route_states` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `endpoint_id` bigint unsigned NOT NULL,
  `account_id` bigint unsigned NOT NULL,
  `route_operation` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL,
  `adapter_id` bigint unsigned NOT NULL,
  `revision_id` bigint unsigned NOT NULL,
  `status` tinyint NOT NULL DEFAULT '1',
  `disabled_until` datetime(3) DEFAULT NULL,
  `status_code` bigint NOT NULL DEFAULT '0',
  `fail_count` bigint NOT NULL DEFAULT '0',
  `last_discovery_at` datetime(3) DEFAULT NULL,
  `last_success_at` datetime(3) DEFAULT NULL,
  `last_error` varchar(1000) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `discovered_models` json DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_endpoint_route_state` (`endpoint_id`,`account_id`,`route_operation`),
  KEY `idx_endpoint_route_states_account` (`account_id`),
  KEY `idx_endpoint_route_states_revision` (`revision_id`),
  KEY `idx_endpoint_route_states_disabled_until` (`disabled_until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `endpoints` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `model_code` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '模型标识',
  `route_operation` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Endpoint route operation',
  `supported_operations` json DEFAULT NULL COMMENT 'Supported public operations',
  `channel_id` bigint unsigned NOT NULL COMMENT '渠道ID',
  `account_id` bigint unsigned DEFAULT '0' COMMENT '所属账号(key)ID,0=渠道级兼容',
  `origin_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'legacy_unknown',
  `origin_account_id` bigint unsigned NOT NULL DEFAULT '0',
  `origin_snapshot` json DEFAULT NULL,
  `discovered_at` datetime(3) DEFAULT NULL,
  `protocol` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'openai' COMMENT '协议类型',
  `request_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '/v1/chat/completions' COMMENT '请求路径',
  `request_method` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'POST' COMMENT '请求方法',
  `content_type` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT 'application/json' COMMENT '内容类型',
  `auth_location` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'header' COMMENT '认证位置',
  `auth_key` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT 'Authorization' COMMENT '认证参数名',
  `auth_value_prefix` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT 'Bearer ' COMMENT '认证值前缀',
  `vendor_model` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '上游模型名',
  `interaction_mode` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'stream' COMMENT '交互模式',
  `supports_stream` tinyint(1) DEFAULT '1' COMMENT '是否支持流式',
  `default_stream` tinyint(1) DEFAULT '1' COMMENT '默认使用流式',
  `price_mode` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'token' COMMENT '计价模式',
  `input_price` decimal(12,8) DEFAULT '0.00000000' COMMENT '输入价格',
  `output_price` decimal(12,8) DEFAULT '0.00000000' COMMENT '输出价格',
  `param_mapping` json DEFAULT NULL COMMENT '参数映射',
  `param_schema` json DEFAULT NULL COMMENT '端点参数schema(覆盖模型级)',
  `response_mapping` json DEFAULT NULL COMMENT '响应映射',
  `poll_path` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT '' COMMENT '轮询路径',
  `poll_method` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'GET' COMMENT '轮询方法',
  `poll_interval` bigint DEFAULT '5' COMMENT '轮询间隔秒',
  `poll_max_attempts` bigint DEFAULT '60' COMMENT '最大轮询次数',
  `poll_param_mapping` json DEFAULT NULL COMMENT '轮询参数映射',
  `poll_response_mapping` json DEFAULT NULL COMMENT '轮询响应映射',
  `callback_mapping` json DEFAULT NULL COMMENT '回调映射',
  `extra_headers` json DEFAULT NULL COMMENT '额外请求头',
  `extra_config` json DEFAULT NULL COMMENT '扩展配置',
  `timeout` bigint DEFAULT '120' COMMENT '超时秒',
  `priority` bigint DEFAULT '0' COMMENT '优先级',
  `status` tinyint DEFAULT '1' COMMENT '状态',
  PRIMARY KEY (`id`),
  KEY `idx_endpoints_created_at` (`created_at`),
  KEY `idx_endpoints_deleted_at` (`deleted_at`),
  KEY `idx_endpoints_model_code` (`model_code`),
  KEY `idx_endpoints_channel_id` (`channel_id`),
  KEY `idx_endpoints_account_id` (`account_id`),
  KEY `idx_endpoints_route_operation` (`route_operation`),
  KEY `idx_endpoints_origin_type` (`origin_type`),
  KEY `idx_endpoints_origin_account` (`origin_account_id`),
  CONSTRAINT `fk_endpoints_channel` FOREIGN KEY (`channel_id`) REFERENCES `channels` (`id`),
  CONSTRAINT `fk_endpoints_model` FOREIGN KEY (`model_code`) REFERENCES `models` (`code`) ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `fx_rate_snapshots` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `base_currency` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `quote_currency` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `effective_at` datetime(3) NOT NULL,
  `rate` decimal(38,18) NOT NULL,
  `evidence_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fx_rate_snapshots_pair_time_evidence` (`base_currency`,`quote_currency`,`effective_at`,`evidence_hmac`),
  CONSTRAINT `ck_fx_rate_snapshots_distinct_currency` CHECK ((`base_currency` <> `quote_currency`)),
  CONSTRAINT `ck_fx_rate_snapshots_rate` CHECK ((`rate` > 0)),
  CONSTRAINT `ck_fx_rate_snapshots_status` CHECK ((`status` in (_utf8mb4'pending',_utf8mb4'accepted',_utf8mb4'rejected')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gateway_channels` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `display_name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gateway_channels_code` (`channel_code`),
  CONSTRAINT `ck_gateway_channels_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_abilities` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `model_name` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL,
  `channel_id` bigint unsigned NOT NULL,
  `key_id` bigint unsigned NOT NULL,
  `vendor_model` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL,
  `priority` bigint DEFAULT '0',
  `price_mode` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'token',
  `input_price` decimal(12,8) DEFAULT '0.00000000',
  `output_price` decimal(12,8) DEFAULT '0.00000000',
  `capabilities` json DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_gw_ability_model` (`model_name`,`priority`,`status`),
  KEY `idx_gw_abilities_key_id` (`key_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_ability_transports` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `ability_id` bigint unsigned NOT NULL,
  `transport` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` tinyint NOT NULL DEFAULT '1',
  `config` json DEFAULT NULL,
  `checked_at` datetime(3) DEFAULT NULL,
  `last_error` varchar(1000) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_gw_ability_transport` (`ability_id`,`transport`),
  KEY `idx_gw_ability_transport_status` (`ability_id`,`transport`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_adapter_implementations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `adapter_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `contract_version` int unsigned NOT NULL,
  `implementation_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `minimum_semantic_version` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_adapter_implementations_code_version` (`adapter_code`,`contract_version`),
  UNIQUE KEY `uq_gw_adapter_implementations_digest` (`implementation_digest`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_ai_responses` (
  `resource_id` bigint unsigned NOT NULL,
  `response_no` char(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `previous_response_resource_id` bigint unsigned DEFAULT NULL,
  `result_summary` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`resource_id`),
  UNIQUE KEY `uq_gw_ai_responses_response_no` (`response_no`),
  KEY `fk_gw_ai_responses_previous_resource` (`previous_response_resource_id`),
  CONSTRAINT `fk_gw_ai_responses_previous_resource` FOREIGN KEY (`previous_response_resource_id`) REFERENCES `gw_api_resources` (`id`),
  CONSTRAINT `fk_gw_ai_responses_resource` FOREIGN KEY (`resource_id`) REFERENCES `gw_api_resources` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_api_call_attempts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned NOT NULL,
  `attempt_no` bigint unsigned NOT NULL,
  `catalog_release_id` bigint unsigned NOT NULL,
  `sku_id` bigint unsigned NOT NULL,
  `route_id` bigint unsigned NOT NULL,
  `offering_id` bigint unsigned NOT NULL,
  `cost_plan_id` bigint unsigned NOT NULL,
  `product_transport_id` bigint unsigned NOT NULL,
  `credential_pool_id` bigint unsigned NOT NULL,
  `credential_id` bigint unsigned NOT NULL,
  `credential_version_id` bigint unsigned NOT NULL,
  `purpose_grant_id` bigint unsigned NOT NULL,
  `state` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `active_call_id` bigint unsigned GENERATED ALWAYS AS ((case when (`state` in (_utf8mb4'started',_utf8mb4'recovery_pending')) then `call_id` else NULL end)) STORED,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_api_call_attempts_call_no` (`call_id`,`attempt_no`),
  UNIQUE KEY `uq_gw_api_call_attempts_call_id_id` (`call_id`,`id`),
  UNIQUE KEY `uq_gw_api_call_attempts_one_active` (`active_call_id`),
  KEY `fk_gw_api_call_attempts_route_selection` (`catalog_release_id`,`route_id`,`sku_id`,`offering_id`),
  KEY `fk_gw_api_call_attempts_sku` (`catalog_release_id`,`sku_id`),
  KEY `fk_gw_api_call_attempts_offering_transport` (`catalog_release_id`,`offering_id`,`product_transport_id`),
  KEY `fk_gw_api_call_attempts_product_transport` (`catalog_release_id`,`product_transport_id`),
  KEY `fk_gw_api_call_attempts_pool_credential` (`credential_pool_id`,`credential_id`),
  KEY `fk_gw_api_call_attempts_credential_version` (`credential_id`,`credential_version_id`),
  KEY `fk_gw_api_call_attempts_purpose_grant` (`credential_id`,`purpose_grant_id`),
  KEY `fk_gw_api_call_attempts_cost_plan` (`catalog_release_id`,`offering_id`,`cost_plan_id`),
  CONSTRAINT `fk_gw_api_call_attempts_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_api_call_attempts_cost_plan` FOREIGN KEY (`catalog_release_id`, `offering_id`, `cost_plan_id`) REFERENCES `gw_cost_plans` (`release_id`, `offering_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_api_call_attempts_credential_version` FOREIGN KEY (`credential_id`, `credential_version_id`) REFERENCES `gw_credential_versions` (`credential_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_offering` FOREIGN KEY (`catalog_release_id`, `offering_id`) REFERENCES `gw_offerings` (`release_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_offering_transport` FOREIGN KEY (`catalog_release_id`, `offering_id`, `product_transport_id`) REFERENCES `gw_offerings` (`release_id`, `id`, `product_transport_id`),
  CONSTRAINT `fk_gw_api_call_attempts_pool` FOREIGN KEY (`credential_pool_id`) REFERENCES `gw_credential_pools` (`id`),
  CONSTRAINT `fk_gw_api_call_attempts_pool_credential` FOREIGN KEY (`credential_pool_id`, `credential_id`) REFERENCES `gw_credentials` (`credential_pool_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_product_transport` FOREIGN KEY (`catalog_release_id`, `product_transport_id`) REFERENCES `gw_product_transports` (`release_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_purpose_grant` FOREIGN KEY (`credential_id`, `purpose_grant_id`) REFERENCES `gw_credential_purpose_grants` (`credential_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_route` FOREIGN KEY (`catalog_release_id`, `route_id`) REFERENCES `gw_routes` (`release_id`, `id`),
  CONSTRAINT `fk_gw_api_call_attempts_route_selection` FOREIGN KEY (`catalog_release_id`, `route_id`, `sku_id`, `offering_id`) REFERENCES `gw_routes` (`release_id`, `id`, `sku_id`, `offering_id`),
  CONSTRAINT `fk_gw_api_call_attempts_sku` FOREIGN KEY (`catalog_release_id`, `sku_id`) REFERENCES `gw_skus` (`release_id`, `id`),
  CONSTRAINT `ck_gw_api_call_attempts_state` CHECK ((`state` in (_utf8mb4'started',_utf8mb4'recovery_pending',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'cancelled',_utf8mb4'not_created',_utf8mb4'terminated_unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_api_call_idempotencies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token_id` bigint unsigned NOT NULL,
  `operation_contract_id` bigint unsigned NOT NULL,
  `call_id` bigint unsigned DEFAULT NULL,
  `key_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `request_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `replay_expires_at` datetime(3) DEFAULT NULL,
  `key_reuse_after` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_api_call_idempotencies_token_contract_key` (`token_id`,`operation_contract_id`,`hmac_key_version`,`key_hmac`),
  KEY `fk_gw_api_call_idempotencies_operation_contract` (`operation_contract_id`),
  KEY `fk_gw_api_call_idempotencies_call` (`call_id`),
  CONSTRAINT `fk_gw_api_call_idempotencies_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_api_call_idempotencies_operation_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`),
  CONSTRAINT `ck_gw_api_call_idempotencies_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'expired',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_api_call_idempotency_keys` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `idempotency_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `operation_contract_id` bigint unsigned NOT NULL,
  `key_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_api_call_idempotency_keys_fact_version` (`idempotency_id`,`hmac_key_version`),
  UNIQUE KEY `uq_gw_api_call_idempotency_keys_lookup` (`token_id`,`operation_contract_id`,`hmac_key_version`,`key_hmac`),
  KEY `fk_gw_api_call_idempotency_keys_operation_contract` (`operation_contract_id`),
  CONSTRAINT `fk_gw_api_call_idempotency_keys_fact` FOREIGN KEY (`idempotency_id`) REFERENCES `gw_api_call_idempotencies` (`id`),
  CONSTRAINT `fk_gw_api_call_idempotency_keys_operation_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_api_call_payloads` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned NOT NULL,
  `kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `schema_version` int unsigned NOT NULL,
  `encrypted_blob_id` bigint unsigned DEFAULT NULL,
  `content_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `content_length` bigint unsigned NOT NULL,
  `retention_until` datetime(3) DEFAULT NULL,
  `purged_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_api_call_payloads_call_kind` (`call_id`,`kind`),
  KEY `fk_gw_api_call_payloads_blob` (`encrypted_blob_id`),
  CONSTRAINT `fk_gw_api_call_payloads_blob` FOREIGN KEY (`encrypted_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_api_call_payloads_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `ck_gw_api_call_payloads_kind` CHECK ((`kind` in (_utf8mb4'request',_utf8mb4'result')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_api_calls` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `public_id` char(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `operation_contract_id` bigint unsigned NOT NULL,
  `catalog_release_id` bigint unsigned NOT NULL,
  `model_operation_id` bigint unsigned NOT NULL,
  `sku_id` bigint unsigned NOT NULL,
  `request_payload_id` bigint unsigned DEFAULT NULL,
  `result_payload_id` bigint unsigned DEFAULT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `quoted_amount` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `price_currency` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `price_currency_version` int unsigned NOT NULL,
  `delivery_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `xfs_api_key` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `current_attempt_id` bigint unsigned DEFAULT NULL,
  `final_attempt_id` bigint unsigned DEFAULT NULL,
  `callback_event_seq` bigint unsigned NOT NULL DEFAULT '0',
  `next_action_at` datetime(3) DEFAULT NULL,
  `lease_owner` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  `xfs_base_url` varchar(2048) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_api_calls_public_id` (`public_id`),
  UNIQUE KEY `uq_gw_api_calls_id_public_id` (`id`,`public_id`),
  KEY `fk_gw_api_calls_operation_contract` (`operation_contract_id`),
  KEY `fk_gw_api_calls_model_operation` (`catalog_release_id`,`model_operation_id`),
  KEY `fk_gw_api_calls_model_operation_sku` (`catalog_release_id`,`sku_id`,`model_operation_id`),
  CONSTRAINT `fk_gw_api_calls_model_operation` FOREIGN KEY (`catalog_release_id`, `model_operation_id`) REFERENCES `gw_model_operations` (`release_id`, `id`),
  CONSTRAINT `fk_gw_api_calls_model_operation_sku` FOREIGN KEY (`catalog_release_id`, `sku_id`, `model_operation_id`) REFERENCES `gw_skus` (`release_id`, `id`, `model_operation_id`),
  CONSTRAINT `fk_gw_api_calls_operation_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`),
  CONSTRAINT `fk_gw_api_calls_release` FOREIGN KEY (`catalog_release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `fk_gw_api_calls_sku` FOREIGN KEY (`catalog_release_id`, `sku_id`) REFERENCES `gw_skus` (`release_id`, `id`),
  CONSTRAINT `ck_gw_api_calls_amount` CHECK ((`quoted_amount` >= 0)),
  CONSTRAINT `ck_gw_api_calls_delivery_mode` CHECK ((`delivery_mode` in (_utf8mb4'reference',_utf8mb4'managed_copy'))),
  CONSTRAINT `ck_gw_api_calls_status` CHECK ((`status` in (_utf8mb4'received',_utf8mb4'in_progress',_utf8mb4'retry_pending',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'cancelled',_utf8mb4'indeterminate')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_api_resources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `public_id` char(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  `resource_kind` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `call_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_api_resources_public_id` (`public_id`),
  UNIQUE KEY `uq_gw_api_resources_call` (`call_id`),
  UNIQUE KEY `uq_gw_api_resources_id_public_id` (`id`,`public_id`),
  KEY `idx_gw_api_resources_visibility` (`token_id`,`resource_kind`,`deleted_at`,`public_id`),
  CONSTRAINT `fk_gw_api_resources_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `ck_gw_api_resources_kind` CHECK ((`resource_kind` in (_utf8mb4'capability_task',_utf8mb4'response',_utf8mb4'video_task',_utf8mb4'file')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_async_executions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `attempt_id` bigint unsigned NOT NULL,
  `upstream_scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `upstream_scope_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `cancel_intent_at` datetime(3) DEFAULT NULL,
  `cancel_resolved_at` datetime(3) DEFAULT NULL,
  `cancel_resolution` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `next_action_at` datetime(3) DEFAULT NULL,
  `action_seq` bigint unsigned NOT NULL DEFAULT '0',
  `lease_owner` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_async_executions_attempt` (`attempt_id`),
  UNIQUE KEY `uq_gw_async_id_attempt` (`id`,`attempt_id`),
  CONSTRAINT `fk_gw_async_executions_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `ck_gw_async_executions_cancel` CHECK ((((`cancel_intent_at` is null) and (`cancel_resolved_at` is null) and (`cancel_resolution` is null)) or ((`cancel_intent_at` is not null) and (((`cancel_resolved_at` is null) and (`cancel_resolution` is null)) or ((`cancel_resolved_at` is not null) and (`cancel_resolution` in (_utf8mb4'confirmed',_utf8mb4'rejected',_utf8mb4'superseded'))))))),
  CONSTRAINT `ck_gw_async_executions_state` CHECK ((`state` in (_utf8mb4'allocated',_utf8mb4'submitting',_utf8mb4'submission_unknown',_utf8mb4'accepted',_utf8mb4'running',_utf8mb4'manual_review',_utf8mb4'cancel_requested',_utf8mb4'cancel_unknown',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'cancelled',_utf8mb4'not_created',_utf8mb4'terminated_unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_async_outbox` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned DEFAULT NULL,
  `attempt_id` bigint unsigned DEFAULT NULL,
  `async_execution_id` bigint unsigned DEFAULT NULL,
  `callback_receipt_id` bigint unsigned DEFAULT NULL,
  `result_delivery_id` bigint unsigned DEFAULT NULL,
  `action_seq` bigint unsigned NOT NULL,
  `action` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `available_at` datetime(3) NOT NULL,
  `lease_owner` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `attempt_count` bigint unsigned NOT NULL DEFAULT '0',
  `last_error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_async_outbox_call_seq` (`call_id`,`action_seq`),
  UNIQUE KEY `uq_gw_async_outbox_attempt_seq` (`attempt_id`,`action_seq`),
  UNIQUE KEY `uq_gw_async_outbox_async_seq` (`async_execution_id`,`action_seq`),
  UNIQUE KEY `uq_gw_outbox_id_async` (`id`,`async_execution_id`),
  UNIQUE KEY `uq_gw_async_outbox_callback_receipt_seq` (`callback_receipt_id`,`action_seq`),
  UNIQUE KEY `uq_gw_async_outbox_delivery_seq` (`result_delivery_id`,`action_seq`),
  UNIQUE KEY `uq_gw_outbox_id_delivery` (`id`,`result_delivery_id`),
  KEY `idx_gw_async_outbox_delivery_claim` (`action`,`status`,`available_at`,`lease_expires_at`,`id`),
  CONSTRAINT `fk_gw_async_outbox_async_execution` FOREIGN KEY (`async_execution_id`) REFERENCES `gw_async_executions` (`id`),
  CONSTRAINT `fk_gw_async_outbox_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_async_outbox_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_async_outbox_callback_receipt` FOREIGN KEY (`callback_receipt_id`) REFERENCES `gw_upstream_callback_receipts` (`id`),
  CONSTRAINT `fk_gw_async_outbox_result_delivery` FOREIGN KEY (`result_delivery_id`) REFERENCES `gw_result_deliveries` (`id`),
  CONSTRAINT `ck_gw_async_outbox_action` CHECK ((`action` in (_utf8mb4'submit',_utf8mb4'recover',_utf8mb4'query',_utf8mb4'cancel',_utf8mb4'reconcile_delivery',_utf8mb4'callback'))),
  CONSTRAINT `ck_gw_async_outbox_one_root` CHECK (((((((`call_id` is not null) + (`attempt_id` is not null)) + (`async_execution_id` is not null)) + (`callback_receipt_id` is not null)) + (`result_delivery_id` is not null)) = 1)),
  CONSTRAINT `ck_gw_async_outbox_status` CHECK ((`status` in (_utf8mb4'pending',_utf8mb4'dispatching',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'dead_letter')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_availability_sources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `source_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `contract_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `protocol_definition_id` bigint unsigned DEFAULT NULL,
  `legacy_catalog_source_id` bigint unsigned DEFAULT NULL,
  `credential_id` bigint unsigned NOT NULL,
  `credential_purpose_grant_id` bigint unsigned NOT NULL,
  `base_url` varchar(2048) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_timeout_ms` int unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_availability_sources_channel_code` (`channel_id`,`source_code`),
  UNIQUE KEY `uq_gw_availability_sources_legacy` (`legacy_catalog_source_id`),
  KEY `idx_gw_availability_sources_contract_status` (`contract_code`,`status`),
  KEY `fk_gw_availability_sources_grant` (`credential_id`,`credential_purpose_grant_id`),
  KEY `idx_gw_availability_sources_protocol` (`protocol_definition_id`),
  CONSTRAINT `fk_gw_availability_sources_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_availability_sources_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_availability_sources_grant` FOREIGN KEY (`credential_id`, `credential_purpose_grant_id`) REFERENCES `gw_credential_purpose_grants` (`credential_id`, `id`),
  CONSTRAINT `fk_gw_availability_sources_protocol` FOREIGN KEY (`protocol_definition_id`) REFERENCES `gw_protocol_definitions` (`id`),
  CONSTRAINT `ck_gw_availability_sources_contract` CHECK ((`contract_code` in (_utf8mb4'aicost_success_rate_v1',_utf8mb4'cangyuan_channel_monitor_v1',_utf8mb4'declarative_availability_v1'))),
  CONSTRAINT `ck_gw_availability_sources_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'disabled'))),
  CONSTRAINT `ck_gw_availability_sources_timeout` CHECK ((`request_timeout_ms` between 1000 and 120000)),
  CONSTRAINT `ck_gw_availability_sources_version` CHECK ((`config_version` > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_callback_binding_token_aliases` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `async_execution_id` bigint unsigned NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `value_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `encrypted_blob_id` bigint unsigned DEFAULT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active',
  `expires_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `invalidated_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_callback_binding_alias_execution_version` (`async_execution_id`,`hmac_key_version`),
  UNIQUE KEY `uq_gw_callback_binding_alias_match` (`hmac_key_version`,`value_hmac`),
  KEY `fk_gw_callback_binding_alias_blob` (`encrypted_blob_id`),
  KEY `idx_gw_callback_binding_alias_lifecycle` (`status`,`expires_at`,`async_execution_id`),
  CONSTRAINT `fk_gw_callback_binding_alias_blob` FOREIGN KEY (`encrypted_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_callback_binding_alias_execution` FOREIGN KEY (`async_execution_id`) REFERENCES `gw_async_executions` (`id`),
  CONSTRAINT `ck_gw_callback_binding_alias_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'expired',_utf8mb4'revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_callback_deliveries` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned NOT NULL,
  `callback_target_id` bigint unsigned NOT NULL,
  `callback_event_seq` bigint unsigned NOT NULL,
  `encrypted_payload_blob_id` bigint unsigned NOT NULL,
  `payload_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `available_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `attempt_count` bigint unsigned NOT NULL DEFAULT '0',
  `max_attempts` int unsigned NOT NULL DEFAULT '5',
  `lease_owner` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `last_error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `completed_at` datetime(3) DEFAULT NULL,
  `replay_expires_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_callback_deliveries_call_target_seq` (`call_id`,`callback_target_id`,`callback_event_seq`),
  KEY `fk_gw_callback_deliveries_blob` (`encrypted_payload_blob_id`),
  KEY `idx_gw_callback_deliveries_dispatch` (`state`,`available_at`,`id`),
  KEY `idx_gw_callback_deliveries_lease` (`state`,`lease_expires_at`,`id`),
  CONSTRAINT `fk_gw_callback_deliveries_blob` FOREIGN KEY (`encrypted_payload_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_callback_deliveries_target` FOREIGN KEY (`call_id`, `callback_target_id`) REFERENCES `gw_callback_targets` (`call_id`, `id`),
  CONSTRAINT `ck_gw_callback_deliveries_state` CHECK ((`state` in (_utf8mb4'pending',_utf8mb4'sending',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'dead_letter')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_callback_delivery_attempts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `callback_delivery_id` bigint unsigned NOT NULL,
  `attempt_no` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `response_hmac` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `request_bytes_complete` tinyint(1) NOT NULL DEFAULT '0',
  `response_bytes_complete` tinyint(1) NOT NULL DEFAULT '0',
  `http_status` int unsigned DEFAULT NULL,
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_callback_delivery_attempts_delivery_no` (`callback_delivery_id`,`attempt_no`),
  CONSTRAINT `fk_gw_callback_delivery_attempts_delivery` FOREIGN KEY (`callback_delivery_id`) REFERENCES `gw_callback_deliveries` (`id`),
  CONSTRAINT `ck_gw_callback_delivery_attempts_state` CHECK ((`state` in (_utf8mb4'dispatching',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_callback_targets` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `encrypted_config_blob_id` bigint unsigned NOT NULL,
  `target_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `algorithm` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `policy_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_callback_targets_call` (`call_id`),
  UNIQUE KEY `uq_gw_callback_targets_call_id` (`call_id`,`id`),
  KEY `fk_gw_callback_targets_blob` (`encrypted_config_blob_id`),
  CONSTRAINT `fk_gw_callback_targets_blob` FOREIGN KEY (`encrypted_config_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_callback_targets_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_capability_tasks` (
  `resource_id` bigint unsigned NOT NULL,
  `task_no` char(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `progress` tinyint unsigned NOT NULL DEFAULT '0',
  `prompt_preview` varchar(2000) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `parameter_summary` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`resource_id`),
  UNIQUE KEY `uq_gw_capability_tasks_task_no` (`task_no`),
  CONSTRAINT `fk_gw_capability_tasks_resource` FOREIGN KEY (`resource_id`) REFERENCES `gw_api_resources` (`id`),
  CONSTRAINT `ck_gw_capability_tasks_progress` CHECK ((`progress` <= 100))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_discovery_item_endpoints` (
  `snapshot_item_id` bigint unsigned NOT NULL,
  `endpoint_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`snapshot_item_id`,`endpoint_type`),
  CONSTRAINT `fk_gw_catalog_discovery_item_endpoints_item` FOREIGN KEY (`snapshot_item_id`) REFERENCES `gw_catalog_discovery_items` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_discovery_item_groups` (
  `snapshot_item_id` bigint unsigned NOT NULL,
  `group_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`snapshot_item_id`,`group_code`),
  CONSTRAINT `fk_gw_catalog_discovery_item_groups_item` FOREIGN KEY (`snapshot_item_id`) REFERENCES `gw_catalog_discovery_items` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_discovery_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `snapshot_id` bigint unsigned NOT NULL,
  `ordinal` int unsigned NOT NULL,
  `model_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` varchar(1000) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `tags` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `vendor_id` bigint unsigned DEFAULT NULL,
  `provider_quota_type` tinyint unsigned DEFAULT NULL,
  `model_price` decimal(36,18) DEFAULT NULL,
  `model_ratio` decimal(36,18) DEFAULT NULL,
  `completion_ratio` decimal(36,18) DEFAULT NULL,
  `owner_by` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `pricing_version` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `selected_group_enabled` tinyint(1) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_discovery_items_model` (`snapshot_id`,`model_code`),
  UNIQUE KEY `uq_gw_catalog_discovery_items_ordinal` (`snapshot_id`,`ordinal`),
  KEY `idx_gw_catalog_discovery_items_model` (`model_code`,`snapshot_id`),
  CONSTRAINT `fk_gw_catalog_discovery_items_snapshot` FOREIGN KEY (`snapshot_id`) REFERENCES `gw_catalog_discovery_snapshots` (`id`),
  CONSTRAINT `ck_gw_catalog_discovery_items_prices` CHECK ((((`model_price` is null) or (`model_price` >= 0)) and ((`model_ratio` is null) or (`model_ratio` >= 0)) and ((`completion_ratio` is null) or (`completion_ratio` >= 0)))),
  CONSTRAINT `ck_gw_catalog_discovery_items_quota` CHECK (((`provider_quota_type` is null) or (`provider_quota_type` in (0,1))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_discovery_review_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `snapshot_id` bigint unsigned NOT NULL,
  `review_seq` bigint unsigned NOT NULL,
  `decision` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reviewer_user_id` bigint unsigned DEFAULT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_discovery_review_events_seq` (`snapshot_id`,`review_seq`),
  UNIQUE KEY `uq_gw_catalog_discovery_review_events_snapshot_id_id` (`snapshot_id`,`id`),
  KEY `fk_gw_catalog_discovery_review_events_reviewer` (`reviewer_user_id`),
  CONSTRAINT `fk_gw_catalog_discovery_review_events_reviewer` FOREIGN KEY (`reviewer_user_id`) REFERENCES `users` (`id`),
  CONSTRAINT `fk_gw_catalog_discovery_review_events_snapshot` FOREIGN KEY (`snapshot_id`) REFERENCES `gw_catalog_discovery_snapshots` (`id`),
  CONSTRAINT `ck_gw_catalog_discovery_review_events_decision` CHECK ((`decision` in (_utf8mb4'submitted',_utf8mb4'accepted',_utf8mb4'rejected')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_discovery_review_state` (
  `snapshot_id` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `latest_review_event_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`snapshot_id`),
  KEY `fk_gw_catalog_discovery_review_state_event` (`snapshot_id`,`latest_review_event_id`),
  CONSTRAINT `fk_gw_catalog_discovery_review_state_event` FOREIGN KEY (`snapshot_id`, `latest_review_event_id`) REFERENCES `gw_catalog_discovery_review_events` (`snapshot_id`, `id`),
  CONSTRAINT `fk_gw_catalog_discovery_review_state_snapshot` FOREIGN KEY (`snapshot_id`) REFERENCES `gw_catalog_discovery_snapshots` (`id`),
  CONSTRAINT `ck_gw_catalog_discovery_review_state_value` CHECK ((`state` in (_utf8mb4'submitted',_utf8mb4'accepted',_utf8mb4'rejected')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_discovery_snapshots` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `release_source_id` bigint unsigned NOT NULL,
  `control_plane_run_id` bigint unsigned NOT NULL,
  `contract_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `observed_at` datetime(3) NOT NULL,
  `response_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `result_schema_version` int unsigned NOT NULL,
  `item_count` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_discovery_snapshots_run` (`control_plane_run_id`),
  UNIQUE KEY `uq_gw_catalog_discovery_snapshots_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_catalog_discovery_snapshots_source` (`release_id`,`release_source_id`),
  KEY `idx_gw_catalog_discovery_snapshots_source` (`release_source_id`,`id`),
  CONSTRAINT `fk_gw_catalog_discovery_snapshots_run` FOREIGN KEY (`control_plane_run_id`) REFERENCES `gw_control_plane_runs` (`id`),
  CONSTRAINT `fk_gw_catalog_discovery_snapshots_source` FOREIGN KEY (`release_id`, `release_source_id`) REFERENCES `gw_catalog_release_sources` (`release_id`, `id`),
  CONSTRAINT `ck_gw_catalog_discovery_snapshots_contract` CHECK ((`contract_code` in (_utf8mb4'aicost_models_v1',_utf8mb4'aicost_pricing_v1',_utf8mb4'cangyuan_channel_monitor_v1',_utf8mb4'declarative_catalog_v1'))),
  CONSTRAINT `ck_gw_catalog_discovery_snapshots_count` CHECK ((`item_count` <= 10000))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_import_snapshots` (
  `catalog_import_id` bigint unsigned NOT NULL,
  `snapshot_id` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`catalog_import_id`),
  UNIQUE KEY `uq_gw_catalog_import_snapshots_snapshot` (`snapshot_id`),
  CONSTRAINT `fk_gw_catalog_import_snapshots_import` FOREIGN KEY (`catalog_import_id`) REFERENCES `gw_catalog_imports` (`id`),
  CONSTRAINT `fk_gw_catalog_import_snapshots_snapshot` FOREIGN KEY (`snapshot_id`) REFERENCES `gw_catalog_discovery_snapshots` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_imports` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `release_source_id` bigint unsigned NOT NULL,
  `control_plane_run_id` bigint unsigned NOT NULL,
  `snapshot_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `result_schema_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_imports_release_source` (`release_source_id`),
  UNIQUE KEY `uq_gw_catalog_imports_run` (`control_plane_run_id`),
  KEY `fk_gw_catalog_imports_release_source` (`release_id`,`release_source_id`),
  CONSTRAINT `fk_gw_catalog_imports_release_source` FOREIGN KEY (`release_id`, `release_source_id`) REFERENCES `gw_catalog_release_sources` (`release_id`, `id`),
  CONSTRAINT `fk_gw_catalog_imports_run` FOREIGN KEY (`control_plane_run_id`) REFERENCES `gw_control_plane_runs` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_model_names` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `catalog_model_id` bigint unsigned NOT NULL,
  `model_id` bigint unsigned NOT NULL,
  `model_name_id` bigint unsigned NOT NULL,
  `is_primary` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_model_names_release_name` (`release_id`,`model_name_id`),
  UNIQUE KEY `uq_gw_catalog_model_names_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_catalog_model_names_catalog_model` (`release_id`,`catalog_model_id`,`model_id`),
  KEY `fk_gw_catalog_model_names_model_name` (`model_id`,`model_name_id`),
  CONSTRAINT `fk_gw_catalog_model_names_catalog_model` FOREIGN KEY (`release_id`, `catalog_model_id`, `model_id`) REFERENCES `gw_catalog_models` (`release_id`, `id`, `model_id`),
  CONSTRAINT `fk_gw_catalog_model_names_model_name` FOREIGN KEY (`model_id`, `model_name_id`) REFERENCES `gw_model_names` (`model_id`, `id`),
  CONSTRAINT `fk_gw_catalog_model_names_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_models` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `model_id` bigint unsigned NOT NULL,
  `display_name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `description` varchar(1000) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `capability_tags` json DEFAULT NULL,
  `sort_order` int NOT NULL DEFAULT '0',
  `visibility` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_models_release_model` (`release_id`,`model_id`),
  UNIQUE KEY `uq_gw_catalog_models_release_id_id` (`release_id`,`id`),
  UNIQUE KEY `uq_gw_catalog_models_release_id_model_id` (`release_id`,`id`,`model_id`),
  KEY `fk_gw_catalog_models_model` (`model_id`),
  CONSTRAINT `fk_gw_catalog_models_model` FOREIGN KEY (`model_id`) REFERENCES `gw_models` (`id`),
  CONSTRAINT `fk_gw_catalog_models_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_catalog_models_visibility` CHECK ((`visibility` in (_utf8mb4'visible',_utf8mb4'deprecated',_utf8mb4'hidden')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_price_candidate_reviews` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `candidate_id` bigint unsigned NOT NULL,
  `decision` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `unit_code` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `currency_version` int unsigned DEFAULT NULL,
  `rate_evidence_id` bigint unsigned DEFAULT NULL,
  `reviewer_user_id` bigint unsigned NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_price_candidate_reviews_candidate` (`candidate_id`),
  KEY `fk_gw_catalog_price_candidate_reviews_evidence` (`rate_evidence_id`),
  KEY `fk_gw_catalog_price_candidate_reviews_reviewer` (`reviewer_user_id`),
  CONSTRAINT `fk_gw_catalog_price_candidate_reviews_candidate` FOREIGN KEY (`candidate_id`) REFERENCES `gw_catalog_price_candidates` (`id`),
  CONSTRAINT `fk_gw_catalog_price_candidate_reviews_evidence` FOREIGN KEY (`rate_evidence_id`) REFERENCES `gw_rate_evidence` (`id`),
  CONSTRAINT `fk_gw_catalog_price_candidate_reviews_reviewer` FOREIGN KEY (`reviewer_user_id`) REFERENCES `users` (`id`),
  CONSTRAINT `ck_gw_catalog_price_candidate_reviews_decision` CHECK ((((`decision` = _utf8mb4'confirmed') and (`unit_code` is not null) and (`currency_code` is not null) and (`currency_version` is not null) and (`rate_evidence_id` is not null)) or ((`decision` = _utf8mb4'dismissed') and (`unit_code` is null) and (`currency_code` is null) and (`currency_version` is null) and (`rate_evidence_id` is null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_price_candidates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `snapshot_item_id` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_price_candidates_item` (`snapshot_item_id`),
  CONSTRAINT `fk_gw_catalog_price_candidates_item` FOREIGN KEY (`snapshot_item_id`) REFERENCES `gw_catalog_discovery_items` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_readiness` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `deployment_generation_id` bigint unsigned NOT NULL,
  `deployment_member_id` bigint unsigned NOT NULL,
  `release_id` bigint unsigned NOT NULL,
  `content_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `semantic_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `adapter_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `heartbeat_at` datetime(3) NOT NULL,
  `expires_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_readiness_member_release` (`deployment_member_id`,`release_id`),
  KEY `fk_gw_catalog_readiness_member` (`deployment_generation_id`,`deployment_member_id`),
  KEY `fk_gw_catalog_readiness_release` (`release_id`),
  CONSTRAINT `fk_gw_catalog_readiness_member` FOREIGN KEY (`deployment_generation_id`, `deployment_member_id`) REFERENCES `gw_deployment_members` (`deployment_generation_id`, `id`),
  CONSTRAINT `fk_gw_catalog_readiness_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_catalog_readiness_status` CHECK ((`status` in (_utf8mb4'ready',_utf8mb4'failed',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_release_sources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `catalog_source_id` bigint unsigned NOT NULL,
  `source_state_version` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_release_sources_release_source` (`release_id`,`catalog_source_id`),
  UNIQUE KEY `uq_gw_catalog_release_sources_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_catalog_release_sources_source` (`catalog_source_id`),
  CONSTRAINT `fk_gw_catalog_release_sources_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `fk_gw_catalog_release_sources_source` FOREIGN KEY (`catalog_source_id`) REFERENCES `gw_catalog_sources` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_release_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_gw_catalog_release_state_events_release` (`release_id`),
  CONSTRAINT `fk_gw_catalog_release_state_events_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_catalog_release_state_events_state` CHECK ((`new_state` in (_utf8mb4'draft',_utf8mb4'published',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_releases` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_no` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `serialization_version` int unsigned NOT NULL,
  `content_hash_algorithm` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `content_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `semantic_version` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `semantic_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reviewed_by` bigint unsigned DEFAULT NULL,
  `published_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_releases_release_no` (`release_no`),
  UNIQUE KEY `uq_gw_catalog_releases_content_hash` (`content_hash`),
  CONSTRAINT `ck_gw_catalog_releases_hash_algorithm` CHECK ((`content_hash_algorithm` = _utf8mb4'sha256')),
  CONSTRAINT `ck_gw_catalog_releases_status` CHECK ((`status` in (_utf8mb4'draft',_utf8mb4'published',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_runtime_state` (
  `id` tinyint unsigned NOT NULL,
  `active_release_id` bigint unsigned DEFAULT NULL,
  `active_deployment_generation_id` bigint unsigned DEFAULT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_gw_catalog_runtime_state_release` (`active_release_id`),
  KEY `fk_gw_catalog_runtime_state_deployment` (`active_deployment_generation_id`),
  CONSTRAINT `fk_gw_catalog_runtime_state_deployment` FOREIGN KEY (`active_deployment_generation_id`) REFERENCES `gw_deployment_generations` (`id`),
  CONSTRAINT `fk_gw_catalog_runtime_state_release` FOREIGN KEY (`active_release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_catalog_runtime_state_singleton` CHECK ((`id` = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_runtime_state_validate_insert`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_runtime_state_validate_insert` BEFORE INSERT ON `gw_catalog_runtime_state` FOR EACH ROW BEGIN
    IF NEW.`id` <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'gateway runtime state must be singleton';
    END IF;
    IF NEW.`active_release_id` IS NOT NULL AND
       (SELECT COUNT(*) FROM `gw_catalog_releases` WHERE `id`=NEW.`active_release_id` AND `status`='published') <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'active catalog release must be published';
    END IF;
    IF NEW.`active_deployment_generation_id` IS NOT NULL AND
       (SELECT COUNT(*) FROM `gw_deployment_generations` WHERE `id`=NEW.`active_deployment_generation_id` AND `status`='active') <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'active deployment generation must be active';
    END IF;
END */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_runtime_state_validate_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_runtime_state_validate_update` BEFORE UPDATE ON `gw_catalog_runtime_state` FOR EACH ROW BEGIN
    IF NEW.`id` <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'gateway runtime state must be singleton';
    END IF;
    IF NEW.`active_release_id` IS NOT NULL AND
       (SELECT COUNT(*) FROM `gw_catalog_releases` WHERE `id`=NEW.`active_release_id` AND `status`='published') <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'active catalog release must be published';
    END IF;
    IF NEW.`active_deployment_generation_id` IS NOT NULL AND
       (SELECT COUNT(*) FROM `gw_deployment_generations` WHERE `id`=NEW.`active_deployment_generation_id` AND `status`='active') <> 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'active deployment generation must be active';
    END IF;
END */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_source_profiles` (
  `catalog_source_id` bigint unsigned NOT NULL,
  `contract_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `protocol_definition_id` bigint unsigned DEFAULT NULL,
  `base_url` varchar(2048) COLLATE utf8mb4_unicode_ci NOT NULL,
  `external_group` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_timeout_ms` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`catalog_source_id`),
  KEY `idx_gw_catalog_source_profiles_protocol` (`protocol_definition_id`),
  CONSTRAINT `fk_gw_catalog_source_profiles_protocol` FOREIGN KEY (`protocol_definition_id`) REFERENCES `gw_protocol_definitions` (`id`),
  CONSTRAINT `fk_gw_catalog_source_profiles_source` FOREIGN KEY (`catalog_source_id`) REFERENCES `gw_catalog_sources` (`id`),
  CONSTRAINT `ck_gw_catalog_source_profiles_contract` CHECK ((`contract_code` in (_utf8mb4'aicost_models_v1',_utf8mb4'aicost_pricing_v1',_utf8mb4'cangyuan_channel_monitor_v1',_utf8mb4'declarative_catalog_v1'))),
  CONSTRAINT `ck_gw_catalog_source_profiles_timeout` CHECK ((`request_timeout_ms` between 1000 and 120000))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_source_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `catalog_source_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_source_state_events_version` (`catalog_source_id`,`state_version`),
  CONSTRAINT `fk_gw_catalog_source_state_events_source` FOREIGN KEY (`catalog_source_id`) REFERENCES `gw_catalog_sources` (`id`),
  CONSTRAINT `ck_gw_catalog_source_state_events_state` CHECK ((`new_state` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_catalog_sources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `source_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `adapter_implementation_id` bigint unsigned NOT NULL,
  `credential_id` bigint unsigned NOT NULL,
  `credential_purpose_grant_id` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `nonterminal_run_count` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_catalog_sources_channel_code` (`channel_id`,`source_code`),
  UNIQUE KEY `uq_gw_catalog_sources_channel_id_id` (`channel_id`,`id`),
  KEY `fk_gw_catalog_sources_adapter` (`adapter_implementation_id`),
  KEY `fk_gw_catalog_sources_grant` (`credential_id`,`credential_purpose_grant_id`),
  CONSTRAINT `fk_gw_catalog_sources_adapter` FOREIGN KEY (`adapter_implementation_id`) REFERENCES `gw_adapter_implementations` (`id`),
  CONSTRAINT `fk_gw_catalog_sources_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_catalog_sources_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_catalog_sources_grant` FOREIGN KEY (`credential_id`, `credential_purpose_grant_id`) REFERENCES `gw_credential_purpose_grants` (`credential_id`, `id`),
  CONSTRAINT `ck_gw_catalog_sources_runs` CHECK ((`nonterminal_run_count` >= 0)),
  CONSTRAINT `ck_gw_catalog_sources_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_channel_keys` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `api_key` text COLLATE utf8mb4_unicode_ci NOT NULL,
  `weight` bigint DEFAULT '10',
  `status` tinyint DEFAULT '1',
  `max_conc` bigint DEFAULT '0',
  `current_conc` bigint DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_gw_channel_keys_channel_id` (`channel_id`),
  KEY `idx_gw_channel_keys_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_channel_request_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `attempt_id` bigint unsigned DEFAULT NULL,
  `control_plane_run_id` bigint unsigned DEFAULT NULL,
  `request_seq` bigint unsigned NOT NULL,
  `action` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_mapping_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_bytes_hmac` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `response_bytes_hmac` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `request_bytes_complete` tinyint(1) NOT NULL DEFAULT '0',
  `response_bytes_complete` tinyint(1) NOT NULL DEFAULT '0',
  `diagnostic_blob_id` bigint unsigned DEFAULT NULL,
  `request_payload_blob_id` bigint unsigned DEFAULT NULL,
  `response_payload_blob_id` bigint unsigned DEFAULT NULL,
  `http_status` int unsigned DEFAULT NULL,
  `duration_ms` bigint unsigned DEFAULT NULL,
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  `completed_at` datetime(3) DEFAULT NULL,
  `async_execution_id` bigint unsigned DEFAULT NULL,
  `result_delivery_id` bigint unsigned DEFAULT NULL,
  `outbox_id` bigint unsigned DEFAULT NULL,
  `outbox_attempt_count` bigint unsigned DEFAULT NULL,
  `submitted_async_attempt_id` bigint unsigned GENERATED ALWAYS AS ((case when ((`outbox_id` is not null) and (`action` = _utf8mb4'submit') and (`status` <> _utf8mb4'not_sent')) then `attempt_id` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_request_logs_attempt_seq` (`attempt_id`,`request_seq`),
  UNIQUE KEY `uq_gw_request_logs_control_plane_seq` (`control_plane_run_id`,`request_seq`),
  UNIQUE KEY `uq_gw_request_outbox_lease` (`outbox_id`,`outbox_attempt_count`),
  UNIQUE KEY `uq_gw_request_async_submit` (`submitted_async_attempt_id`),
  KEY `fk_gw_request_logs_diagnostic_blob` (`diagnostic_blob_id`),
  KEY `fk_gw_request_async_attempt` (`async_execution_id`,`attempt_id`),
  KEY `fk_gw_request_outbox_async` (`outbox_id`,`async_execution_id`),
  KEY `fk_gw_request_result_delivery_attempt` (`result_delivery_id`,`attempt_id`),
  KEY `fk_gw_request_outbox_delivery` (`outbox_id`,`result_delivery_id`),
  KEY `fk_gw_request_logs_request_payload_blob` (`request_payload_blob_id`),
  KEY `fk_gw_request_logs_response_payload_blob` (`response_payload_blob_id`),
  CONSTRAINT `fk_gw_request_async_attempt` FOREIGN KEY (`async_execution_id`, `attempt_id`) REFERENCES `gw_async_executions` (`id`, `attempt_id`),
  CONSTRAINT `fk_gw_request_logs_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_request_logs_diagnostic_blob` FOREIGN KEY (`diagnostic_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_request_logs_request_payload_blob` FOREIGN KEY (`request_payload_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_request_logs_response_payload_blob` FOREIGN KEY (`response_payload_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_request_outbox_async` FOREIGN KEY (`outbox_id`, `async_execution_id`) REFERENCES `gw_async_outbox` (`id`, `async_execution_id`),
  CONSTRAINT `fk_gw_request_outbox_delivery` FOREIGN KEY (`outbox_id`, `result_delivery_id`) REFERENCES `gw_async_outbox` (`id`, `result_delivery_id`),
  CONSTRAINT `fk_gw_request_result_delivery_attempt` FOREIGN KEY (`result_delivery_id`, `attempt_id`) REFERENCES `gw_result_deliveries` (`id`, `attempt_id`),
  CONSTRAINT `ck_gw_request_async_dispatch` CHECK ((((`outbox_id` is null) and (`async_execution_id` is null) and (`result_delivery_id` is null) and (`outbox_attempt_count` is null)) or ((`outbox_id` is not null) and (`attempt_id` is not null) and (`outbox_attempt_count` is not null) and (`outbox_attempt_count` > 0) and (((`async_execution_id` is not null) and (`result_delivery_id` is null)) or ((`async_execution_id` is null) and (`result_delivery_id` is not null)))))),
  CONSTRAINT `ck_gw_request_logs_action` CHECK ((`action` in (_utf8mb4'submit',_utf8mb4'recover',_utf8mb4'query',_utf8mb4'cancel',_utf8mb4'reconcile_delivery',_utf8mb4'named_action',_utf8mb4'result_fetch',_utf8mb4'catalog_discovery',_utf8mb4'entitlement_probe',_utf8mb4'commercial_check'))),
  CONSTRAINT `ck_gw_request_logs_one_root` CHECK ((((`attempt_id` is not null) and (`control_plane_run_id` is null)) or ((`attempt_id` is null) and (`control_plane_run_id` is not null)))),
  CONSTRAINT `ck_gw_request_logs_status` CHECK ((`status` in (_utf8mb4'prepared',_utf8mb4'dispatching',_utf8mb4'not_sent',_utf8mb4'sent',_utf8mb4'response_recorded',_utf8mb4'unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_channel_transports` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `channel_id` bigint unsigned NOT NULL,
  `adapter_implementation_id` bigint unsigned NOT NULL,
  `transport_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `base_url` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL,
  `protocol` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_method` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_path` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL,
  `auth_scheme` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `execution_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_compatibility_fingerprint` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `timeout_ms` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_channel_transports_release_code` (`release_id`,`transport_code`),
  UNIQUE KEY `uq_gw_channel_transports_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_channel_transports_channel` (`channel_id`),
  KEY `fk_gw_channel_transports_adapter` (`adapter_implementation_id`),
  CONSTRAINT `fk_gw_channel_transports_adapter` FOREIGN KEY (`adapter_implementation_id`) REFERENCES `gw_adapter_implementations` (`id`),
  CONSTRAINT `fk_gw_channel_transports_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_channel_transports_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_channel_transports_timeout` CHECK ((`timeout_ms` > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_channels` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
  `protocol` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'openai',
  `base_url` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `extra_headers` json DEFAULT NULL,
  `config` json DEFAULT NULL,
  `status` tinyint DEFAULT '1',
  `sort` bigint DEFAULT '0',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_gw_channels_sort` (`sort`),
  KEY `idx_gw_channels_created_at` (`created_at`),
  KEY `idx_gw_channels_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_commercial_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `commercial_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `latest_event_id` bigint unsigned NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_commercial_state_fingerprint` (`commercial_fingerprint`),
  KEY `fk_gw_commercial_state_event_v2` (`latest_event_id`,`commercial_fingerprint`),
  CONSTRAINT `fk_gw_commercial_state_event_v2` FOREIGN KEY (`latest_event_id`, `commercial_fingerprint`) REFERENCES `gw_commercial_validation_events` (`id`, `commercial_fingerprint`),
  CONSTRAINT `ck_gw_commercial_state_state` CHECK ((`state` in (_utf8mb4'valid',_utf8mb4'drift',_utf8mb4'unknown',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_commercial_validation_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `commercial_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `validation_seq` bigint unsigned NOT NULL,
  `control_plane_run_id` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `observation_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `checked_at` datetime(3) NOT NULL,
  `valid_until` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_commercial_validation_seq` (`commercial_fingerprint`,`validation_seq`),
  UNIQUE KEY `uq_gw_commercial_validation_identity` (`id`,`commercial_fingerprint`),
  KEY `fk_gw_commercial_validation_run` (`control_plane_run_id`),
  CONSTRAINT `fk_gw_commercial_validation_run` FOREIGN KEY (`control_plane_run_id`) REFERENCES `gw_control_plane_runs` (`id`),
  CONSTRAINT `ck_gw_commercial_validation_state` CHECK ((`state` in (_utf8mb4'valid',_utf8mb4'drift',_utf8mb4'unknown',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_control_plane_run_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `control_plane_run_id` bigint unsigned NOT NULL,
  `event_seq` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_control_plane_run_events_seq` (`control_plane_run_id`,`event_seq`),
  CONSTRAINT `fk_gw_control_plane_run_events_run` FOREIGN KEY (`control_plane_run_id`) REFERENCES `gw_control_plane_runs` (`id`),
  CONSTRAINT `ck_gw_control_plane_run_events_new_state` CHECK ((`new_state` in (_utf8mb4'scheduled',_utf8mb4'running',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'manual_review'))),
  CONSTRAINT `ck_gw_control_plane_run_events_old_state` CHECK (((`old_state` is null) or (`old_state` in (_utf8mb4'scheduled',_utf8mb4'running',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'manual_review'))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_control_plane_runs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `action` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `catalog_release_source_id` bigint unsigned DEFAULT NULL,
  `credential_id` bigint unsigned DEFAULT NULL,
  `offering_id` bigint unsigned DEFAULT NULL,
  `target_fingerprint` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `target_config_version` bigint unsigned DEFAULT NULL,
  `auth_credential_version_id` bigint unsigned NOT NULL,
  `auth_purpose_grant_id` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `next_action_at` datetime(3) DEFAULT NULL,
  `lease_owner` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_control_plane_runs_catalog_target` (`catalog_release_source_id`,`action`,`target_fingerprint`,`target_config_version`),
  KEY `fk_gw_control_plane_runs_offering` (`offering_id`),
  KEY `fk_gw_control_plane_runs_auth_version` (`auth_credential_version_id`),
  KEY `fk_gw_control_plane_runs_auth_grant` (`credential_id`,`auth_purpose_grant_id`),
  CONSTRAINT `fk_gw_control_plane_runs_auth_grant` FOREIGN KEY (`credential_id`, `auth_purpose_grant_id`) REFERENCES `gw_credential_purpose_grants` (`credential_id`, `id`),
  CONSTRAINT `fk_gw_control_plane_runs_auth_version` FOREIGN KEY (`auth_credential_version_id`) REFERENCES `gw_credential_versions` (`id`),
  CONSTRAINT `fk_gw_control_plane_runs_catalog_source` FOREIGN KEY (`catalog_release_source_id`) REFERENCES `gw_catalog_release_sources` (`id`),
  CONSTRAINT `fk_gw_control_plane_runs_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_control_plane_runs_offering` FOREIGN KEY (`offering_id`) REFERENCES `gw_offerings` (`id`),
  CONSTRAINT `ck_gw_control_plane_runs_action` CHECK ((`action` in (_utf8mb4'catalog_discovery',_utf8mb4'entitlement_probe',_utf8mb4'commercial_check'))),
  CONSTRAINT `ck_gw_control_plane_runs_state` CHECK ((`state` in (_utf8mb4'scheduled',_utf8mb4'running',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'manual_review'))),
  CONSTRAINT `ck_gw_control_plane_runs_target` CHECK ((((`action` = _utf8mb4'catalog_discovery') and (`catalog_release_source_id` is not null) and (`credential_id` is not null) and (`offering_id` is null)) or ((`action` <> _utf8mb4'catalog_discovery') and (`catalog_release_source_id` is null) and ((`credential_id` is not null) or (`offering_id` is not null)))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_cost_plans` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `offering_id` bigint unsigned NOT NULL,
  `plan_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_cost_plans_release_offering_code` (`release_id`,`offering_id`,`plan_code`),
  UNIQUE KEY `uq_gw_cost_plans_release_id_id` (`release_id`,`id`),
  UNIQUE KEY `uq_gw_cost_plans_release_offering_id` (`release_id`,`offering_id`,`id`),
  CONSTRAINT `fk_gw_cost_plans_offering` FOREIGN KEY (`release_id`, `offering_id`) REFERENCES `gw_offerings` (`release_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_cost_rates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `cost_plan_id` bigint unsigned NOT NULL,
  `rate_evidence_review_event_id` bigint unsigned NOT NULL,
  `unit_code` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `unit_price` decimal(38,18) NOT NULL,
  `pricing_mode` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT 'flat',
  `pricing_expr` text CHARACTER SET utf8mb4 COLLATE utf8mb4_bin,
  `max_price` decimal(38,18) DEFAULT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `component_code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `quantity_source` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `charge_event` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `unit_scale` tinyint unsigned NOT NULL DEFAULT '0',
  `quantity_step` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `max_quantity` decimal(38,18) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_cost_rates_component` (`release_id`,`cost_plan_id`,`component_code`),
  UNIQUE KEY `uq_gw_cost_rates_meter` (`release_id`,`cost_plan_id`,`charge_event`,`quantity_source`),
  KEY `fk_gw_cost_rates_review_event` (`rate_evidence_review_event_id`),
  KEY `fk_gw_cost_rates_currency` (`currency_code`,`currency_version`),
  CONSTRAINT `fk_gw_cost_rates_currency` FOREIGN KEY (`currency_code`, `currency_version`) REFERENCES `billing_currency_definitions` (`currency_code`, `definition_version`),
  CONSTRAINT `fk_gw_cost_rates_plan` FOREIGN KEY (`release_id`, `cost_plan_id`) REFERENCES `gw_cost_plans` (`release_id`, `id`),
  CONSTRAINT `fk_gw_cost_rates_review_event` FOREIGN KEY (`rate_evidence_review_event_id`) REFERENCES `gw_rate_evidence_review_events` (`id`),
  CONSTRAINT `ck_gw_cost_rates_charge_event` CHECK ((`charge_event` in (_utf8mb4'call.succeeded',_utf8mb4'call.failed',_utf8mb4'call.cancelled',_utf8mb4'provider.accepted',_utf8mb4'delivery.ready',_utf8mb4'delivery.failed'))),
  CONSTRAINT `ck_gw_cost_rates_expr_pairing` CHECK ((((`pricing_mode` = _utf8mb4'expression') and (`pricing_expr` is not null) and (`max_price` is not null) and (`max_price` >= 0)) or ((`pricing_mode` = _utf8mb4'flat') and (`pricing_expr` is null) and (`max_price` is null)))),
  CONSTRAINT `ck_gw_cost_rates_max_quantity` CHECK ((`max_quantity` > 0)),
  CONSTRAINT `ck_gw_cost_rates_price` CHECK ((`unit_price` >= 0)),
  CONSTRAINT `ck_gw_cost_rates_pricing_mode` CHECK ((`pricing_mode` in (_utf8mb4'flat',_utf8mb4'expression'))),
  CONSTRAINT `ck_gw_cost_rates_quantity_source` CHECK ((`quantity_source` in (_utf8mb4'one',_utf8mb4'usage.input_tokens',_utf8mb4'usage.output_tokens',_utf8mb4'usage.uncached_input_tokens',_utf8mb4'usage.cached_input_tokens',_utf8mb4'usage.cache_write_tokens',_utf8mb4'usage.input_no_cache_write',_utf8mb4'usage.input_no_cache',_utf8mb4'request.seconds',_utf8mb4'result.seconds',_utf8mb4'request.images',_utf8mb4'result.images',_utf8mb4'result.videos',_utf8mb4'result.megapixels'))),
  CONSTRAINT `ck_gw_cost_rates_quantity_step` CHECK ((`quantity_step` >= 0)),
  CONSTRAINT `ck_gw_cost_rates_unit_scale` CHECK ((`unit_scale` <= 12))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_entitlement_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_id` bigint unsigned NOT NULL,
  `credential_version_id` bigint unsigned NOT NULL,
  `entitlement_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `latest_event_id` bigint unsigned NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_entitlement_state_key` (`credential_id`,`credential_version_id`,`entitlement_fingerprint`),
  KEY `fk_gw_credential_entitlement_state_event_v2` (`latest_event_id`,`credential_id`,`credential_version_id`,`entitlement_fingerprint`),
  CONSTRAINT `fk_gw_credential_entitlement_state_event_v2` FOREIGN KEY (`latest_event_id`, `credential_id`, `credential_version_id`, `entitlement_fingerprint`) REFERENCES `gw_credential_validation_events` (`id`, `credential_id`, `credential_version_id`, `entitlement_fingerprint`),
  CONSTRAINT `ck_gw_credential_entitlement_state_state` CHECK ((`state` in (_utf8mb4'valid',_utf8mb4'drift',_utf8mb4'unknown',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_fingerprints` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `secret_identity_id` bigint unsigned NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `secret_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_fingerprints_identity_version` (`secret_identity_id`,`hmac_key_version`),
  UNIQUE KEY `uq_gw_credential_fingerprints_hmac` (`hmac_key_version`,`secret_hmac`),
  CONSTRAINT `fk_gw_credential_fingerprints_identity` FOREIGN KEY (`secret_identity_id`) REFERENCES `gw_credential_secret_identities` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_pool_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_pool_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_pool_state_events_version` (`credential_pool_id`,`state_version`),
  CONSTRAINT `fk_gw_pool_state_events_pool` FOREIGN KEY (`credential_pool_id`) REFERENCES `gw_credential_pools` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_pools` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `pool_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `display_name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `cost_group_ratio` decimal(18,8) NOT NULL DEFAULT '1.00000000',
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `request_limit` bigint unsigned DEFAULT NULL,
  `task_limit` bigint unsigned DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_pools_channel_code` (`channel_id`,`pool_code`),
  UNIQUE KEY `uq_gw_credential_pools_channel_id_id` (`channel_id`,`id`),
  CONSTRAINT `fk_gw_credential_pools_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `ck_gw_credential_pools_cost_group_ratio` CHECK ((`cost_group_ratio` > 0)),
  CONSTRAINT `ck_gw_credential_pools_limits` CHECK ((((`request_limit` is null) or (`request_limit` > 0)) and ((`task_limit` is null) or (`task_limit` > 0)))),
  CONSTRAINT `ck_gw_credential_pools_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_purpose_grant_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `purpose_grant_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_grant_events_version` (`purpose_grant_id`,`state_version`),
  CONSTRAINT `fk_gw_credential_grant_events_grant` FOREIGN KEY (`purpose_grant_id`) REFERENCES `gw_credential_purpose_grants` (`id`),
  CONSTRAINT `ck_gw_credential_grant_events_state` CHECK ((`new_state` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_purpose_grant_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `grant_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_grant_state_events_version` (`grant_id`,`state_version`),
  CONSTRAINT `fk_gw_grant_state_events_grant` FOREIGN KEY (`grant_id`) REFERENCES `gw_credential_purpose_grants` (`id`),
  CONSTRAINT `ck_gw_grant_state_events_state` CHECK ((`new_state` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_purpose_grants` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_id` bigint unsigned NOT NULL,
  `purpose` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `grant_seq` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `revoked_at` datetime(3) DEFAULT NULL,
  `active_grant_id` bigint unsigned GENERATED ALWAYS AS ((case when (`status` = _utf8mb4'active') then `credential_id` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_grants_seq` (`credential_id`,`purpose`,`grant_seq`),
  UNIQUE KEY `uq_gw_credential_grants_credential_id` (`credential_id`,`id`),
  UNIQUE KEY `uq_gw_credential_grants_purpose_id` (`credential_id`,`id`,`purpose`),
  UNIQUE KEY `uq_gw_credential_grants_one_active` (`active_grant_id`,`purpose`),
  CONSTRAINT `fk_gw_credential_grants_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `ck_gw_credential_grants_purpose` CHECK ((`purpose` in (_utf8mb4'execution',_utf8mb4'catalog_discovery',_utf8mb4'availability_read',_utf8mb4'upstream_callback_verify'))),
  CONSTRAINT `ck_gw_credential_grants_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_secret_identities` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `secret_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_secret_identities_channel_hmac` (`channel_id`,`hmac_key_version`,`secret_hmac`),
  UNIQUE KEY `uq_gw_secret_identities_channel_id_id` (`channel_id`,`id`),
  CONSTRAINT `fk_gw_secret_identities_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `ck_gw_secret_identities_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'security_revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_secret_identity_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `secret_identity_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `new_state` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_secret_identity_events_version` (`secret_identity_id`,`state_version`),
  CONSTRAINT `fk_gw_secret_identity_events_identity` FOREIGN KEY (`secret_identity_id`) REFERENCES `gw_credential_secret_identities` (`id`),
  CONSTRAINT `ck_gw_secret_identity_events_state` CHECK ((`new_state` in (_utf8mb4'active',_utf8mb4'security_revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_slots` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_id` bigint unsigned NOT NULL,
  `credential_pool_id` bigint unsigned NOT NULL,
  `scope` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_log_id` bigint unsigned DEFAULT NULL,
  `attempt_id` bigint unsigned DEFAULT NULL,
  `state` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `acquired_at` datetime(3) NOT NULL,
  `released_at` datetime(3) DEFAULT NULL,
  `active_request_id` bigint unsigned GENERATED ALWAYS AS ((case when ((`scope` = _utf8mb4'request') and (`state` in (_utf8mb4'active',_utf8mb4'recovery_required'))) then `request_log_id` else NULL end)) STORED,
  `active_attempt_id` bigint unsigned GENERATED ALWAYS AS ((case when ((`scope` = _utf8mb4'task') and (`state` in (_utf8mb4'active',_utf8mb4'recovery_required'))) then `attempt_id` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_slots_active_request` (`active_request_id`),
  UNIQUE KEY `uq_gw_credential_slots_active_attempt` (`active_attempt_id`),
  KEY `fk_gw_credential_slots_pool_credential` (`credential_pool_id`,`credential_id`),
  KEY `fk_gw_credential_slots_request_log` (`request_log_id`),
  KEY `fk_gw_credential_slots_attempt` (`attempt_id`),
  KEY `idx_gw_slots_pool_scope_state` (`credential_pool_id`,`scope`,`state`),
  KEY `idx_gw_slots_credential_scope_state` (`credential_id`,`scope`,`state`),
  CONSTRAINT `fk_gw_credential_slots_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_credential_slots_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_credential_slots_pool_credential` FOREIGN KEY (`credential_pool_id`, `credential_id`) REFERENCES `gw_credentials` (`credential_pool_id`, `id`),
  CONSTRAINT `fk_gw_credential_slots_request_log` FOREIGN KEY (`request_log_id`) REFERENCES `gw_channel_request_logs` (`id`),
  CONSTRAINT `ck_gw_credential_slots_recovery_scope` CHECK (((`state` <> _utf8mb4'recovery_required') or (`scope` = _utf8mb4'task'))),
  CONSTRAINT `ck_gw_credential_slots_scope` CHECK ((((`scope` = _utf8mb4'request') and (`request_log_id` is not null) and (`attempt_id` is null)) or ((`scope` = _utf8mb4'task') and (`request_log_id` is null) and (`attempt_id` is not null)))),
  CONSTRAINT `ck_gw_credential_slots_state` CHECK ((`state` in (_utf8mb4'active',_utf8mb4'recovery_required',_utf8mb4'released')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_state_events_version` (`credential_id`,`state_version`),
  CONSTRAINT `fk_gw_credential_state_events_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_validation_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_id` bigint unsigned NOT NULL,
  `credential_version_id` bigint unsigned NOT NULL,
  `entitlement_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `validation_seq` bigint unsigned NOT NULL,
  `control_plane_run_id` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `response_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `checked_at` datetime(3) NOT NULL,
  `valid_until` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_validation_seq` (`credential_id`,`credential_version_id`,`entitlement_fingerprint`,`validation_seq`),
  UNIQUE KEY `uq_gw_credential_validation_identity` (`id`,`credential_id`,`credential_version_id`,`entitlement_fingerprint`),
  KEY `fk_gw_credential_validation_run` (`control_plane_run_id`),
  CONSTRAINT `fk_gw_credential_validation_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_credential_validation_run` FOREIGN KEY (`control_plane_run_id`) REFERENCES `gw_control_plane_runs` (`id`),
  CONSTRAINT `fk_gw_credential_validation_version` FOREIGN KEY (`credential_id`, `credential_version_id`) REFERENCES `gw_credential_versions` (`credential_id`, `id`),
  CONSTRAINT `ck_gw_credential_validation_state` CHECK ((`state` in (_utf8mb4'valid',_utf8mb4'drift',_utf8mb4'unknown',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_version_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_version_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_version_events_version` (`credential_version_id`,`state_version`),
  CONSTRAINT `fk_gw_credential_version_events_version` FOREIGN KEY (`credential_version_id`) REFERENCES `gw_credential_versions` (`id`),
  CONSTRAINT `ck_gw_credential_version_events_state` CHECK ((`new_state` in (_utf8mb4'preparing',_utf8mb4'active',_utf8mb4'superseded',_utf8mb4'retired',_utf8mb4'security_revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_version_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `credential_version_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_version_state_events_version` (`credential_version_id`,`state_version`),
  CONSTRAINT `fk_gw_credential_version_state_events_version` FOREIGN KEY (`credential_version_id`) REFERENCES `gw_credential_versions` (`id`),
  CONSTRAINT `ck_gw_credential_version_state_events_state` CHECK ((`new_state` in (_utf8mb4'preparing',_utf8mb4'active',_utf8mb4'superseded',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credential_versions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `credential_id` bigint unsigned NOT NULL,
  `secret_identity_id` bigint unsigned NOT NULL,
  `version_no` bigint unsigned NOT NULL,
  `encrypted_blob_id` bigint unsigned DEFAULT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `valid_until` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `retired_at` datetime(3) DEFAULT NULL,
  `active_credential_id` bigint unsigned GENERATED ALWAYS AS ((case when (`status` = _utf8mb4'active') then `credential_id` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credential_versions_credential_version` (`credential_id`,`version_no`),
  UNIQUE KEY `uq_gw_credential_versions_credential_id_id` (`credential_id`,`id`),
  UNIQUE KEY `uq_gw_credential_versions_channel_id_id` (`channel_id`,`id`),
  UNIQUE KEY `uq_gw_credential_versions_blob` (`encrypted_blob_id`),
  UNIQUE KEY `uq_gw_credential_versions_one_active` (`active_credential_id`),
  KEY `fk_gw_credential_versions_credential` (`channel_id`,`credential_id`),
  KEY `fk_gw_credential_versions_secret_identity` (`credential_id`,`secret_identity_id`),
  CONSTRAINT `fk_gw_credential_versions_blob` FOREIGN KEY (`encrypted_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_credential_versions_credential` FOREIGN KEY (`channel_id`, `credential_id`) REFERENCES `gw_credentials` (`channel_id`, `id`),
  CONSTRAINT `fk_gw_credential_versions_secret_identity` FOREIGN KEY (`credential_id`, `secret_identity_id`) REFERENCES `gw_credentials` (`id`, `secret_identity_id`),
  CONSTRAINT `ck_gw_credential_versions_status` CHECK ((`status` in (_utf8mb4'preparing',_utf8mb4'active',_utf8mb4'superseded',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_credentials` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `credential_pool_id` bigint unsigned DEFAULT NULL,
  `secret_identity_id` bigint unsigned NOT NULL,
  `current_version_id` bigint unsigned DEFAULT NULL,
  `credential_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `secret` text COLLATE utf8mb4_unicode_ci,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `request_limit` bigint unsigned DEFAULT NULL,
  `task_limit` bigint unsigned DEFAULT NULL,
  `weight` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_credentials_channel_code` (`channel_id`,`credential_code`),
  UNIQUE KEY `uq_gw_credentials_channel_id_id` (`channel_id`,`id`),
  UNIQUE KEY `uq_gw_credentials_id_secret_identity` (`id`,`secret_identity_id`),
  UNIQUE KEY `uq_gw_credentials_secret_identity` (`secret_identity_id`),
  UNIQUE KEY `uq_gw_credentials_pool_id` (`credential_pool_id`,`id`),
  KEY `fk_gw_credentials_pool` (`channel_id`,`credential_pool_id`),
  KEY `fk_gw_credentials_secret_identity` (`channel_id`,`secret_identity_id`),
  CONSTRAINT `fk_gw_credentials_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_credentials_pool` FOREIGN KEY (`channel_id`, `credential_pool_id`) REFERENCES `gw_credential_pools` (`channel_id`, `id`),
  CONSTRAINT `fk_gw_credentials_secret_identity` FOREIGN KEY (`channel_id`, `secret_identity_id`) REFERENCES `gw_credential_secret_identities` (`channel_id`, `id`),
  CONSTRAINT `ck_gw_credentials_limits` CHECK ((((`request_limit` is null) or (`request_limit` > 0)) and ((`task_limit` is null) or (`task_limit` > 0)) and (`weight` > 0))),
  CONSTRAINT `ck_gw_credentials_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_deployment_generations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `generation_no` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `semantic_version` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `semantic_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `member_frozen_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_deployment_generations_no` (`generation_no`),
  CONSTRAINT `ck_gw_deployment_generations_status` CHECK ((`status` in (_utf8mb4'preparing',_utf8mb4'active',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_deployment_members` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `deployment_generation_id` bigint unsigned NOT NULL,
  `instance_id` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `role` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_deployment_members_generation_instance_role` (`deployment_generation_id`,`instance_id`,`role`),
  UNIQUE KEY `uq_gw_deployment_members_generation_id` (`deployment_generation_id`,`id`),
  CONSTRAINT `fk_gw_deployment_members_generation` FOREIGN KEY (`deployment_generation_id`) REFERENCES `gw_deployment_generations` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_execution_health` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `scope` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `channel_id` bigint unsigned NOT NULL,
  `execution_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `credential_version_id` bigint unsigned DEFAULT NULL,
  `credential_version_key` bigint unsigned GENERATED ALWAYS AS ((case when (`scope` = _utf8mb4'credential') then `credential_version_id` else 0 end)) STORED,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `retry_at` datetime(3) DEFAULT NULL,
  `lease_owner` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `lease_expires_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_execution_health_scope` (`scope`,`channel_id`,`execution_fingerprint`,`credential_version_key`),
  KEY `fk_gw_execution_health_credential_version` (`channel_id`,`credential_version_id`),
  CONSTRAINT `fk_gw_execution_health_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_execution_health_credential_version` FOREIGN KEY (`channel_id`, `credential_version_id`) REFERENCES `gw_credential_versions` (`channel_id`, `id`),
  CONSTRAINT `ck_gw_execution_health_scope` CHECK ((((`scope` = _utf8mb4'shared') and (`credential_version_id` is null)) or ((`scope` = _utf8mb4'credential') and (`credential_version_id` is not null)))),
  CONSTRAINT `ck_gw_execution_health_state` CHECK ((`state` in (_utf8mb4'closed',_utf8mb4'open',_utf8mb4'half_open')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_execution_health_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `execution_health_id` bigint unsigned NOT NULL,
  `health_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_log_id` bigint unsigned DEFAULT NULL,
  `evidence_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_execution_health_events_version` (`execution_health_id`,`health_version`),
  KEY `fk_gw_execution_health_events_log` (`request_log_id`),
  CONSTRAINT `fk_gw_execution_health_events_health` FOREIGN KEY (`execution_health_id`) REFERENCES `gw_execution_health` (`id`),
  CONSTRAINT `fk_gw_execution_health_events_log` FOREIGN KEY (`request_log_id`) REFERENCES `gw_channel_request_logs` (`id`),
  CONSTRAINT `ck_gw_execution_health_events_state` CHECK ((`new_state` in (_utf8mb4'closed',_utf8mb4'open',_utf8mb4'half_open')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_file_resources` (
  `id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `filename` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `purpose` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL,
  `bytes` bigint unsigned NOT NULL,
  `mime_type` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_file_resources_id_owner` (`id`,`user_id`,`token_id`),
  KEY `idx_gw_file_resources_token_created` (`token_id`,`created_at`,`id`),
  KEY `idx_gw_file_resources_user_created` (`user_id`,`created_at`,`id`),
  KEY `idx_gw_file_resources_purpose` (`purpose`),
  KEY `fk_gw_file_resources_token_owner` (`token_id`,`user_id`),
  CONSTRAINT `fk_gw_file_resources_token_owner` FOREIGN KEY (`token_id`, `user_id`) REFERENCES `tokens` (`id`, `user_id`),
  CONSTRAINT `ck_gw_file_resources_bytes` CHECK ((`bytes` > 0)),
  CONSTRAINT `ck_gw_file_resources_status` CHECK ((`status` in (_utf8mb4'uploading',_utf8mb4'processed',_utf8mb4'deleting',_utf8mb4'deleted',_utf8mb4'failed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_legacy_account_snapshots` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `migration_run_id` bigint unsigned NOT NULL,
  `subject_type` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned DEFAULT NULL,
  `balance` decimal(38,18) NOT NULL,
  `total_used` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `billing_account_id` bigint unsigned NOT NULL,
  `budget_window_id` bigint unsigned DEFAULT NULL,
  `source_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_created_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_legacy_account_snapshot_source` (`subject_type`,`source_id`),
  UNIQUE KEY `uq_gw_legacy_account_snapshot_hmac` (`subject_type`,`source_id`,`source_hmac`),
  KEY `fk_gw_legacy_account_snapshot_run` (`migration_run_id`),
  KEY `fk_gw_legacy_account_snapshot_account` (`billing_account_id`),
  KEY `fk_gw_legacy_account_snapshot_window` (`budget_window_id`),
  CONSTRAINT `fk_gw_legacy_account_snapshot_account` FOREIGN KEY (`billing_account_id`) REFERENCES `billing_accounts` (`id`),
  CONSTRAINT `fk_gw_legacy_account_snapshot_run` FOREIGN KEY (`migration_run_id`) REFERENCES `gw_migration_runs` (`id`),
  CONSTRAINT `fk_gw_legacy_account_snapshot_window` FOREIGN KEY (`budget_window_id`) REFERENCES `token_budget_windows` (`id`),
  CONSTRAINT `ck_gw_legacy_account_snapshot_subject` CHECK ((((`subject_type` = _utf8mb4'user') and (`token_id` is null) and (`budget_window_id` is null)) or ((`subject_type` = _utf8mb4'token') and (`token_id` = `source_id`) and (`budget_window_id` is not null)))),
  CONSTRAINT `ck_gw_legacy_account_snapshot_used` CHECK ((`total_used` >= 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_legacy_account_snapshots_no_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_legacy_account_snapshots_no_update` BEFORE UPDATE ON `gw_legacy_account_snapshots` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Legacy account snapshots are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_legacy_account_snapshots_no_delete`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_legacy_account_snapshots_no_delete` BEFORE DELETE ON `gw_legacy_account_snapshots` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Legacy account snapshots are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_legacy_billing_evidence` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `migration_run_id` bigint unsigned NOT NULL,
  `source_table` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_pk` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `record_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `account_type` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `account_id` bigint unsigned DEFAULT NULL,
  `legacy_type` varchar(24) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `legacy_status` varchar(24) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `direction` varchar(8) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `category` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `amount` decimal(38,18) NOT NULL,
  `balance_before` decimal(38,18) DEFAULT NULL,
  `balance_after` decimal(38,18) DEFAULT NULL,
  `target_call_id` bigint unsigned DEFAULT NULL,
  `target_attempt_id` bigint unsigned DEFAULT NULL,
  `evidence_hmacs` json NOT NULL,
  `source_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_created_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_legacy_billing_evidence_source` (`source_table`,`source_pk`),
  KEY `fk_gw_legacy_billing_evidence_run` (`migration_run_id`),
  KEY `fk_gw_legacy_billing_evidence_call` (`target_call_id`),
  KEY `fk_gw_legacy_billing_evidence_attempt` (`target_attempt_id`),
  CONSTRAINT `fk_gw_legacy_billing_evidence_attempt` FOREIGN KEY (`target_attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_legacy_billing_evidence_call` FOREIGN KEY (`target_call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_legacy_billing_evidence_run` FOREIGN KEY (`migration_run_id`) REFERENCES `gw_migration_runs` (`id`),
  CONSTRAINT `ck_gw_legacy_billing_evidence_amount` CHECK ((`amount` > 0)),
  CONSTRAINT `ck_gw_legacy_billing_evidence_direction` CHECK (((`direction` is null) or (`direction` in (_utf8mb4'debit',_utf8mb4'credit')))),
  CONSTRAINT `ck_gw_legacy_billing_evidence_kind` CHECK ((((`record_kind` = _utf8mb4'billing_log') and (`source_table` = _utf8mb4'billing_logs')) or ((`record_kind` = _utf8mb4'balance_entry') and (`source_table` = _utf8mb4'balance_entries'))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_legacy_billing_evidence_no_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_legacy_billing_evidence_no_update` BEFORE UPDATE ON `gw_legacy_billing_evidence` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Legacy billing evidence is immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_legacy_billing_evidence_no_delete`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_legacy_billing_evidence_no_delete` BEFORE DELETE ON `gw_legacy_billing_evidence` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Legacy billing evidence is immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_media_asset_refs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `media_asset_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `role` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ordinal` int unsigned NOT NULL DEFAULT '0',
  `ai_file_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `call_id` bigint unsigned DEFAULT NULL,
  `result_delivery_id` bigint unsigned DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_media_asset_refs_call_role_ordinal` (`call_id`,`role`,`ordinal`),
  UNIQUE KEY `uq_gw_media_asset_refs_delivery_role_ordinal` (`result_delivery_id`,`role`,`ordinal`),
  UNIQUE KEY `uq_gw_media_asset_refs_file_role_ordinal` (`ai_file_id`,`role`,`ordinal`),
  KEY `fk_gw_media_asset_refs_asset_owner` (`media_asset_id`,`user_id`,`token_id`),
  KEY `fk_gw_media_asset_refs_file_owner` (`ai_file_id`,`user_id`,`token_id`),
  CONSTRAINT `fk_gw_media_asset_refs_asset` FOREIGN KEY (`media_asset_id`) REFERENCES `gw_media_assets` (`id`),
  CONSTRAINT `fk_gw_media_asset_refs_asset_owner` FOREIGN KEY (`media_asset_id`, `user_id`, `token_id`) REFERENCES `gw_media_assets` (`id`, `user_id`, `token_id`),
  CONSTRAINT `fk_gw_media_asset_refs_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_media_asset_refs_delivery` FOREIGN KEY (`result_delivery_id`) REFERENCES `gw_result_deliveries` (`id`),
  CONSTRAINT `fk_gw_media_asset_refs_file_owner` FOREIGN KEY (`ai_file_id`, `user_id`, `token_id`) REFERENCES `gw_file_resources` (`id`, `user_id`, `token_id`),
  CONSTRAINT `ck_gw_media_asset_refs_parent` CHECK (((((`ai_file_id` is not null) + (`call_id` is not null)) + (`result_delivery_id` is not null)) = 1)),
  CONSTRAINT `ck_gw_media_asset_refs_role` CHECK ((`role` in (_utf8mb4'input',_utf8mb4'result',_utf8mb4'file')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_media_asset_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `media_asset_id` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_media_asset_state_events_version` (`media_asset_id`,`state_version`),
  CONSTRAINT `fk_gw_media_asset_state_events_asset` FOREIGN KEY (`media_asset_id`) REFERENCES `gw_media_assets` (`id`),
  CONSTRAINT `ck_gw_media_asset_state_events_new_state` CHECK ((`new_state` in (_utf8mb4'staging',_utf8mb4'active',_utf8mb4'deleting',_utf8mb4'deleted'))),
  CONSTRAINT `ck_gw_media_asset_state_events_old_state` CHECK (((`old_state` is null) or (`old_state` in (_utf8mb4'staging',_utf8mb4'active',_utf8mb4'deleting',_utf8mb4'deleted'))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_media_assets` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `attempt_id` bigint unsigned DEFAULT NULL,
  `purpose` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `object_key` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `storage_locator` varchar(2048) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `object_version` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `content_type` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `content_length` bigint unsigned NOT NULL,
  `sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `retention_until` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  `xfs_base_url` varchar(2048) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `xfs_object_locator` varchar(2048) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `xfs_api_key` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_media_assets_object_key` (`object_key`),
  UNIQUE KEY `uq_gw_media_assets_id_owner` (`id`,`user_id`,`token_id`),
  KEY `idx_gw_media_assets_attempt` (`attempt_id`),
  CONSTRAINT `fk_gw_media_assets_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `ck_gw_media_assets_length` CHECK ((`content_length` > 0)),
  CONSTRAINT `ck_gw_media_assets_purpose` CHECK ((`purpose` in (_utf8mb4'input',_utf8mb4'result',_utf8mb4'file'))),
  CONSTRAINT `ck_gw_media_assets_state` CHECK ((`state` in (_utf8mb4'staging',_utf8mb4'active',_utf8mb4'deleting',_utf8mb4'deleted')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_migration_issues` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `run_id` bigint unsigned NOT NULL,
  `source_table` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_pk` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `issue_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `detail` varchar(1000) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_gw_migration_issue_run` (`run_id`),
  CONSTRAINT `fk_gw_migration_issue_run` FOREIGN KEY (`run_id`) REFERENCES `gw_migration_runs` (`id`),
  CONSTRAINT `ck_gw_migration_issue_status` CHECK ((`status` in (_utf8mb4'open',_utf8mb4'resolved',_utf8mb4'waived')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_migration_mapping_proofs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `run_id` bigint unsigned NOT NULL,
  `object_map_id` bigint unsigned NOT NULL,
  `source_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `observed_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_migration_mapping_proof_run_map` (`run_id`,`object_map_id`),
  KEY `idx_gw_migration_mapping_proof_map` (`object_map_id`,`source_hmac`),
  CONSTRAINT `fk_gw_migration_mapping_proof_revision` FOREIGN KEY (`object_map_id`, `source_hmac`) REFERENCES `gw_migration_source_revisions` (`object_map_id`, `source_hmac`),
  CONSTRAINT `fk_gw_migration_mapping_proof_run` FOREIGN KEY (`run_id`) REFERENCES `gw_migration_runs` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_migration_mapping_proof_no_update`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_migration_mapping_proof_no_update` BEFORE UPDATE ON `gw_migration_mapping_proofs` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Migration mapping proofs are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
/*!50003 SET @saved_cs_results     = @@character_set_results */ ;
/*!50003 SET @saved_col_connection = @@collation_connection */ ;
/*!50003 SET character_set_client  = utf8mb4 */ ;
/*!50003 SET character_set_results = utf8mb4 */ ;
/*!50003 SET collation_connection  = utf8mb4_0900_ai_ci */ ;
/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;
/*!50003 SET sql_mode              = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION' */ ;
DROP TRIGGER IF EXISTS `trg_gw_migration_mapping_proof_no_delete`;
/*!50003 CREATE*/ /*!50003 TRIGGER `trg_gw_migration_mapping_proof_no_delete` BEFORE DELETE ON `gw_migration_mapping_proofs` FOR EACH ROW SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Migration mapping proofs are immutable' */;
/*!50003 SET sql_mode              = @saved_sql_mode */ ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
/*!50003 SET character_set_results = @saved_cs_results */ ;
/*!50003 SET collation_connection  = @saved_col_connection */ ;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_migration_object_map` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `run_id` bigint unsigned NOT NULL,
  `source_table` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_pk` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `target_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `target_id` bigint unsigned NOT NULL,
  `target_discriminator` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_migration_object_map_source_target` (`source_table`,`source_pk`,`target_type`,`target_discriminator`),
  KEY `fk_gw_migration_object_map_run` (`run_id`),
  CONSTRAINT `fk_gw_migration_object_map_run` FOREIGN KEY (`run_id`) REFERENCES `gw_migration_runs` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_migration_runs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `operation` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_revision_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `started_at` datetime(3) NOT NULL,
  `finished_at` datetime(3) DEFAULT NULL,
  `error_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_migration_runs_operation_revision` (`operation`,`source_revision_hmac`),
  CONSTRAINT `ck_gw_migration_runs_status` CHECK ((`status` in (_utf8mb4'scheduled',_utf8mb4'running',_utf8mb4'succeeded',_utf8mb4'failed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_migration_source_revisions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `object_map_id` bigint unsigned NOT NULL,
  `source_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `observed_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_migration_source_revision` (`object_map_id`,`source_hmac`),
  CONSTRAINT `fk_gw_migration_source_revision_map` FOREIGN KEY (`object_map_id`) REFERENCES `gw_migration_object_map` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_model_meta` (
  `model_name` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL,
  `display_name` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `thinking_config` json DEFAULT NULL,
  `max_tokens` bigint DEFAULT '0',
  `features` json DEFAULT NULL,
  `group_name` varchar(80) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `status` tinyint DEFAULT '1',
  `sort` bigint DEFAULT '0',
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`model_name`),
  KEY `idx_gw_model_meta_sort` (`sort`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_model_names` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `model_id` bigint unsigned NOT NULL,
  `api_name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `is_primary` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_model_names_api_name` (`api_name`),
  UNIQUE KEY `uq_gw_model_names_model_id_id` (`model_id`,`id`),
  CONSTRAINT `fk_gw_model_names_model` FOREIGN KEY (`model_id`) REFERENCES `gw_models` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_model_operations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `catalog_model_id` bigint unsigned NOT NULL,
  `operation_contract_id` bigint unsigned NOT NULL,
  `normalization_version` int unsigned NOT NULL,
  `semantic_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_model_operations_release_model_contract` (`release_id`,`catalog_model_id`,`operation_contract_id`),
  UNIQUE KEY `uq_gw_model_operations_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_model_operations_contract` (`operation_contract_id`),
  CONSTRAINT `fk_gw_model_operations_catalog_model` FOREIGN KEY (`release_id`, `catalog_model_id`) REFERENCES `gw_catalog_models` (`release_id`, `id`),
  CONSTRAINT `fk_gw_model_operations_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`),
  CONSTRAINT `fk_gw_model_operations_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_models` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `model_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_models_model_code` (`model_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_offering_runtime_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `offering_id` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_offering_runtime_state_release_offering` (`release_id`,`offering_id`),
  CONSTRAINT `fk_gw_offering_runtime_state_offering` FOREIGN KEY (`release_id`, `offering_id`) REFERENCES `gw_offerings` (`release_id`, `id`),
  CONSTRAINT `ck_gw_offering_runtime_state_state` CHECK ((`state` in (_utf8mb4'active',_utf8mb4'draining',_utf8mb4'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_offering_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `offering_id` bigint unsigned NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_offering_state_events_version` (`release_id`,`offering_id`,`state_version`),
  CONSTRAINT `fk_gw_offering_state_events_offering` FOREIGN KEY (`release_id`, `offering_id`) REFERENCES `gw_offerings` (`release_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_offerings` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `product_transport_id` bigint unsigned NOT NULL,
  `credential_pool_id` bigint unsigned NOT NULL,
  `entitlement_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `commercial_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `cost_plan_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_offerings_release_transport_pool` (`release_id`,`product_transport_id`,`credential_pool_id`),
  UNIQUE KEY `uq_gw_offerings_release_id_id` (`release_id`,`id`),
  UNIQUE KEY `uq_gw_offerings_release_id_product_transport` (`release_id`,`id`,`product_transport_id`),
  KEY `fk_gw_offerings_pool` (`credential_pool_id`),
  KEY `idx_gw_offerings_entitlement` (`entitlement_fingerprint`),
  CONSTRAINT `fk_gw_offerings_pool` FOREIGN KEY (`credential_pool_id`) REFERENCES `gw_credential_pools` (`id`),
  CONSTRAINT `fk_gw_offerings_product_transport` FOREIGN KEY (`release_id`, `product_transport_id`) REFERENCES `gw_product_transports` (`release_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_operation_contracts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `operation_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `contract_version` int unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_operation_contracts_code_version` (`operation_code`,`contract_version`),
  CONSTRAINT `ck_gw_operation_contracts_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_operation_routes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `operation_contract_id` bigint unsigned NOT NULL,
  `http_method` varchar(10) COLLATE utf8mb4_unicode_ci NOT NULL,
  `route_template` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_operation_routes_method_template` (`http_method`,`route_template`),
  UNIQUE KEY `uq_gw_operation_routes_contract_id_id` (`operation_contract_id`,`id`),
  CONSTRAINT `fk_gw_operation_routes_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_product_transport_actions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `product_transport_id` bigint unsigned NOT NULL,
  `action_code` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `allowed_source_state` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `idempotency_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_schema_version` int unsigned NOT NULL,
  `response_schema_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_product_transport_actions_code` (`release_id`,`product_transport_id`,`action_code`),
  CONSTRAINT `fk_gw_product_transport_actions_transport` FOREIGN KEY (`release_id`, `product_transport_id`) REFERENCES `gw_product_transports` (`release_id`, `id`),
  CONSTRAINT `ck_gw_product_transport_actions_code` CHECK ((`action_code` in (_utf8mb4'submit',_utf8mb4'recover',_utf8mb4'query',_utf8mb4'cancel',_utf8mb4'named_action',_utf8mb4'result_fetch'))),
  CONSTRAINT `ck_gw_product_transport_actions_idempotency` CHECK ((`idempotency_mode` in (_utf8mb4'none',_utf8mb4'upstream',_utf8mb4'user_keyed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_product_transports` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `product_id` bigint unsigned NOT NULL,
  `channel_transport_id` bigint unsigned NOT NULL,
  `task_scope` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `cancel_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_url_policy` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `upstream_scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `upstream_scope_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `execution_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_compatibility_fingerprint` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `timeout_ms` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_product_transports_release_product_transport` (`release_id`,`product_id`,`channel_transport_id`),
  UNIQUE KEY `uq_gw_product_transports_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_product_transports_transport` (`release_id`,`channel_transport_id`),
  CONSTRAINT `fk_gw_product_transports_product` FOREIGN KEY (`release_id`, `product_id`) REFERENCES `gw_products` (`release_id`, `id`),
  CONSTRAINT `fk_gw_product_transports_transport` FOREIGN KEY (`release_id`, `channel_transport_id`) REFERENCES `gw_channel_transports` (`release_id`, `id`),
  CONSTRAINT `ck_gw_product_transports_cancel_mode` CHECK ((`cancel_mode` in (_utf8mb4'none',_utf8mb4'upstream',_utf8mb4'local_only'))),
  CONSTRAINT `ck_gw_product_transports_source_policy` CHECK ((`source_url_policy` in (_utf8mb4'fixed',_utf8mb4'refreshable'))),
  CONSTRAINT `ck_gw_product_transports_task_scope` CHECK ((`task_scope` in (_utf8mb4'none',_utf8mb4'request',_utf8mb4'task'))),
  CONSTRAINT `ck_gw_product_transports_timeout` CHECK ((`timeout_ms` > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_products` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `channel_id` bigint unsigned NOT NULL,
  `product_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `vendor_model` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `capability_constraints` json DEFAULT NULL,
  `constraints_schema_version` int unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_products_release_code` (`release_id`,`product_code`),
  UNIQUE KEY `uq_gw_products_release_id_id` (`release_id`,`id`),
  KEY `fk_gw_products_channel` (`channel_id`),
  CONSTRAINT `fk_gw_products_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_products_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_protocol_definitions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `protocol_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `protocol_version` int unsigned NOT NULL,
  `protocol_kind` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `display_name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `definition_json` json NOT NULL,
  `definition_digest` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_protocol_definitions_code_version` (`protocol_code`,`protocol_version`),
  UNIQUE KEY `uq_gw_protocol_definitions_digest` (`definition_digest`),
  CONSTRAINT `ck_gw_protocol_definitions_kind` CHECK ((`protocol_kind` in (_utf8mb4'catalog_discovery',_utf8mb4'availability'))),
  CONSTRAINT `ck_gw_protocol_definitions_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'disabled'))),
  CONSTRAINT `ck_gw_protocol_definitions_version` CHECK (((`protocol_version` > 0) and (`config_version` > 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_provider_state_id_aliases` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `provider_state_ref_id` bigint unsigned NOT NULL,
  `scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `scope_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `value_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `matchable` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `matchable_ref_id` bigint unsigned GENERATED ALWAYS AS ((case when `matchable` then `provider_state_ref_id` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_provider_state_alias_ref_version` (`provider_state_ref_id`,`hmac_key_version`),
  UNIQUE KEY `uq_gw_provider_state_alias_match` (`scope_kind`,`scope_key`,`hmac_key_version`,`value_hmac`,`matchable_ref_id`),
  CONSTRAINT `fk_gw_provider_state_alias_ref` FOREIGN KEY (`provider_state_ref_id`) REFERENCES `gw_provider_state_refs` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_provider_state_refs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_attempt_id` bigint unsigned NOT NULL,
  `product_transport_id` bigint unsigned NOT NULL,
  `offering_id` bigint unsigned NOT NULL,
  `credential_id` bigint unsigned NOT NULL,
  `state_compatibility_fingerprint` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `scope_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `encrypted_state_blob_id` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `expires_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_provider_state_refs_id_attempt` (`id`,`created_attempt_id`),
  KEY `fk_gw_provider_state_refs_attempt` (`created_attempt_id`),
  KEY `fk_gw_provider_state_refs_transport` (`product_transport_id`),
  KEY `fk_gw_provider_state_refs_offering` (`offering_id`),
  KEY `fk_gw_provider_state_refs_credential` (`credential_id`),
  KEY `fk_gw_provider_state_refs_blob` (`encrypted_state_blob_id`),
  CONSTRAINT `fk_gw_provider_state_refs_attempt` FOREIGN KEY (`created_attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_provider_state_refs_blob` FOREIGN KEY (`encrypted_state_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_provider_state_refs_credential` FOREIGN KEY (`credential_id`) REFERENCES `gw_credentials` (`id`),
  CONSTRAINT `fk_gw_provider_state_refs_offering` FOREIGN KEY (`offering_id`) REFERENCES `gw_offerings` (`id`),
  CONSTRAINT `fk_gw_provider_state_refs_transport` FOREIGN KEY (`product_transport_id`) REFERENCES `gw_product_transports` (`id`),
  CONSTRAINT `ck_gw_provider_state_refs_scope` CHECK ((`scope_kind` in (_utf8mb4'product_transport',_utf8mb4'credential_pool',_utf8mb4'credential',_utf8mb4'credential_version'))),
  CONSTRAINT `ck_gw_provider_state_refs_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'expired',_utf8mb4'revoked')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_rate_evidence` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `source_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `authority_level` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_reference` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `observed_at` datetime(3) NOT NULL,
  `fact_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `unit_code` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `unit_price` decimal(38,18) NOT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_rate_evidence_fact` (`source_type`,`source_reference`,`fact_hmac`),
  KEY `fk_gw_rate_evidence_currency` (`currency_code`,`currency_version`),
  CONSTRAINT `fk_gw_rate_evidence_currency` FOREIGN KEY (`currency_code`, `currency_version`) REFERENCES `billing_currency_definitions` (`currency_code`, `definition_version`),
  CONSTRAINT `ck_gw_rate_evidence_price` CHECK ((`unit_price` >= 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_rate_evidence_review_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `rate_evidence_id` bigint unsigned NOT NULL,
  `review_seq` bigint unsigned NOT NULL,
  `decision` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reviewer_user_id` bigint unsigned NOT NULL,
  `reason_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_rate_review_events_seq` (`rate_evidence_id`,`review_seq`),
  CONSTRAINT `fk_gw_rate_review_events_evidence` FOREIGN KEY (`rate_evidence_id`) REFERENCES `gw_rate_evidence` (`id`),
  CONSTRAINT `ck_gw_rate_review_events_decision` CHECK ((`decision` in (_utf8mb4'submitted',_utf8mb4'accepted',_utf8mb4'rejected',_utf8mb4'superseded')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_rate_evidence_review_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `rate_evidence_id` bigint unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `latest_review_event_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_rate_review_state_evidence` (`rate_evidence_id`),
  KEY `fk_gw_rate_review_state_event` (`latest_review_event_id`),
  CONSTRAINT `fk_gw_rate_review_state_event` FOREIGN KEY (`latest_review_event_id`) REFERENCES `gw_rate_evidence_review_events` (`id`),
  CONSTRAINT `fk_gw_rate_review_state_evidence` FOREIGN KEY (`rate_evidence_id`) REFERENCES `gw_rate_evidence` (`id`),
  CONSTRAINT `ck_gw_rate_review_state_state` CHECK ((`state` in (_utf8mb4'submitted',_utf8mb4'accepted',_utf8mb4'rejected',_utf8mb4'superseded')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_result_deliveries` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned NOT NULL,
  `attempt_id` bigint unsigned NOT NULL,
  `result_ordinal` int unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `token_id` bigint unsigned NOT NULL,
  `delivery_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `current_source_id` bigint unsigned DEFAULT NULL,
  `media_asset_ref_id` bigint unsigned DEFAULT NULL,
  `state` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `action_seq` bigint unsigned NOT NULL DEFAULT '0',
  `content_sha256` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `content_length` bigint unsigned DEFAULT NULL,
  `content_type` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `retry_at` datetime(3) DEFAULT NULL,
  `expires_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_result_deliveries_attempt_ordinal` (`attempt_id`,`result_ordinal`),
  UNIQUE KEY `uq_gw_result_deliveries_id_call_attempt` (`id`,`call_id`,`attempt_id`),
  UNIQUE KEY `uq_gw_result_deliveries_id_attempt` (`id`,`attempt_id`),
  KEY `fk_gw_result_deliveries_attempt` (`call_id`,`attempt_id`),
  KEY `idx_gw_result_deliveries_expiry` (`state`,`expires_at`,`id`),
  KEY `idx_gw_result_deliveries_recovery` (`state`,`retry_at`,`id`),
  CONSTRAINT `fk_gw_result_deliveries_attempt` FOREIGN KEY (`call_id`, `attempt_id`) REFERENCES `gw_api_call_attempts` (`call_id`, `id`),
  CONSTRAINT `fk_gw_result_deliveries_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `ck_gw_result_deliveries_mode` CHECK ((`delivery_mode` in (_utf8mb4'reference',_utf8mb4'managed_copy'))),
  CONSTRAINT `ck_gw_result_deliveries_source` CHECK ((((`delivery_mode` = _utf8mb4'reference') and (`source_kind` = _utf8mb4'remote_url')) or ((`delivery_mode` = _utf8mb4'managed_copy') and (`source_kind` in (_utf8mb4'remote_url',_utf8mb4'inline_response'))))),
  CONSTRAINT `ck_gw_result_deliveries_state` CHECK ((`state` in (_utf8mb4'pending',_utf8mb4'ready',_utf8mb4'delivery_failed',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_result_delivery_sources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `result_delivery_id` bigint unsigned NOT NULL,
  `source_seq` bigint unsigned NOT NULL,
  `encrypted_url_blob_id` bigint unsigned NOT NULL,
  `url_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `provider_result_identity_hmac` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `observed_at` datetime(3) NOT NULL,
  `expires_at` datetime(3) DEFAULT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `active_delivery_id` bigint unsigned GENERATED ALWAYS AS ((case when (`state` = _utf8mb4'active') then `result_delivery_id` else NULL end)) STORED,
  `observed_request_log_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_result_delivery_sources_delivery_seq` (`result_delivery_id`,`source_seq`),
  UNIQUE KEY `uq_gw_result_delivery_sources_delivery_id` (`result_delivery_id`,`id`),
  UNIQUE KEY `uq_gw_result_delivery_sources_one_active` (`active_delivery_id`),
  KEY `fk_gw_result_delivery_sources_blob` (`encrypted_url_blob_id`),
  KEY `fk_gw_result_sources_observed_request` (`observed_request_log_id`),
  CONSTRAINT `fk_gw_result_delivery_sources_blob` FOREIGN KEY (`encrypted_url_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_result_delivery_sources_delivery` FOREIGN KEY (`result_delivery_id`) REFERENCES `gw_result_deliveries` (`id`),
  CONSTRAINT `fk_gw_result_sources_observed_request` FOREIGN KEY (`observed_request_log_id`) REFERENCES `gw_channel_request_logs` (`id`),
  CONSTRAINT `ck_gw_result_delivery_sources_state` CHECK ((`state` in (_utf8mb4'active',_utf8mb4'superseded',_utf8mb4'invalid',_utf8mb4'consumed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_route_states` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `key_id` bigint unsigned NOT NULL,
  `model_name` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL,
  `transport` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `disabled_until` datetime(3) NOT NULL,
  `reason` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `status_code` bigint DEFAULT '0',
  `fail_count` bigint DEFAULT '1',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_gw_route_state` (`key_id`,`model_name`,`transport`),
  KEY `idx_gw_route_states_key_id` (`key_id`),
  KEY `idx_gw_route_states_disabled_until` (`disabled_until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_routes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `sku_id` bigint unsigned NOT NULL,
  `offering_id` bigint unsigned NOT NULL,
  `priority` int unsigned NOT NULL,
  `weight` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_routes_release_sku_offering` (`release_id`,`sku_id`,`offering_id`),
  UNIQUE KEY `uq_gw_routes_release_id_id` (`release_id`,`id`),
  UNIQUE KEY `uq_gw_routes_release_id_sku_offering` (`release_id`,`id`,`sku_id`,`offering_id`),
  KEY `fk_gw_routes_offering` (`release_id`,`offering_id`),
  CONSTRAINT `fk_gw_routes_offering` FOREIGN KEY (`release_id`, `offering_id`) REFERENCES `gw_offerings` (`release_id`, `id`),
  CONSTRAINT `fk_gw_routes_sku` FOREIGN KEY (`release_id`, `sku_id`) REFERENCES `gw_skus` (`release_id`, `id`),
  CONSTRAINT `ck_gw_routes_weight` CHECK ((`weight` > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_routing_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token_id` bigint unsigned NOT NULL,
  `policy_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_routing_policies_token_code` (`token_id`,`policy_code`),
  UNIQUE KEY `uq_gw_routing_policies_token_id_id` (`token_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_routing_policy_entries` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `routing_policy_version_id` bigint unsigned NOT NULL,
  `operation_contract_id` bigint unsigned DEFAULT NULL,
  `model_id` bigint unsigned DEFAULT NULL,
  `channel_id` bigint unsigned DEFAULT NULL,
  `decision` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `priority` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_routing_policy_entries_version_order` (`routing_policy_version_id`,`priority`,`id`),
  KEY `fk_gw_routing_policy_entries_contract` (`operation_contract_id`),
  KEY `fk_gw_routing_policy_entries_model` (`model_id`),
  KEY `fk_gw_routing_policy_entries_channel` (`channel_id`),
  CONSTRAINT `fk_gw_routing_policy_entries_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`),
  CONSTRAINT `fk_gw_routing_policy_entries_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`),
  CONSTRAINT `fk_gw_routing_policy_entries_model` FOREIGN KEY (`model_id`) REFERENCES `gw_models` (`id`),
  CONSTRAINT `fk_gw_routing_policy_entries_version` FOREIGN KEY (`routing_policy_version_id`) REFERENCES `gw_routing_policy_versions` (`id`),
  CONSTRAINT `ck_gw_routing_policy_entries_decision` CHECK ((`decision` in (_utf8mb4'allow',_utf8mb4'deny')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_routing_policy_version_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `routing_policy_version_id` bigint unsigned NOT NULL,
  `old_state` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `fk_gw_routing_policy_version_events_version` (`routing_policy_version_id`),
  CONSTRAINT `fk_gw_routing_policy_version_events_version` FOREIGN KEY (`routing_policy_version_id`) REFERENCES `gw_routing_policy_versions` (`id`),
  CONSTRAINT `ck_gw_routing_policy_version_events_state` CHECK ((`new_state` in (_utf8mb4'draft',_utf8mb4'published',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_routing_policy_versions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `routing_policy_id` bigint unsigned NOT NULL,
  `version_no` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `published_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_routing_policy_versions_policy_version` (`routing_policy_id`,`version_no`),
  CONSTRAINT `fk_gw_routing_policy_versions_policy` FOREIGN KEY (`routing_policy_id`) REFERENCES `gw_routing_policies` (`id`),
  CONSTRAINT `ck_gw_routing_policy_versions_status` CHECK ((`status` in (_utf8mb4'draft',_utf8mb4'published',_utf8mb4'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_runtime_requirement_guards` (
  `shard_id` int unsigned NOT NULL,
  `generation` bigint unsigned NOT NULL DEFAULT '1',
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`shard_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_runtime_requirement_refs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `requirement_id` bigint unsigned NOT NULL,
  `call_id` bigint unsigned DEFAULT NULL,
  `attempt_id` bigint unsigned DEFAULT NULL,
  `control_plane_run_id` bigint unsigned DEFAULT NULL,
  `provider_state_ref_id` bigint unsigned DEFAULT NULL,
  `credential_slot_id` bigint unsigned DEFAULT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_runtime_requirement_refs_requirement_call` (`requirement_id`,`call_id`),
  UNIQUE KEY `uq_gw_runtime_requirement_refs_requirement_attempt` (`requirement_id`,`attempt_id`),
  UNIQUE KEY `uq_gw_runtime_requirement_refs_requirement_run` (`requirement_id`,`control_plane_run_id`),
  UNIQUE KEY `uq_gw_runtime_requirement_refs_requirement_provider_state` (`requirement_id`,`provider_state_ref_id`),
  UNIQUE KEY `uq_gw_runtime_requirement_refs_requirement_slot` (`requirement_id`,`credential_slot_id`),
  KEY `fk_gw_runtime_requirement_refs_call` (`call_id`),
  KEY `fk_gw_runtime_requirement_refs_attempt` (`attempt_id`),
  KEY `fk_gw_runtime_requirement_refs_run` (`control_plane_run_id`),
  KEY `fk_gw_runtime_requirement_refs_provider_state` (`provider_state_ref_id`),
  KEY `fk_gw_runtime_requirement_refs_slot` (`credential_slot_id`),
  CONSTRAINT `fk_gw_runtime_requirement_refs_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_runtime_requirement_refs_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_runtime_requirement_refs_provider_state` FOREIGN KEY (`provider_state_ref_id`) REFERENCES `gw_provider_state_refs` (`id`),
  CONSTRAINT `fk_gw_runtime_requirement_refs_requirement` FOREIGN KEY (`requirement_id`) REFERENCES `gw_runtime_requirements` (`id`),
  CONSTRAINT `fk_gw_runtime_requirement_refs_run` FOREIGN KEY (`control_plane_run_id`) REFERENCES `gw_control_plane_runs` (`id`),
  CONSTRAINT `fk_gw_runtime_requirement_refs_slot` FOREIGN KEY (`credential_slot_id`) REFERENCES `gw_credential_slots` (`id`),
  CONSTRAINT `ck_gw_runtime_requirement_refs_parent` CHECK (((((((`call_id` is not null) + (`attempt_id` is not null)) + (`control_plane_run_id` is not null)) + (`provider_state_ref_id` is not null)) + (`credential_slot_id` is not null)) = 1)),
  CONSTRAINT `ck_gw_runtime_requirement_refs_state` CHECK ((`state` in (_utf8mb4'active',_utf8mb4'released')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_runtime_requirements` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `shard_id` int unsigned NOT NULL,
  `kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `role` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `release_id` bigint unsigned DEFAULT NULL,
  `operation_contract_id` bigint unsigned DEFAULT NULL,
  `scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `scope_key` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `compatibility_fingerprint` char(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `adapter_implementation_id` bigint unsigned DEFAULT NULL,
  `release_key` bigint unsigned GENERATED ALWAYS AS ((case when (`kind` = _utf8mb4'release') then `release_id` else 0 end)) STORED,
  `compatibility_contract_key` bigint unsigned GENERATED ALWAYS AS ((case when (`kind` = _utf8mb4'compatibility') then `operation_contract_id` else 0 end)) STORED,
  `implementation_key` bigint unsigned GENERATED ALWAYS AS ((case when (`kind` = _utf8mb4'implementation') then `adapter_implementation_id` else 0 end)) STORED,
  `active_ref_count` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_runtime_requirements_release` (`shard_id`,`role`,`kind`,`release_key`),
  UNIQUE KEY `uq_gw_runtime_requirements_compatibility` (`shard_id`,`role`,`kind`,`compatibility_contract_key`,`scope_kind`,`scope_key`,`compatibility_fingerprint`),
  UNIQUE KEY `uq_gw_runtime_requirements_implementation` (`shard_id`,`role`,`kind`,`implementation_key`),
  KEY `fk_gw_runtime_requirements_release` (`release_id`),
  KEY `fk_gw_runtime_requirements_contract` (`operation_contract_id`),
  KEY `fk_gw_runtime_requirements_adapter` (`adapter_implementation_id`),
  CONSTRAINT `fk_gw_runtime_requirements_adapter` FOREIGN KEY (`adapter_implementation_id`) REFERENCES `gw_adapter_implementations` (`id`),
  CONSTRAINT `fk_gw_runtime_requirements_contract` FOREIGN KEY (`operation_contract_id`) REFERENCES `gw_operation_contracts` (`id`),
  CONSTRAINT `fk_gw_runtime_requirements_guard` FOREIGN KEY (`shard_id`) REFERENCES `gw_runtime_requirement_guards` (`shard_id`),
  CONSTRAINT `fk_gw_runtime_requirements_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_runtime_requirements_kind` CHECK ((((`kind` = _utf8mb4'release') and (`release_id` is not null) and (`operation_contract_id` is null) and (`adapter_implementation_id` is null)) or ((`kind` = _utf8mb4'compatibility') and (`release_id` is null) and (`operation_contract_id` is not null) and (`adapter_implementation_id` is null) and (`scope_kind` is not null) and (`scope_key` is not null) and (`compatibility_fingerprint` is not null)) or ((`kind` = _utf8mb4'implementation') and (`release_id` is null) and (`operation_contract_id` is null) and (`adapter_implementation_id` is not null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_sell_rates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `sku_id` bigint unsigned NOT NULL,
  `rate_evidence_review_event_id` bigint unsigned NOT NULL,
  `unit_code` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `unit_price` decimal(38,18) NOT NULL,
  `pricing_mode` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT 'flat',
  `pricing_expr` text CHARACTER SET utf8mb4 COLLATE utf8mb4_bin,
  `max_price` decimal(38,18) DEFAULT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `component_code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `quantity_source` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `charge_event` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `unit_scale` tinyint unsigned NOT NULL DEFAULT '0',
  `quantity_step` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `max_quantity` decimal(38,18) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_sell_rates_component` (`release_id`,`sku_id`,`component_code`),
  UNIQUE KEY `uq_gw_sell_rates_meter` (`release_id`,`sku_id`,`charge_event`,`quantity_source`),
  KEY `fk_gw_sell_rates_review_event` (`rate_evidence_review_event_id`),
  KEY `fk_gw_sell_rates_currency` (`currency_code`,`currency_version`),
  CONSTRAINT `fk_gw_sell_rates_currency` FOREIGN KEY (`currency_code`, `currency_version`) REFERENCES `billing_currency_definitions` (`currency_code`, `definition_version`),
  CONSTRAINT `fk_gw_sell_rates_review_event` FOREIGN KEY (`rate_evidence_review_event_id`) REFERENCES `gw_rate_evidence_review_events` (`id`),
  CONSTRAINT `fk_gw_sell_rates_sku` FOREIGN KEY (`release_id`, `sku_id`) REFERENCES `gw_skus` (`release_id`, `id`),
  CONSTRAINT `ck_gw_sell_rates_charge_event` CHECK ((`charge_event` in (_utf8mb4'call.succeeded',_utf8mb4'call.failed',_utf8mb4'call.cancelled',_utf8mb4'provider.accepted',_utf8mb4'delivery.ready',_utf8mb4'delivery.failed'))),
  CONSTRAINT `ck_gw_sell_rates_expr_pairing` CHECK ((((`pricing_mode` = _utf8mb4'expression') and (`pricing_expr` is not null) and (`max_price` is not null) and (`max_price` >= 0)) or ((`pricing_mode` = _utf8mb4'flat') and (`pricing_expr` is null) and (`max_price` is null)))),
  CONSTRAINT `ck_gw_sell_rates_max_quantity` CHECK ((`max_quantity` > 0)),
  CONSTRAINT `ck_gw_sell_rates_price` CHECK ((`unit_price` >= 0)),
  CONSTRAINT `ck_gw_sell_rates_pricing_mode` CHECK ((`pricing_mode` in (_utf8mb4'flat',_utf8mb4'expression'))),
  CONSTRAINT `ck_gw_sell_rates_quantity_source` CHECK ((`quantity_source` in (_utf8mb4'one',_utf8mb4'usage.input_tokens',_utf8mb4'usage.output_tokens',_utf8mb4'usage.uncached_input_tokens',_utf8mb4'usage.cached_input_tokens',_utf8mb4'usage.cache_write_tokens',_utf8mb4'usage.input_no_cache_write',_utf8mb4'usage.input_no_cache',_utf8mb4'request.seconds',_utf8mb4'result.seconds',_utf8mb4'request.images',_utf8mb4'result.images',_utf8mb4'result.videos',_utf8mb4'result.megapixels'))),
  CONSTRAINT `ck_gw_sell_rates_quantity_step` CHECK ((`quantity_step` >= 0)),
  CONSTRAINT `ck_gw_sell_rates_unit_scale` CHECK ((`unit_scale` <= 12))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_sku_downstream_paths` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `sku_id` bigint unsigned NOT NULL,
  `path` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_sku_downstream_paths` (`release_id`,`sku_id`,`path`),
  UNIQUE KEY `uq_gw_sku_downstream_paths_release_id_id` (`release_id`,`id`),
  CONSTRAINT `fk_gw_sku_downstream_paths_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `fk_gw_sku_downstream_paths_sku` FOREIGN KEY (`release_id`, `sku_id`) REFERENCES `gw_skus` (`release_id`, `id`),
  CONSTRAINT `ck_gw_sku_downstream_paths_path` CHECK ((`path` like _utf8mb4'/%'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_skus` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `model_operation_id` bigint unsigned NOT NULL,
  `sku_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `variant_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT 'default',
  `delivery_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `max_results` int unsigned NOT NULL DEFAULT '1',
  `idempotency_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `service_tiers` json NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_skus_release_code` (`release_id`,`sku_code`),
  UNIQUE KEY `uq_gw_skus_release_id_id` (`release_id`,`id`),
  UNIQUE KEY `uq_gw_skus_release_id_model_operation` (`release_id`,`id`,`model_operation_id`),
  KEY `fk_gw_skus_model_operation` (`release_id`,`model_operation_id`),
  KEY `idx_gw_skus_release_variant` (`release_id`,`variant_code`),
  CONSTRAINT `fk_gw_skus_model_operation` FOREIGN KEY (`release_id`, `model_operation_id`) REFERENCES `gw_model_operations` (`release_id`, `id`),
  CONSTRAINT `fk_gw_skus_release` FOREIGN KEY (`release_id`) REFERENCES `gw_catalog_releases` (`id`),
  CONSTRAINT `ck_gw_skus_delivery_mode` CHECK ((`delivery_mode` in (_utf8mb4'reference',_utf8mb4'managed_copy'))),
  CONSTRAINT `ck_gw_skus_idempotency_mode` CHECK ((`idempotency_mode` in (_utf8mb4'required',_utf8mb4'optional',_utf8mb4'forbidden'))),
  CONSTRAINT `ck_gw_skus_max_results` CHECK ((`max_results` > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_state_transition_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `call_id` bigint unsigned DEFAULT NULL,
  `attempt_id` bigint unsigned DEFAULT NULL,
  `async_execution_id` bigint unsigned DEFAULT NULL,
  `task_identity_id` bigint unsigned DEFAULT NULL,
  `provider_state_ref_id` bigint unsigned DEFAULT NULL,
  `callback_receipt_id` bigint unsigned DEFAULT NULL,
  `old_state` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `new_state` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `result_delivery_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_state_events_call_version` (`call_id`,`state_version`),
  UNIQUE KEY `uq_gw_state_events_attempt_version` (`attempt_id`,`state_version`),
  UNIQUE KEY `uq_gw_state_events_async_version` (`async_execution_id`,`state_version`),
  UNIQUE KEY `uq_gw_state_events_task_identity_version` (`task_identity_id`,`state_version`),
  UNIQUE KEY `uq_gw_state_events_provider_state_version` (`provider_state_ref_id`,`state_version`),
  UNIQUE KEY `uq_gw_state_events_receipt_version` (`callback_receipt_id`,`state_version`),
  UNIQUE KEY `uq_gw_state_events_delivery_version` (`result_delivery_id`,`state_version`),
  CONSTRAINT `fk_gw_state_events_async` FOREIGN KEY (`async_execution_id`) REFERENCES `gw_async_executions` (`id`),
  CONSTRAINT `fk_gw_state_events_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_state_events_call` FOREIGN KEY (`call_id`) REFERENCES `gw_api_calls` (`id`),
  CONSTRAINT `fk_gw_state_events_delivery` FOREIGN KEY (`result_delivery_id`) REFERENCES `gw_result_deliveries` (`id`),
  CONSTRAINT `fk_gw_state_events_provider_state` FOREIGN KEY (`provider_state_ref_id`) REFERENCES `gw_provider_state_refs` (`id`),
  CONSTRAINT `fk_gw_state_events_receipt` FOREIGN KEY (`callback_receipt_id`) REFERENCES `gw_upstream_callback_receipts` (`id`),
  CONSTRAINT `fk_gw_state_events_task_identity` FOREIGN KEY (`task_identity_id`) REFERENCES `gw_upstream_task_identities` (`id`),
  CONSTRAINT `ck_gw_state_events_one_parent` CHECK (((((((((`call_id` is not null) + (`attempt_id` is not null)) + (`async_execution_id` is not null)) + (`task_identity_id` is not null)) + (`provider_state_ref_id` is not null)) + (`callback_receipt_id` is not null)) + (`result_delivery_id` is not null)) = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_transport_allowed_hosts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `release_id` bigint unsigned NOT NULL,
  `channel_transport_id` bigint unsigned NOT NULL,
  `protocol` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL,
  `host_pattern` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `port` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_transport_hosts_release_transport_host` (`release_id`,`channel_transport_id`,`host_pattern`,`port`),
  CONSTRAINT `fk_gw_transport_hosts_transport` FOREIGN KEY (`release_id`, `channel_transport_id`) REFERENCES `gw_channel_transports` (`release_id`, `id`),
  CONSTRAINT `ck_gw_transport_hosts_port` CHECK ((`port` between 1 and 65535)),
  CONSTRAINT `ck_gw_transport_hosts_protocol` CHECK ((`protocol` in (_utf8mb4'http',_utf8mb4'https')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_availability` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `availability_source_id` bigint unsigned NOT NULL,
  `catalog_source_id` bigint unsigned DEFAULT NULL,
  `model_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `category` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `success_rate` decimal(5,2) DEFAULT NULL,
  `average_completion_seconds` decimal(10,1) NOT NULL DEFAULT '0.0',
  `channel_status` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `estimated_completion_ms` bigint unsigned DEFAULT NULL,
  `window_minutes` int unsigned NOT NULL DEFAULT '0',
  `observed_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  `buckets` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_upstream_availability_source_model` (`availability_source_id`,`model_code`),
  KEY `idx_gw_upstream_availability_model` (`model_code`),
  KEY `idx_gw_upstream_availability_legacy_source` (`catalog_source_id`),
  CONSTRAINT `fk_gw_upstream_availability_monitor` FOREIGN KEY (`availability_source_id`) REFERENCES `gw_availability_sources` (`id`),
  CONSTRAINT `ck_gw_upstream_availability_completion` CHECK ((`average_completion_seconds` >= 0)),
  CONSTRAINT `ck_gw_upstream_availability_estimated_completion` CHECK (((`estimated_completion_ms` is null) or (`estimated_completion_ms` <= 86400000))),
  CONSTRAINT `ck_gw_upstream_availability_rate` CHECK (((`success_rate` is null) or (`success_rate` between 0 and 100))),
  CONSTRAINT `ck_gw_upstream_availability_window` CHECK (((`success_rate` is null) or (`window_minutes` > 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_callback_receipt_aliases` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `receipt_id` bigint unsigned NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `event_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_callback_receipt_alias_receipt_version` (`receipt_id`,`hmac_key_version`),
  UNIQUE KEY `uq_gw_callback_receipt_alias_match` (`hmac_key_version`,`event_hmac`),
  CONSTRAINT `fk_gw_callback_receipt_alias_receipt` FOREIGN KEY (`receipt_id`) REFERENCES `gw_upstream_callback_receipts` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_callback_receipts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_identity_id` bigint unsigned DEFAULT NULL,
  `async_execution_id` bigint unsigned DEFAULT NULL,
  `event_scope` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `event_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `verified_credential_version_id` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `payload_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `encrypted_payload_blob_id` bigint unsigned DEFAULT NULL,
  `expires_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_callback_receipts_event` (`event_scope`,`hmac_key_version`,`event_hmac`),
  KEY `fk_gw_callback_receipts_task_identity` (`task_identity_id`),
  KEY `fk_gw_callback_receipts_async_execution` (`async_execution_id`),
  KEY `fk_gw_callback_receipts_credential_version` (`verified_credential_version_id`),
  KEY `idx_gw_callback_receipts_processing` (`status`,`expires_at`,`id`),
  KEY `fk_gw_callback_receipts_payload_blob` (`encrypted_payload_blob_id`),
  CONSTRAINT `fk_gw_callback_receipts_async_execution` FOREIGN KEY (`async_execution_id`) REFERENCES `gw_async_executions` (`id`),
  CONSTRAINT `fk_gw_callback_receipts_credential_version` FOREIGN KEY (`verified_credential_version_id`) REFERENCES `gw_credential_versions` (`id`),
  CONSTRAINT `fk_gw_callback_receipts_payload_blob` FOREIGN KEY (`encrypted_payload_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `fk_gw_callback_receipts_task_identity` FOREIGN KEY (`task_identity_id`) REFERENCES `gw_upstream_task_identities` (`id`),
  CONSTRAINT `ck_gw_callback_receipts_parent` CHECK (((`task_identity_id` is not null) or (`async_execution_id` is not null))),
  CONSTRAINT `ck_gw_callback_receipts_parent_exclusive` CHECK ((((`task_identity_id` is not null) + (`async_execution_id` is not null)) = 1)),
  CONSTRAINT `ck_gw_callback_receipts_status` CHECK ((`status` in (_utf8mb4'received',_utf8mb4'processed',_utf8mb4'manual_review',_utf8mb4'rejected')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_cost_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `attempt_id` bigint unsigned NOT NULL,
  `component_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `event_seq` bigint unsigned NOT NULL,
  `direction` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(38,18) NOT NULL,
  `quantity` decimal(38,18) NOT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `source_type` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reconciliation_state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `request_log_id` bigint unsigned DEFAULT NULL,
  `result_delivery_id` bigint unsigned DEFAULT NULL,
  `evidence_id` bigint unsigned DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_upstream_cost_events_attempt_component_seq` (`attempt_id`,`component_code`,`event_seq`),
  KEY `fk_gw_upstream_cost_events_currency` (`currency_code`,`currency_version`),
  KEY `fk_gw_upstream_cost_events_log` (`request_log_id`),
  KEY `fk_gw_upstream_cost_events_delivery` (`result_delivery_id`),
  KEY `fk_gw_upstream_cost_events_evidence` (`evidence_id`),
  CONSTRAINT `fk_gw_upstream_cost_events_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `gw_api_call_attempts` (`id`),
  CONSTRAINT `fk_gw_upstream_cost_events_currency` FOREIGN KEY (`currency_code`, `currency_version`) REFERENCES `billing_currency_definitions` (`currency_code`, `definition_version`),
  CONSTRAINT `fk_gw_upstream_cost_events_delivery` FOREIGN KEY (`result_delivery_id`) REFERENCES `gw_result_deliveries` (`id`),
  CONSTRAINT `fk_gw_upstream_cost_events_evidence` FOREIGN KEY (`evidence_id`) REFERENCES `gw_upstream_cost_evidence` (`id`),
  CONSTRAINT `fk_gw_upstream_cost_events_log` FOREIGN KEY (`request_log_id`) REFERENCES `gw_channel_request_logs` (`id`),
  CONSTRAINT `ck_gw_upstream_cost_events_amount` CHECK (((`amount` >= 0) and (`quantity` >= 0))),
  CONSTRAINT `ck_gw_upstream_cost_events_direction` CHECK ((`direction` in (_utf8mb4'increase',_utf8mb4'decrease'))),
  CONSTRAINT `ck_gw_upstream_cost_events_reconciliation` CHECK ((`reconciliation_state` in (_utf8mb4'pending',_utf8mb4'confirmed',_utf8mb4'disputed',_utf8mb4'voided'))),
  CONSTRAINT `ck_gw_upstream_cost_events_source` CHECK ((`source_type` in (_utf8mb4'estimated',_utf8mb4'reported',_utf8mb4'manual')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_cost_evidence` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL,
  `source_type` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `external_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `fact_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_upstream_cost_evidence_source` (`channel_id`,`source_type`,`external_key`),
  CONSTRAINT `fk_gw_upstream_cost_evidence_channel` FOREIGN KEY (`channel_id`) REFERENCES `gateway_channels` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_error_rules` (
  `id` tinyint unsigned NOT NULL,
  `patterns` json NOT NULL,
  `message` varchar(500) COLLATE utf8mb4_unicode_ci NOT NULL,
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_gw_upstream_error_rules_singleton` CHECK ((`id` = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_task_id_aliases` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_identity_id` bigint unsigned NOT NULL,
  `scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `scope_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `hmac_key_version` int unsigned NOT NULL,
  `value_hmac` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `matchable` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime(3) NOT NULL,
  `invalidated_at` datetime(3) DEFAULT NULL,
  `matchable_identity_id` bigint unsigned GENERATED ALWAYS AS ((case when `matchable` then `task_identity_id` else NULL end)) STORED,
  `active_value_hmac` char(64) COLLATE utf8mb4_unicode_ci GENERATED ALWAYS AS ((case when `matchable` then `value_hmac` else NULL end)) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_task_aliases_identity_version` (`task_identity_id`,`hmac_key_version`),
  UNIQUE KEY `uq_gw_task_aliases_match` (`scope_kind`,`scope_key`,`hmac_key_version`,`value_hmac`,`matchable_identity_id`),
  UNIQUE KEY `uq_gw_task_alias_one_owner` (`scope_kind`,`scope_key`,`hmac_key_version`,`active_value_hmac`),
  CONSTRAINT `fk_gw_task_aliases_identity` FOREIGN KEY (`task_identity_id`) REFERENCES `gw_upstream_task_identities` (`id`),
  CONSTRAINT `ck_gw_task_aliases_scope` CHECK ((`scope_kind` in (_utf8mb4'global',_utf8mb4'channel',_utf8mb4'product_transport',_utf8mb4'credential_pool',_utf8mb4'credential')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_upstream_task_identities` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `async_execution_id` bigint unsigned DEFAULT NULL,
  `scope_kind` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `scope_key` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `encrypted_blob_id` bigint unsigned NOT NULL,
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state_version` bigint unsigned NOT NULL DEFAULT '1',
  `expires_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gw_task_identities_id_blob` (`id`,`encrypted_blob_id`),
  UNIQUE KEY `uq_gw_task_identities_async_execution` (`async_execution_id`),
  KEY `fk_gw_task_identities_blob` (`encrypted_blob_id`),
  CONSTRAINT `fk_gw_task_identities_async_execution` FOREIGN KEY (`async_execution_id`) REFERENCES `gw_async_executions` (`id`),
  CONSTRAINT `fk_gw_task_identities_blob` FOREIGN KEY (`encrypted_blob_id`) REFERENCES `encrypted_blobs` (`id`),
  CONSTRAINT `ck_gw_task_identities_scope` CHECK ((`scope_kind` in (_utf8mb4'global',_utf8mb4'channel',_utf8mb4'product_transport',_utf8mb4'credential_pool',_utf8mb4'credential'))),
  CONSTRAINT `ck_gw_task_identities_status` CHECK ((`status` in (_utf8mb4'unbound',_utf8mb4'bound',_utf8mb4'expired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `gw_video_tasks` (
  `resource_id` bigint unsigned NOT NULL,
  `task_no` char(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  `status` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `progress` tinyint unsigned NOT NULL DEFAULT '0',
  `specification_summary` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`resource_id`),
  UNIQUE KEY `uq_gw_video_tasks_task_no` (`task_no`),
  CONSTRAINT `fk_gw_video_tasks_resource` FOREIGN KEY (`resource_id`) REFERENCES `gw_api_resources` (`id`),
  CONSTRAINT `ck_gw_video_tasks_progress` CHECK ((`progress` <= 100))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `ledger_accounts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `billing_account_id` bigint unsigned DEFAULT NULL,
  `system_account_code` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `balance` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_ledger_accounts_id_currency` (`id`,`currency_code`,`currency_version`),
  UNIQUE KEY `uq_ledger_accounts_user_account_currency` (`billing_account_id`,`currency_code`,`currency_version`),
  UNIQUE KEY `uq_ledger_accounts_system_currency` (`system_account_code`,`currency_code`,`currency_version`),
  CONSTRAINT `fk_ledger_accounts_billing_account` FOREIGN KEY (`billing_account_id`, `currency_code`, `currency_version`) REFERENCES `billing_accounts` (`id`, `currency_code`, `currency_version`),
  CONSTRAINT `ck_ledger_accounts_owner` CHECK ((((`billing_account_id` is not null) and (`system_account_code` is null)) or ((`billing_account_id` is null) and (`system_account_code` is not null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `ledger_entries` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `ledger_transaction_id` bigint unsigned NOT NULL,
  `ledger_account_id` bigint unsigned NOT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `entry_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `direction` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount` decimal(38,18) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_ledger_entries_transaction_code` (`ledger_transaction_id`,`entry_code`),
  KEY `fk_ledger_entries_transaction` (`ledger_transaction_id`,`currency_code`,`currency_version`),
  KEY `fk_ledger_entries_account` (`ledger_account_id`,`currency_code`,`currency_version`),
  CONSTRAINT `fk_ledger_entries_account` FOREIGN KEY (`ledger_account_id`, `currency_code`, `currency_version`) REFERENCES `ledger_accounts` (`id`, `currency_code`, `currency_version`),
  CONSTRAINT `fk_ledger_entries_transaction` FOREIGN KEY (`ledger_transaction_id`, `currency_code`, `currency_version`) REFERENCES `ledger_transactions` (`id`, `currency_code`, `currency_version`),
  CONSTRAINT `ck_ledger_entries_amount` CHECK ((`amount` > 0)),
  CONSTRAINT `ck_ledger_entries_direction` CHECK ((`direction` in (_utf8mb4'debit',_utf8mb4'credit')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `ledger_transactions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `billing_event_id` bigint unsigned NOT NULL,
  `currency_code` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `currency_version` int unsigned NOT NULL,
  `state` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `posted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_ledger_transactions_billing_event` (`billing_event_id`),
  UNIQUE KEY `uq_ledger_transactions_id_currency` (`id`,`currency_code`,`currency_version`),
  KEY `fk_ledger_transactions_event` (`billing_event_id`,`currency_code`,`currency_version`),
  CONSTRAINT `fk_ledger_transactions_event` FOREIGN KEY (`billing_event_id`, `currency_code`, `currency_version`) REFERENCES `billing_events` (`id`, `currency_code`, `currency_version`),
  CONSTRAINT `ck_ledger_transactions_state` CHECK ((`state` in (_utf8mb4'assembling',_utf8mb4'posted')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `messages` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `conversation_id` bigint unsigned NOT NULL COMMENT '对话ID',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '关联调用ID',
  `request_log_id` bigint unsigned DEFAULT '0' COMMENT '关联请求日志ID',
  `role` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '角色',
  `content` mediumtext COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '内容',
  `attachments` mediumtext COLLATE utf8mb4_unicode_ci COMMENT '多模态附件(JSON)',
  `reasoning_content` mediumtext COLLATE utf8mb4_unicode_ci COMMENT '思考/推理内容',
  `finish_reason` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '完成原因',
  `input_tokens` bigint DEFAULT '0' COMMENT '输入token',
  `output_tokens` bigint DEFAULT '0' COMMENT '输出token',
  `model` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '使用模型',
  `channel_id` bigint unsigned DEFAULT '0' COMMENT '渠道ID',
  `account_id` bigint unsigned DEFAULT '0' COMMENT '账号ID',
  `latency_ms` bigint DEFAULT '0' COMMENT '耗时毫秒',
  `cost` decimal(20,8) NOT NULL DEFAULT '0.00000000' COMMENT '费用',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_conversation_created` (`conversation_id`,`created_at`),
  KEY `idx_messages_call_id` (`call_id`),
  KEY `idx_messages_request_log_id` (`request_log_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `models` (
  `code` varchar(80) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '模型标识',
  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '显示名称',
  `type` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'chat' COMMENT '模型类型',
  `provider` varchar(30) COLLATE utf8mb4_unicode_ci DEFAULT '' COMMENT '来源标识',
  `protocol` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'openai' COMMENT '协议类型(chat虚拟端点用)',
  `description` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT '' COMMENT '模型描述',
  `features` json DEFAULT NULL COMMENT '能力标签',
  `aliases` json DEFAULT NULL COMMENT 'upstream model aliases',
  `param_schema` json DEFAULT NULL COMMENT '参数schema',
  `max_tokens` bigint DEFAULT '0' COMMENT '最大token数',
  `thinking_config` json DEFAULT NULL COMMENT '思考模式配置(档位/默认/锁定)',
  `sort` bigint DEFAULT '0' COMMENT '排序(降序)',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`code`),
  KEY `idx_models_sort` (`sort`),
  KEY `idx_models_created_at` (`created_at`),
  KEY `idx_models_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `prism_update_sources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `source_name` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `base_url` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `priority` int NOT NULL DEFAULT '100',
  `config_version` bigint unsigned NOT NULL DEFAULT '1',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `updated_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_prism_update_sources_name` (`source_name`),
  UNIQUE KEY `uq_prism_update_sources_url` (`base_url`),
  CONSTRAINT `ck_prism_update_sources_enabled` CHECK ((`enabled` in (0,1))),
  CONSTRAINT `ck_prism_update_sources_priority` CHECK ((`priority` >= 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `prism_update_sources_meta` (
  `id` tinyint unsigned NOT NULL,
  `revision` bigint unsigned NOT NULL DEFAULT '1',
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_prism_update_sources_meta_id` CHECK ((`id` = 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `tasks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `task_no` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '任务编号',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '关联调用ID',
  `user_id` bigint unsigned DEFAULT NULL COMMENT '用户ID',
  `token_id` bigint unsigned DEFAULT NULL COMMENT '令牌ID',
  `model_code` varchar(80) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '模型标识',
  `route_operation` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '路由操作',
  `channel_id` bigint unsigned DEFAULT NULL COMMENT '渠道ID',
  `endpoint_id` bigint unsigned DEFAULT NULL COMMENT '端点配置ID',
  `account_id` bigint unsigned DEFAULT NULL COMMENT '渠道账号ID',
  `adapter_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT 'Endpoint Adapter ID',
  `adapter_revision_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT 'Endpoint Adapter Revision ID',
  `vendor_task_id` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '供应商任务ID',
  `status` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT 'pending' COMMENT '任务状态',
  `progress` bigint DEFAULT '0' COMMENT '进度(0-100)',
  `callback_url` varchar(500) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '回调地址',
  `callback_status` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '回调状态',
  `callback_attempts` bigint DEFAULT '0' COMMENT '回调尝试次数',
  `request_params` json DEFAULT NULL COMMENT '原始请求参数',
  `mapped_params` json DEFAULT NULL COMMENT '映射后参数',
  `endpoint_snapshot` json DEFAULT NULL COMMENT 'Endpoint execution snapshot',
  `submit_checkpoint` json DEFAULT NULL COMMENT '上游提交恢复检查点',
  `vendor_response` json DEFAULT NULL COMMENT '供应商原始响应',
  `result` json DEFAULT NULL COMMENT '统一结果',
  `error_message` text COLLATE utf8mb4_unicode_ci COMMENT '错误信息',
  `cost` decimal(20,8) NOT NULL DEFAULT '0.00000000' COMMENT '费用',
  `refunded` tinyint(1) DEFAULT '0' COMMENT '是否已退款',
  `account_slot_released` tinyint(1) DEFAULT '0' COMMENT '账号并发槽位是否已释放',
  `worker_lease_owner` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '任务执行租约所有者',
  `worker_lease_stage` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '任务执行租约阶段',
  `worker_lease_expires_at` datetime(3) DEFAULT NULL COMMENT '任务执行租约过期时间',
  `poll_cursor` bigint NOT NULL DEFAULT '0' COMMENT '下一轮轮询序号',
  `started_at` datetime(3) DEFAULT NULL COMMENT '开始时间',
  `completed_at` datetime(3) DEFAULT NULL COMMENT '完成时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_tasks_task_no` (`task_no`),
  KEY `idx_tasks_created_at` (`created_at`),
  KEY `idx_tasks_deleted_at` (`deleted_at`),
  KEY `idx_tasks_call_id` (`call_id`),
  KEY `idx_tasks_user_id` (`user_id`),
  KEY `idx_tasks_token_id` (`token_id`),
  KEY `idx_tasks_model_code` (`model_code`),
  KEY `idx_tasks_channel_id` (`channel_id`),
  KEY `idx_tasks_vendor_task_id` (`vendor_task_id`),
  KEY `idx_tasks_status` (`status`),
  KEY `idx_tasks_worker_lease_expires_at` (`worker_lease_expires_at`),
  KEY `fk_tasks_endpoint` (`endpoint_id`),
  KEY `idx_tasks_route_operation` (`route_operation`),
  KEY `idx_tasks_adapter_id` (`adapter_id`),
  KEY `idx_tasks_adapter_revision_id` (`adapter_revision_id`),
  CONSTRAINT `fk_tasks_channel` FOREIGN KEY (`channel_id`) REFERENCES `channels` (`id`),
  CONSTRAINT `fk_tasks_endpoint` FOREIGN KEY (`endpoint_id`) REFERENCES `endpoints` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `token_auth_state_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token_id` bigint unsigned NOT NULL,
  `auth_version` bigint unsigned NOT NULL,
  `old_status` tinyint NOT NULL,
  `new_status` tinyint NOT NULL,
  `reason_code` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `actor_user_id` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_token_auth_event_version` (`token_id`,`auth_version`),
  CONSTRAINT `fk_token_auth_event_token` FOREIGN KEY (`token_id`) REFERENCES `tokens` (`id`),
  CONSTRAINT `ck_token_auth_event_status` CHECK (((`old_status` in (0,1)) and (`new_status` in (0,1))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `token_budget_adjustment_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `budget_window_id` bigint unsigned NOT NULL,
  `adjustment_seq` bigint unsigned NOT NULL,
  `amount_delta` decimal(38,18) NOT NULL,
  `source_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_key` varchar(160) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_token_budget_adjustments_window_seq` (`budget_window_id`,`adjustment_seq`),
  UNIQUE KEY `uq_token_budget_adjustments_source` (`source_type`,`source_key`),
  CONSTRAINT `fk_token_budget_adjustments_window` FOREIGN KEY (`budget_window_id`) REFERENCES `token_budget_windows` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `token_budget_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token_id` bigint unsigned NOT NULL,
  `policy_code` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
  `window_kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `limit_amount` decimal(38,18) DEFAULT NULL,
  `period_seconds` bigint unsigned DEFAULT NULL,
  `timezone_name` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'UTC',
  `algorithm_version` int unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_token_budget_policies_token_code` (`token_id`,`policy_code`),
  UNIQUE KEY `uq_token_budget_policies_token_id_id` (`token_id`,`id`),
  CONSTRAINT `ck_token_budget_policies_kind` CHECK ((`window_kind` in (_utf8mb4'periodic',_utf8mb4'lifetime',_utf8mb4'unlimited'))),
  CONSTRAINT `ck_token_budget_policies_limit` CHECK ((((`window_kind` = _utf8mb4'unlimited') and (`limit_amount` is null) and (`period_seconds` is null)) or ((`window_kind` <> _utf8mb4'unlimited') and (`limit_amount` is not null) and (`limit_amount` >= 0)))),
  CONSTRAINT `ck_token_budget_policies_period` CHECK (((`window_kind` <> _utf8mb4'periodic') or ((`period_seconds` is not null) and (`period_seconds` > 0))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `token_budget_policy_activations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token_id` bigint unsigned NOT NULL,
  `policy_id` bigint unsigned NOT NULL,
  `activation_seq` bigint unsigned NOT NULL,
  `predecessor_id` bigint unsigned DEFAULT NULL,
  `effective_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_token_budget_activations_token_seq` (`token_id`,`activation_seq`),
  UNIQUE KEY `uq_token_budget_activations_token_effective` (`token_id`,`effective_at`),
  UNIQUE KEY `uq_token_budget_activations_token_id_id` (`token_id`,`id`),
  UNIQUE KEY `uq_token_budget_activations_predecessor` (`token_id`,`predecessor_id`),
  KEY `fk_token_budget_activations_policy` (`token_id`,`policy_id`),
  CONSTRAINT `fk_token_budget_activations_policy` FOREIGN KEY (`token_id`, `policy_id`) REFERENCES `token_budget_policies` (`token_id`, `id`),
  CONSTRAINT `fk_token_budget_activations_predecessor` FOREIGN KEY (`token_id`, `predecessor_id`) REFERENCES `token_budget_policy_activations` (`token_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `token_budget_windows` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token_id` bigint unsigned NOT NULL,
  `policy_id` bigint unsigned NOT NULL,
  `activation_id` bigint unsigned NOT NULL,
  `window_start` datetime(3) NOT NULL,
  `window_end` datetime(3) DEFAULT NULL,
  `limit_amount` decimal(38,18) DEFAULT NULL,
  `used_amount` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `held_amount` decimal(38,18) NOT NULL DEFAULT '0.000000000000000000',
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_token_budget_windows_policy_start` (`policy_id`,`window_start`),
  UNIQUE KEY `uq_token_budget_windows_token_id_id` (`token_id`,`id`),
  KEY `fk_token_budget_windows_policy` (`token_id`,`policy_id`),
  KEY `fk_token_budget_windows_activation` (`token_id`,`activation_id`),
  CONSTRAINT `fk_token_budget_windows_activation` FOREIGN KEY (`token_id`, `activation_id`) REFERENCES `token_budget_policy_activations` (`token_id`, `id`),
  CONSTRAINT `fk_token_budget_windows_policy` FOREIGN KEY (`token_id`, `policy_id`) REFERENCES `token_budget_policies` (`token_id`, `id`),
  CONSTRAINT `ck_token_budget_windows_amounts` CHECK (((`used_amount` >= 0) and (`held_amount` >= 0) and ((`limit_amount` is null) or (`limit_amount` >= 0))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `token_channel_priorities` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `token_id` bigint unsigned NOT NULL COMMENT '令牌ID',
  `capability_code` varchar(30) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '能力编码',
  `channel_id` bigint unsigned NOT NULL COMMENT '渠道ID',
  `priority` bigint DEFAULT '1' COMMENT '优先级(1最高)',
  PRIMARY KEY (`id`),
  KEY `idx_token_channel_priorities_created_at` (`created_at`),
  KEY `idx_token_channel_priorities_deleted_at` (`deleted_at`),
  KEY `idx_token_capability` (`token_id`,`capability_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `tokens` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `user_id` bigint unsigned DEFAULT '0' COMMENT '用户ID',
  `xfs_api_key` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `selector` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `secret_digest` binary(32) DEFAULT NULL,
  `secret_digest_version` smallint unsigned NOT NULL DEFAULT '1',
  `auth_version` bigint unsigned NOT NULL DEFAULT '1',
  `key_hint` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '密钥提示(后四位)',
  `name` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '令牌名称',
  `balance` decimal(20,8) NOT NULL DEFAULT '0.00000000' COMMENT '剩余额度',
  `total_used` decimal(20,8) NOT NULL DEFAULT '0.00000000' COMMENT '已使用额度',
  `rate_limit` bigint DEFAULT '60' COMMENT '速率限制(次/分钟)',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `expires_at` datetime(3) DEFAULT NULL,
  `revoked_at` datetime(3) DEFAULT NULL,
  `xfs_base_url` varchar(2048) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_tokens_id_user_id` (`id`,`user_id`),
  UNIQUE KEY `uq_tokens_selector` (`selector`),
  KEY `idx_tokens_created_at` (`created_at`),
  KEY `idx_tokens_deleted_at` (`deleted_at`),
  KEY `idx_tokens_user_id` (`user_id`),
  CONSTRAINT `ck_tokens_auth_state` CHECK (((`auth_version` > 0) and (`selector` is not null) and (`secret_digest` is not null) and (((`status` = 1) and (`revoked_at` is null)) or ((`status` = 0) and (`revoked_at` is not null))))),
  CONSTRAINT `ck_tokens_secret_digest_version` CHECK ((`secret_digest_version` in (0,1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `users` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `username` varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '用户名',
  `password` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '密码(加密)',
  `role` varchar(10) COLLATE utf8mb4_unicode_ci DEFAULT 'user' COMMENT '角色(admin/user)',
  `balance` decimal(20,8) NOT NULL DEFAULT '0.00000000' COMMENT '账户余额',
  `status` tinyint DEFAULT '1' COMMENT '状态(1启用/0禁用)',
  `session_version` bigint unsigned NOT NULL DEFAULT '0' COMMENT '控制台会话版本',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_users_username` (`username`),
  KEY `idx_users_created_at` (`created_at`),
  KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `video_assets` (
  `id` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '素材ID',
  `token_id` bigint unsigned NOT NULL COMMENT '令牌ID',
  `sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '文件SHA-256',
  `size_bytes` bigint NOT NULL COMMENT '文件大小(字节)',
  `kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型(image/video/audio)',
  `content_type` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'MIME类型',
  `duration_seconds` decimal(6,2) DEFAULT NULL COMMENT '时长(秒)',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态(uploading/ready/expired)',
  `storage_path` varchar(1024) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Storage path or public URL',
  `upstream_refs` json DEFAULT NULL COMMENT '上游引用缓存',
  `expires_at` datetime(3) NOT NULL COMMENT '过期时间',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_video_assets_token_sha256_status` (`token_id`,`sha256`,`status`),
  KEY `idx_token_sha256` (`token_id`,`sha256`,`status`),
  KEY `idx_expires` (`status`,`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频素材';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `video_channel_keys` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `channel_id` bigint unsigned NOT NULL COMMENT '所属渠道ID',
  `api_key` varchar(256) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'API密钥',
  `label` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '人类可读标签',
  `weight` int NOT NULL DEFAULT '1' COMMENT '同渠道内权重',
  `max_concurrency` int NOT NULL DEFAULT '0' COMMENT '最大并发(0=不限)',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT '状态(active/disabled/exhausted)',
  `current_concurrency` int NOT NULL DEFAULT '0' COMMENT '当前并发',
  `total_calls` bigint NOT NULL DEFAULT '0' COMMENT '总调用次数',
  `consecutive_failures` int NOT NULL DEFAULT '0' COMMENT '连续失败次数',
  `last_used_at` datetime(3) DEFAULT NULL COMMENT '最后使用时间',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_channel_status` (`channel_id`,`status`),
  CONSTRAINT `video_channel_keys_ibfk_1` FOREIGN KEY (`channel_id`) REFERENCES `video_channels` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频渠道密钥';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `video_channels` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '渠道名称',
  `adapter_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '适配器类型(seedance/generic)',
  `adapter_profile` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '适配器配置版本',
  `base_url` varchar(256) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '上游地址',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT '状态(active/disabled)',
  `priority` int NOT NULL DEFAULT '0' COMMENT '选路优先级(降序)',
  `request_timeout_seconds` smallint unsigned NOT NULL DEFAULT '30' COMMENT '上游请求超时(秒)',
  `models` json NOT NULL COMMENT '支持模型列表',
  `capabilities` json DEFAULT NULL COMMENT '能力声明',
  `supports_first_frame` tinyint(1) NOT NULL DEFAULT '0' COMMENT '支持首帧',
  `supports_last_frame` tinyint(1) NOT NULL DEFAULT '0' COMMENT '支持尾帧',
  `supports_audio` tinyint(1) NOT NULL DEFAULT '0' COMMENT '支持生成音频',
  `supports_web_search` tinyint(1) NOT NULL DEFAULT '0' COMMENT '支持联网增强',
  `cancel_mode` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'disabled' COMMENT '取消策略(disabled/local_only/provider)',
  `pricing` json DEFAULT NULL COMMENT '计费配置',
  `pricing_mode` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'fixed' COMMENT '计费模式(fixed/upstream_estimate)',
  `fixed_price` decimal(10,4) NOT NULL DEFAULT '0.0000' COMMENT '固定价格',
  `markup_ratio` decimal(4,2) NOT NULL DEFAULT '1.00' COMMENT '加价系数',
  `asset_resolver` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'direct_url' COMMENT '素材解析器(sub2api/direct_url/tos)',
  `result_storage_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否转存生成结果',
  `extra_config` json DEFAULT NULL COMMENT '适配器附加配置',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频渠道';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE IF NOT EXISTS `video_tasks` (
  `id` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '任务ID',
  `call_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_id` bigint unsigned NOT NULL DEFAULT '0',
  `token_id` bigint unsigned NOT NULL COMMENT '令牌ID',
  `model` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '模型标识',
  `vendor_model` varchar(120) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '任务状态',
  `progress` tinyint unsigned NOT NULL DEFAULT '0' COMMENT '进度(0-100)',
  `task_mode` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '任务模式(text/references)',
  `service_tier` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'standard' COMMENT '视频执行档位',
  `prompt` text COLLATE utf8mb4_unicode_ci COMMENT '提示词',
  `resolution` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '分辨率',
  `ratio` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '比例',
  `duration` smallint DEFAULT NULL COMMENT '时长(秒)',
  `generate_audio` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否生成音频',
  `content_json` json DEFAULT NULL COMMENT '内容项',
  `params_json` json DEFAULT NULL COMMENT '附加参数',
  `channel_id` bigint unsigned DEFAULT NULL COMMENT '渠道ID',
  `key_id` bigint unsigned DEFAULT NULL COMMENT '密钥ID',
  `adapter_type` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '适配器类型',
  `route_plan` json DEFAULT NULL,
  `provider_task_id` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '上游任务ID',
  `provider_response` json DEFAULT NULL COMMENT '上游原始响应',
  `provider_metadata` json DEFAULT NULL COMMENT '结构化上游任务元数据',
  `estimated_cost` decimal(10,4) DEFAULT NULL COMMENT '预估费用',
  `markup_ratio` decimal(4,2) DEFAULT NULL COMMENT '加价系数',
  `final_cost` decimal(10,4) DEFAULT NULL COMMENT '最终费用',
  `billing_status` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '计费状态(pending/charged/refunded/partial_refund)',
  `result_json` json DEFAULT NULL COMMENT '生成结果',
  `error_message` text COLLATE utf8mb4_unicode_ci COMMENT '错误信息',
  `worker_lease` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Worker lease owner',
  `worker_lease_stage` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `worker_lease_expires_at` datetime(3) DEFAULT NULL,
  `submit_checkpoint` json DEFAULT NULL,
  `poll_count` int NOT NULL DEFAULT '0' COMMENT '轮询次数',
  `callback_url` varchar(512) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '回调地址',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `submitted_at` datetime(3) DEFAULT NULL COMMENT '提交时间',
  `completed_at` datetime(3) DEFAULT NULL COMMENT '完成时间',
  PRIMARY KEY (`id`),
  KEY `idx_token_status` (`token_id`,`status`,`created_at` DESC),
  KEY `idx_status_created` (`status`,`created_at`),
  KEY `idx_channel_key` (`channel_id`,`key_id`),
  KEY `idx_video_tasks_call_id` (`call_id`),
  KEY `idx_user_status` (`user_id`,`status`,`created_at` DESC),
  KEY `idx_video_worker_recovery` (`status`,`worker_lease_expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频生成任务';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;


-- Builtin system metadata only.
INSERT INTO `crypto_keyring_state` (`id`, `purpose`, `current_version`, `created_at`, `updated_at`) VALUES (1,'gateway-credential',1,CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `crypto_keyring_state` (`id`, `purpose`, `current_version`, `created_at`, `updated_at`) VALUES (2,'gateway-payload',1,CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `crypto_key_versions` (`id`, `keyring_id`, `key_version`, `status`, `provider_key_ref`, `algorithm`, `created_at`, `retired_at`) VALUES (1,1,1,'current','env:PRISM_GATEWAY_KEK_B64','aes-256-gcm',CURRENT_TIMESTAMP(3),NULL) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `crypto_key_versions` (`id`, `keyring_id`, `key_version`, `status`, `provider_key_ref`, `algorithm`, `created_at`, `retired_at`) VALUES (2,2,1,'current','env:PRISM_GATEWAY_PAYLOAD_KEK_B64','aes-256-gcm',CURRENT_TIMESTAMP(3),NULL) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `gw_catalog_runtime_state` (`id`, `active_release_id`, `active_deployment_generation_id`, `state_version`, `updated_at`) VALUES (1,NULL,NULL,1,CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `gw_operation_contracts` (`id`, `operation_code`, `contract_version`, `status`, `created_at`) VALUES (1,'images.edit',1,'active',CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `gw_operation_routes` (`id`, `operation_contract_id`, `http_method`, `route_template`, `created_at`) VALUES (1,1,'POST','/v1/images/edits',CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `gw_protocol_definitions` (`id`, `protocol_code`, `protocol_version`, `protocol_kind`, `display_name`, `definition_json`, `definition_digest`, `status`, `config_version`, `created_at`, `updated_at`) VALUES (1,'standard_model_catalog',1,'catalog_discovery','标准模型列表（Bearer /v1/models）','{\"auth\": {\"mode\": \"bearer\", \"header_name\": \"Authorization\", \"header_format\": \"Bearer {secret}\"}, \"kind\": \"catalog_discovery\", \"steps\": [{\"name\": \"models\", \"path\": \"/v1/models\", \"method\": \"GET\"}], \"models\": {\"items_path\": \"data[]\", \"model_path\": \"id\", \"description_path\": \"description\"}, \"schema_version\": 1}','b6138dbcdd4497a222d29114cc1432f76529fa6b0bd7a9718f0f0fe05148490d','active',1,CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `gw_upstream_error_rules` (`id`, `patterns`, `message`, `updated_at`) VALUES (1,'[\"(?i)余额|\\\\bbalance\\\\b\", \"(?i)[¥￥$€£]\\\\s*[0-9]+(?:[.,][0-9]+)*|\\\\b(?:USD|CNY|RMB|EUR|GBP)\\\\s*[0-9]+(?:[.,][0-9]+)*|[0-9]+(?:[.,][0-9]+)*\\\\s*(?:元|美元|人民币|\\\\b(?:USD|CNY|RMB|EUR|GBP)\\\\b)\"]','服务暂时不可用，请稍后重试',CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;
INSERT INTO `prism_update_sources_meta` (`id`, `revision`, `updated_at`) VALUES (1,1,CURRENT_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE `id` = `id`;

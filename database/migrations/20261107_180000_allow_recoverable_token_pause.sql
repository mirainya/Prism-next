-- Reason: a paused Key must be recoverable without erasing a revocation.
-- Requirement: console pause/resume and permanent revocation are separate actions.
-- Scope: only the tokens authorization CHECK; credentials, balances and rows are unchanged.
-- Deployment: apply before the build exposing pause/resume. Keep revoked_at for permanent revocation.
ALTER TABLE tokens
    DROP CHECK ck_tokens_auth_state,
    ADD CONSTRAINT ck_tokens_auth_state CHECK (
        auth_version > 0
        AND selector IS NOT NULL
        AND secret_digest IS NOT NULL
        AND (status = 0 OR (status = 1 AND revoked_at IS NULL))
    );

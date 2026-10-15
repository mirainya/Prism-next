# Clean migration baseline

The public migration lineage begins at `20261015_200000`. Original SQL is privately archived. Public retired-history metadata contains version numbers and checksums only.

## New installations

Run `prism migrate up`. The clean baseline contains application structure and builtin system metadata. It does not create provider channels, credentials, pricing, or model mappings.

## Existing installations

Stop application traffic and back up the database before running `prism migrate rebase`, followed by `prism migrate up`.

Supported checkpoints are the complete 117-version v2.0.20 history and the complete 118-version history including async tracking. Every version, name hash, SQL checksum, and dirty flag is validated. The complete table and trigger structure must match. Unexpected objects and structural differences are rejected.

Rebase only adds migration ledger records in a transaction. Existing ledger records and application data remain unchanged. Both historical and forward SQL versions are checksum-pinned; the forward version makes table engine, charset and collation explicit. Tracking is registered without execution only when the approved version mapping and both table definitions are proven; otherwise it remains pending.

Older or partially migrated databases must first finish upgrading using the previous release. Rebase cannot repair drift or dirty records. A database adopted by the new lineage must not be started with an older binary that cannot recognize its new ledger records.

## Privacy boundary

Provider onboarding files remain private. Tests and examples use fictitious domains. Removing files from the current revision does not erase historical Git objects, tags, or previously published release artifacts; rewriting published history requires a separate reviewed operation.

## Validation status

Local isolated MySQL verification passed for fresh installation, both legacy checkpoints, idempotent adoption, dirty-history rejection, schema-drift rejection, and unchanged application data across adoption. Production migration and publication have not been performed.

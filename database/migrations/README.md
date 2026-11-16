# Database Migrations

`database/migrations` is the only production schema source. The server never runs GORM AutoMigrate.

## Commands

```bash
prism migrate status
prism migrate up
prism migrate adopt
```

- `status` reports applied and pending managed migrations.
- `up` applies pending managed migrations under a MySQL named lock.
- `adopt` registers a verified legacy database at the managed baseline.

The server refuses to start when the database is legacy, dirty, or has pending migrations.

## Baseline

`20261026_120000_clean_schema_baseline.sql` contains the complete public schema for a new database. Retired migration history is represented by hashes in `internal/migrate/retired_history.json`; provider configuration and historical SQL are not shipped in the public repository.

An installation created before the baseline must:

1. Stop every Prism HTTP and Worker process.
2. Back up the database.
3. Obtain the matching private migration archive and apply every missing migration through `20261026_000000_price_display_settings.sql`.
4. Run `prism migrate rebase` to register the verified public baseline.
5. Run `prism migrate up` for any later managed migrations.

## Rules

- Never edit or rename an applied migration.
- Use `YYYYMMDD_HHMMSS_description.sql` for every new migration.
- Keep migrations idempotent because MySQL DDL cannot be fully transactional.
- Include the reason, requirement, impact, and deployment constraints in the file header.
- Test both a fresh baseline and repeated execution before release.

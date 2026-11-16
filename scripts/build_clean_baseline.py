"""Build a baseline candidate from an isolated, fully migrated MySQL dump.

Inputs are private local artifacts. No database connection is made here.
Provider configuration and migration ledger rows are never copied.
"""

import argparse
import hashlib
import json
import pathlib
import re


SYSTEM_TABLES = (
    "crypto_keyring_state", "crypto_key_versions", "gw_catalog_runtime_state",
    "gw_operation_contracts", "gw_operation_routes", "gw_protocol_definitions",
    "gw_upstream_error_rules", "prism_update_sources_meta",
)


def canonical_ddl(sql):
    sql = sql.strip().rstrip(";")
    if sql.startswith("CREATE TABLE IF NOT EXISTS "):
        sql = "CREATE TABLE " + sql.removeprefix("CREATE TABLE IF NOT EXISTS ")
    result, quote, pending, index = [], None, False, 0
    while index < len(sql):
        char = sql[index]
        if quote:
            result.append(char)
            if char == "\\" and index + 1 < len(sql):
                index += 1
                result.append(sql[index])
            elif char == quote:
                if index + 1 < len(sql) and sql[index + 1] == quote:
                    index += 1
                    result.append(sql[index])
                else:
                    quote = None
        elif char.isspace():
            pending = bool(result)
        else:
            if char == "C":
                match = re.match(r"CHARACTER\s+SET\s+([a-zA-Z0-9_]+)\s+COLLATE\s+([a-zA-Z0-9_]+)", sql[index:])
                if match and match[2].startswith(match[1] + "_"):
                    index += match[0].index("COLLATE")
                    continue
            if sql.startswith("AUTO_INCREMENT=", index) and index > 0 and sql[index - 1].isspace():
                match = re.match(r"AUTO_INCREMENT=\d+(?=\s|$)", sql[index:])
                if match:
                    index += len(match[0])
                    continue
            if pending:
                result.append(" ")
                pending = False
            result.append(char)
            if char in "'\"`":
                quote = char
        index += 1
    if quote:
        raise ValueError("unclosed SQL quote")
    return "".join(result)


def build(structure, data, output, excluded_tables=()):
    sql = structure.read_text(encoding="utf-8")
    if re.search(r"^\s*(?:INSERT|REPLACE)\s", sql, re.M | re.I):
        raise ValueError("structure input contains data")
    for table in excluded_tables:
        if not re.fullmatch(r"[a-z][a-z0-9_]*", table):
            raise ValueError("invalid excluded table name")
        sql, count = re.subn(r"^CREATE TABLE `" + re.escape(table) + r"` \(.*?^\) ENGINE=.*?;\n",
                             "", sql, flags=re.M | re.S)
        if count != 1:
            raise ValueError("excluded table not found exactly once")
    tables = re.findall(r"^CREATE TABLE `([^`]+)`", sql, re.M)
    if not tables or "prism_schema_migrations" in tables:
        raise ValueError("expected application structure without the migration ledger")
    # DEFINER must be determined at installation, never copied from the builder.
    sql = re.sub(r"/\*!\d+ DEFINER=`[^`]+`@`[^`]+`\*/\s*", "", sql)
    sql = re.sub(r"(?m)^(\) ENGINE=[a-zA-Z0-9_]+) AUTO_INCREMENT=\d+", r"\1", sql)
    sql = re.sub(r"^DELIMITER .*\n", "", sql, flags=re.M)
    sql = sql.replace("*/;;", "*/;")
    if "DEFINER=" in sql or "DELIMITER " in sql:
        raise ValueError("unsupported dump control syntax")
    table_hashes = {}
    for match in re.finditer(r"^CREATE TABLE `([^`]+)` \(.*?^\) ENGINE=.*?;", sql, re.M | re.S):
        table_hashes[match[1]] = hashlib.sha256(canonical_ddl(match[0]).encode("utf-8")).hexdigest()
    triggers = []
    for match in re.finditer(
        r"TRIGGER `([^`]+)` (BEFORE|AFTER) (INSERT|UPDATE|DELETE) ON `([^`]+)` FOR EACH ROW (.*?)\s*\*/;",
        sql, re.S,
    ):
        triggers.append({"name": match[1], "table": match[4], "timing": match[2],
                         "event": match[3], "body_checksum": hashlib.sha256(
                             canonical_ddl(match[5]).encode("utf-8")).hexdigest()})
    if len(table_hashes) != len(tables) or len(triggers) != len(re.findall(r"\bTRIGGER `", sql)):
        raise ValueError("dump contains unsupported schema objects")
    # A failed installation can resume without modifying already inserted rows.
    sql = sql.replace("CREATE TABLE `", "CREATE TABLE IF NOT EXISTS `")
    sql = re.sub(r"(/\*!50003 CREATE\*/ /\*!50003 TRIGGER `([^`]+)`)",
                 lambda m: "DROP TRIGGER IF EXISTS `" + m[2] + "`;\n" + m[1], sql)
    seeds = []
    excluded = set()
    for line in data.read_text(encoding="utf-8").splitlines():
        match = re.match(r"INSERT INTO `([^`]+)`", line)
        if not match:
            continue
        if match[1] not in SYSTEM_TABLES:
            excluded.add(match[1])
            continue
        # These are trusted builtin seed rows, never production data. Avoid
        # publishing timestamps from the temporary database used by the builder.
        line = re.sub(r"'\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?'",
                      "CURRENT_TIMESTAMP(3)", line)
        seeds.append(line.rstrip(";") + " ON DUPLICATE KEY UPDATE `id` = `id`;")
    if not seeds:
        raise ValueError("builtin seed rows are missing")
    seeds.sort(key=lambda line: SYSTEM_TABLES.index(re.match(r"INSERT INTO `([^`]+)`", line)[1]))
    candidate = (
        "-- Clean application schema baseline; no provider onboarding data.\n"
        "-- Generated from a fully migrated isolated MySQL 8 database.\n"
        + sql + "\n-- Builtin system metadata only.\n" + "\n".join(seeds) + "\n"
    )
    if re.search(r"https?://", candidate, flags=re.I):
        raise ValueError("baseline must not contain deployment URLs")
    if output.exists():
        raise ValueError("candidate output already exists")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(candidate, encoding="utf-8")
    schema = {"schema": 1, "tables": table_hashes, "triggers": triggers}
    output.with_suffix(".schema.json").write_text(json.dumps(schema, indent=2) + "\n", encoding="utf-8")
    report = {
        "sha256": hashlib.sha256(candidate.encode("utf-8")).hexdigest(),
        "table_count": len(tables),
        "trigger_count": len(re.findall(r"\bTRIGGER `", candidate)),
        "system_seed_rows": len(seeds), "excluded_data_tables": sorted(excluded),
        "verified_in_mysql": False,
    }
    output.with_suffix(".report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--structure", type=pathlib.Path, required=True)
    parser.add_argument("--data", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--exclude-table", action="append", default=[])
    args = parser.parse_args()
    print(json.dumps(build(args.structure, args.data, args.output, args.exclude_table)))


if __name__ == "__main__":
    main()

"""Archive immutable SQL privately and prepare a public, hash-only history.

This tool never connects to a database or changes the original migrations.
The output directory must be outside Git tracking (normally under .temp).
"""

import argparse
import hashlib
import json
import pathlib
import re
import shutil
import subprocess


def digest(content):
    return hashlib.sha256(content).hexdigest()


def prepare(source, output, through, baseline, compatible=(), forward=()):
    if output.exists():
        raise ValueError("archive output already exists")
    if not re.fullmatch(r"\d{8}_\d{6}", through) or not re.fullmatch(r"\d{8}_\d{6}", baseline):
        raise ValueError("versions must use YYYYMMDD_HHMMSS")
    if baseline <= through:
        raise ValueError("new baseline must follow the retired history")
    tracked = subprocess.run(
        ["git", "check-ignore", "--quiet", str(output.resolve())],
        cwd=source, check=False, capture_output=True,
    )
    if tracked.returncode != 0:
        raise ValueError("archive must be in a Git-ignored directory")
    migrations = []
    files = sorted((source / "database/migrations").glob("*.sql"))
    for path in files:
        match = re.fullmatch(r"(\d{8}_\d{6})_([a-z0-9][a-z0-9_]*)\.sql", path.name)
        if not match or match[1] < "20260718_150000":
            continue
        if match[1] > through:
            raise ValueError("source contains newer migrations; use an isolated release checkout")
        content = path.read_bytes().replace(b"\r\n", b"\n")
        migrations.append({
            "version": match[1],
            "checksum": digest(content),
            "name_checksum": digest(match[2].encode("utf-8")),
        })
    versions = [item["version"] for item in migrations]
    if not versions or versions[0] != "20260718_150000" or versions[-1] != through:
        raise ValueError("history must contain both the original baseline and cutoff")
    if len(versions) != len(set(versions)):
        raise ValueError("duplicate migration version")
    if len(set(compatible)) != len(compatible) or any(v not in versions[:-1] for v in compatible):
        raise ValueError("invalid compatible history checkpoint")
    mappings = []
    seen = set()
    for value in forward:
        old, new, checksum = value.split(":")
        if old not in versions or not re.fullmatch(r"\d{8}_\d{6}", new) or new <= baseline or new in seen or not re.fullmatch(r"[0-9a-f]{64}", checksum):
            raise ValueError("invalid forward migration mapping")
        seen.add(new)
        mappings.append({"retired_version": old, "forward_version": new, "forward_checksum": checksum})
    output.mkdir(parents=True)
    archive = output / "original-sql"
    archive.mkdir()
    inventory = []
    for path in files:
        target = archive / path.name
        shutil.copyfile(path, target)
        if target.read_bytes() != path.read_bytes():
            raise ValueError("archive verification failed")
        inventory.append({"filename": path.name, "sha256": digest(target.read_bytes())})
    manifest = {"schema": 1, "baseline_version": baseline,
                "through_version": through, "migrations": migrations}
    if compatible:
        manifest["compatible_through_versions"] = list(compatible)
    if mappings:
        manifest["forward_migrations"] = mappings
    (output / "retired_history.json").write_text(
        json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    (output / "private-inventory.json").write_text(
        json.dumps(inventory, indent=2) + "\n", encoding="utf-8")
    # Approval artifact only: the program does not execute these statements.
    sql = ["-- Isolated local verification only; never run on a production server.",
           "CREATE DATABASE `prism_baseline_audit_20261015` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
           "USE `prism_baseline_audit_20261015`;", "SET time_zone = '+00:00';"]
    for path in files:
        if any(path.name.startswith(version + "_") for version in versions):
            sql.extend(["\n-- " + path.name, path.read_text(encoding="utf-8")])
    replay = "\n".join(sql) + "\n"
    (output / "local-replay.sql").write_text(replay, encoding="utf-8")
    # DELIMITER is a mysql-client directive, not part of the archived SQL.
    client_replay = re.sub(r"(^CREATE TRIGGER\b.*?^END);",
                           lambda m: "DELIMITER $$\n" + m[1] + "$$\nDELIMITER ;",
                           replay, flags=re.M | re.S)
    (output / "local-replay.mysql.sql").write_text(client_replay, encoding="utf-8")
    return manifest, len(inventory)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--through", required=True)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--compatible-through", action="append", default=[])
    parser.add_argument("--forward-mapping", action="append", default=[], metavar="OLD_VERSION:NEW_VERSION:FORWARD_SHA256")
    args = parser.parse_args()
    manifest, count = prepare(args.source.resolve(), args.output.resolve(), args.through, args.baseline,
                              args.compatible_through, args.forward_mapping)
    print(json.dumps({"archived_files": count, "managed_versions": len(manifest["migrations"]),
                      "output": str(args.output)}, ensure_ascii=True))


if __name__ == "__main__":
    main()

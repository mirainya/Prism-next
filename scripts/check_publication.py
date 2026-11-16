"""Check the exact Git index for private paths and unreviewed HTTP hosts."""

import re
import subprocess
import sys
from pathlib import PurePosixPath
from urllib.parse import urlsplit


PUBLIC_HOSTS = {
    "github.com",
    "api.openai.com",
    "platform.openai.com",
    "generativelanguage.googleapis.com",
    "www.w3.org",
    "registry.npmjs.org",
    "registry.npmmirror.com",
    "opencollective.com",
    "tidelift.com",
    "jaywcjlove.github.io",
}
URL = re.compile(r"https?://[^\s\"'<>`\\]+")
EXAMPLE_HOST = re.compile(r"(?:^|\.)(?:example|test|invalid|example\.com|example\.test|example\.invalid|example\.evil)$")


def git(*args):
    return subprocess.check_output(["git", *args])


def check():
    paths = git("ls-files", "-z").decode().strip("\0").split("\0")
    findings = []
    for path in paths:
        parts = PurePosixPath(path).parts
        if (
            any(part in {".private", ".temp", ".agents", ".claude", "node_modules", "data", "dist"} for part in parts)
            or path.startswith(("configs/video/", "scripts/private/"))
            or path.endswith((".private_test.go", ".private.py", ".log", ".exe", ".pem", ".key"))
            or (PurePosixPath(path).name.startswith(".env") and not path.endswith(".example"))
            or path in {"configs/config.yaml", "configs/config.local.yaml", "caps_out.json"}
        ):
            findings.append(f"{path}: private or generated file")
        data = git("show", ":" + path)
        markers = [b"-----BEGIN " + kind + b"PRIVATE KEY-----" for kind in (b"", b"RSA ", b"EC ", b"OPENSSH ")]
        if any(marker in data for marker in markers):
            findings.append(f"{path}: private key marker")
        text = data.decode("utf-8", errors="ignore")
        for number, line in enumerate(text.splitlines(), 1):
            for match in URL.finditer(line):
                raw = match.group().rstrip(".,;)}]")
                try:
                    parsed = urlsplit(raw)
                    host = (parsed.hostname or "").lower()
                except ValueError:
                    findings.append(f"{path}:{number}: malformed HTTP URL")
                    continue
                if host in PUBLIC_HOSTS or host == "localhost" or EXAMPLE_HOST.search(host):
                    continue
                if re.fullmatch(r"\d+(?:\.\d+)*", host):
                    # Literal addresses appear in network boundary tests and local
                    # configuration. They are reviewed separately from hostnames.
                    continue
                if "." not in host:
                    # Interpolation, invalid URL and user-info boundary fixtures.
                    continue
                findings.append(f"{path}:{number}: HTTP host requires review")
    for finding in sorted(set(findings)):
        print(finding)
    print(f"Publication index: {len(paths)} files, {len(set(findings))} findings")
    return bool(findings)


if __name__ == "__main__":
    sys.exit(check())

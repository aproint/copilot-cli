#!/usr/bin/env python3
"""Select CI checks from a NUL-delimited git diff path list."""

import os
import sys
from pathlib import Path


def classify(paths: list[str]) -> dict[str, bool]:
    checks = {"full": False, "go": False, "js": False, "license": False}
    if not paths:
        return dict.fromkeys(checks, True)

    for path in paths:
        if path in {"Makefile", ".node-version", ".github/workflows/ci.yml"} or path.startswith(
            "scripts/ci_changes"
        ):
            return dict.fromkeys(checks, True)

        if path == "scripts/license.sh":
            checks["license"] = True
        elif path in {"go.mod", "go.sum", ".go-version"} or path.startswith(("cmd/", "internal/")):
            checks["go"] = True
        elif path.startswith("cf-custom-resources/") and path != "cf-custom-resources/README.md":
            checks["js"] = True
        elif (
            path in {
                ".github/dependabot.yml",
                ".github/CODEOWNERS",
                ".github/PULL_REQUEST_TEMPLATE.md",
                "mkdocs.yml",
                "requirements.txt",
                "Dockerfile.site",
                "cf-custom-resources/README.md",
            }
            or path.startswith(("site/", ".github/ISSUE_TEMPLATE/"))
            or ("/" not in path and path.endswith(".md"))
        ):
            continue
        else:
            return dict.fromkeys(checks, True)

        if path.endswith((".go", ".js")):
            checks["license"] = True

    return checks


def main() -> None:
    paths = [os.fsdecode(path) for path in Path(sys.argv[1]).read_bytes().split(b"\0") if path]
    checks = classify(paths)
    print(f"Changed paths: {paths!r}", file=sys.stderr)
    print(f"Selected checks: {checks!r}", file=sys.stderr)
    for check, selected in checks.items():
        print(f"{check}={str(selected).lower()}")


if __name__ == "__main__":
    main()

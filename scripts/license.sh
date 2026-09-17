#!/bin/bash
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# Copyright APROINT, s.r.o. in modifications to this fork.
# SPDX-License-Identifier: Apache-2.0
#
# Check Go and JavaScript files changed by a pull request or merge group.

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 rootdir base-sha head-sha" >&2
    exit 1
fi

python3 - "$1" "$2" "$3" <<'PY'
import pathlib
import subprocess
import sys

AMAZON = "// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved."
APROINT = "// Copyright APROINT, s.r.o."
MODIFICATIONS = "// Copyright APROINT, s.r.o. in modifications to this fork."
SPDX = "// SPDX-License-Identifier: Apache-2.0"
DERIVED = "// Derived from AWS Copilot CLI source."

root = pathlib.Path(sys.argv[1]).resolve()
base, head = sys.argv[2:4]


def git(*args):
    return subprocess.run(
        ["git", "-C", str(root), *args], capture_output=True, check=True
    ).stdout


try:
    changes = git(
        "diff", "--no-renames", "--diff-filter=AM", "--name-status", "-z",
        base, head, "--", "*.go", "*.js"
    ).split(b"\0")
except subprocess.CalledProcessError as error:
    sys.exit(f"license check: cannot compare {base} and {head}: {error.stderr.decode(errors='replace').strip()}")


def in_scope(path):
    parts = pathlib.PurePosixPath(path).parts
    return not (
        "mocks" in parts[:-1]
        or (path.endswith(".go") and parts[-1].startswith("mock_"))
        or "node_modules" in parts[:-1]
        or parts[0] == "site"
    )


def header_kind(lines, path):
    offset = 0
    if path.endswith(".go") and lines and lines[0].startswith("//go:build "):
        offset = 1
        while offset < len(lines) and lines[offset].startswith("// +build "):
            offset += 1
        if offset >= len(lines) or lines[offset] != "":
            return None, "expected a blank line after Go build tags"
        offset += 1

    formats = {
        "derived": [AMAZON, MODIFICATIONS, SPDX, DERIVED],
        "inherited": [AMAZON, MODIFICATIONS, SPDX],
        "new": [APROINT, SPDX],
        "legacy": [AMAZON, SPDX],
    }
    for kind, expected in formats.items():
        if lines[offset:offset + len(expected)] == expected:
            following = lines[offset + len(expected):offset + len(expected) + 1]
            if following and following[0].startswith(
                ("// Copyright ", "// SPDX-License-Identifier:", "// Derived from ")
            ):
                return None, "unexpected extra license line after header"
            return kind, None
    return None, f"invalid header at line {offset + 1}"


failures = 0
for status, raw_path in zip(changes[0::2], changes[1::2]):
    path = raw_path.decode("utf-8", errors="surrogateescape")
    if not in_scope(path):
        continue

    lines = (root / path).read_text(encoding="utf-8").splitlines()
    kind, error = header_kind(lines, path)
    if error or kind == "legacy":
        failures += 1
        print(f"{path}: {error or 'Amazon-only header is no longer valid'}")
        continue

    if status == b"A":
        if kind not in ("new", "derived"):
            failures += 1
            print(f"{path}: new files need the APROINT header, or the derived-source marker")
        continue

    try:
        previous = git("show", f"{base}:{path}").decode("utf-8").splitlines()
    except subprocess.CalledProcessError:
        failures += 1
        print(f"{path}: cannot read the base version")
        continue
    previous_kind, _ = header_kind(previous, path)
    if previous_kind in ("new", "inherited", "derived") and kind != previous_kind:
        failures += 1
        print(f"{path}: expected the {previous_kind} header from the base version")

if failures:
    sys.exit(f"license check: {failures} file(s) have invalid headers")
PY

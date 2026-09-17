#!/bin/bash
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# Copyright APROINT, s.r.o. in modifications to this fork.
# SPDX-License-Identifier: Apache-2.0
#
# Check tracked Go and JavaScript files against the tree before the first APROINT commit.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 rootdir" >&2
    exit 1
fi

python3 - "$1" <<'PY'
import pathlib
import subprocess
import sys

FORK_POINT = "a0dbe68908e55c4292e5838bd6cfbaea38364879"
AMAZON = "// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved."
APROINT = "// Copyright APROINT, s.r.o."
MODIFICATIONS = "// Copyright APROINT, s.r.o. in modifications to this fork."
SPDX = "// SPDX-License-Identifier: Apache-2.0"
DERIVED = "// Derived from AWS Copilot CLI source."

root = pathlib.Path(sys.argv[1]).resolve()


def git(*args):
    return subprocess.run(
        ["git", "-C", str(root), *args], capture_output=True, check=True
    ).stdout


try:
    git("cat-file", "-e", f"{FORK_POINT}^{{commit}}")
except subprocess.CalledProcessError:
    sys.exit(f"license check: fork-point commit {FORK_POINT} is unavailable; fetch it before checking licenses")

try:
    inherited = set(git("ls-tree", "-r", "--name-only", "-z", FORK_POINT).split(b"\0"))
    tracked = git("ls-files", "-z", "--", "*.go", "*.js").split(b"\0")
except subprocess.CalledProcessError as error:
    sys.exit(f"license check: git failed: {error.stderr.decode(errors='replace').strip()}")


def in_scope(path):
    parts = pathlib.PurePosixPath(path).parts
    return not (
        "mocks" in parts[:-1]
        or (path.endswith(".go") and parts[-1].startswith("mock_"))
        or "node_modules" in parts[:-1]
        or parts[0] == "site"
    )


failures = 0
for raw_path in tracked:
    if not raw_path:
        continue
    path = raw_path.decode("utf-8", errors="surrogateescape")
    if not in_scope(path):
        continue

    lines = (root / path).read_text(encoding="utf-8").splitlines()
    offset = 0
    if path.endswith(".go") and lines and lines[0].startswith("//go:build "):
        offset = 1
        while offset < len(lines) and lines[offset].startswith("// +build "):
            offset += 1
        if offset >= len(lines) or lines[offset] != "":
            failures += 1
            print(f"{path}: expected a blank line after Go build tags")
            continue
        offset += 1

    if raw_path in inherited:
        expected = [AMAZON, MODIFICATIONS, SPDX]
    elif lines[offset:offset + 1] == [AMAZON]:
        expected = [AMAZON, MODIFICATIONS, SPDX, DERIVED]
    else:
        expected = [APROINT, SPDX]

    if lines[offset:offset + len(expected)] != expected:
        failures += 1
        print(f"{path}: expected header at line {offset + 1}: {' | '.join(expected)}")
    elif len(lines) > offset + len(expected) and lines[offset + len(expected)].startswith(
        ("// Copyright ", "// SPDX-License-Identifier:", "// Derived from ")
    ):
        failures += 1
        print(f"{path}: unexpected extra license line after header")

if failures:
    sys.exit(f"license check: {failures} file(s) have invalid headers")
PY

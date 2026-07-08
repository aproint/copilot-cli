#!/usr/bin/env bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [ "$#" -lt 3 ]; then
  echo "usage: $0 <version> <asset-dir> <asset> [asset ...]" >&2
  exit 2
fi

version="$1"
asset_dir="$2"
shift 2

for asset in "$@"; do
  path="$asset_dir/$asset"
  if [ ! -f "$path" ]; then
    echo "release asset is missing: $path" >&2
    exit 1
  fi

  chmod 0755 "$path"
  version_output="$("$path" --version)"
  expected_output="copilot version: $version"
  if [ "$version_output" != "$expected_output" ]; then
    echo "$asset reported unexpected version: $version_output" >&2
    echo "expected: $expected_output" >&2
    exit 1
  fi

  "$path" --help >/dev/null
  echo "smoked $asset"
done

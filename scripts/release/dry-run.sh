#!/usr/bin/env bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <version> [destination-dir]" >&2
  exit 2
fi

version="$1"
destination_dir="${2:-dist/release}"

make release VERSION="$version"
bash scripts/release/package.sh "$version" bin/local "$destination_dir"
bash scripts/release/checksums.sh "$destination_dir"

echo "Release dry run assets are in $destination_dir"

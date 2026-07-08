#!/usr/bin/env bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

asset_dir="${1:-dist/release}"
checksum_file="$asset_dir/SHA256SUMS"

if [ ! -d "$asset_dir" ]; then
  echo "asset directory does not exist: $asset_dir" >&2
  exit 1
fi

tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT

find "$asset_dir" -maxdepth 1 -type f \
  ! -name 'SHA256SUMS' \
  ! -name '*.sigstore.json' \
  -print | sort | while IFS= read -r path; do
    name="$(basename "$path")"
    (cd "$asset_dir" && shasum -a 256 "$name")
  done > "$tmp_file"

mv "$tmp_file" "$checksum_file"
echo "$checksum_file"
